package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/deviceupdate"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func readyStageLaserDevice(t *testing.T) deviceexperience.Device {
	t.Helper()
	observation := stagelaser.Observation{
		SchemaVersion:          stagelaser.SchemaVersion1,
		FirmwareVersion:        "0.1.0",
		ControlContractVersion: stagelaser.ControlContractVersion,
		ArmState:               stagelaser.ArmDisarmed,
		LogicalState:           stagelaser.StateOff,
		StateQuality:           stagelaser.StateQualityTracked,
		DriverKind:             stagelaser.DriverMechanicalRelay,
		Limits:                 stagelaser.DefaultMechanicalLimits(),
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	return deviceexperience.Device{
		ID:              "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ProfileID:       stagelaser.ProfileID,
		Kind:            deviceexperience.DeviceGeneric,
		ClientVersion:   "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled:         true,
		Runtime: &deviceexperience.RuntimeState{
			Connection:    deviceexperience.ConnectionOnline,
			Readiness:     deviceexperience.ReadinessReady,
			ObservedState: raw,
		},
	}
}

func qualifiedStageLaserArtifact(device deviceexperience.Device) deviceupdate.ArtifactMetadata {
	return deviceupdate.ArtifactMetadata{
		ArtifactID:     "018f2744-0cb0-7bf6-9637-4a3a467a7a31",
		DeviceID:       device.ID,
		ProfileID:      device.ProfileID,
		Version:        "0.2.0",
		SourceRevision: strings.Repeat("a", 40),
		Qualification:  deviceupdate.QualificationQualified,
		SizeBytes:      123456,
		SHA256:         strings.Repeat("b", 64),
		CreatedAt:      time.Date(2026, 10, 7, 11, 0, 0, 0, time.UTC),
	}
}

func TestStageLaserFirmwareMaintenanceRequiresKnownStableOff(t *testing.T) {
	device := readyStageLaserDevice(t)
	if err := firmwareMaintenanceStateReady(device); err != nil {
		t.Fatalf("ready StageLaser rejected: %v", err)
	}

	var observation stagelaser.Observation
	if err := json.Unmarshal(device.Runtime.ObservedState, &observation); err != nil {
		t.Fatal(err)
	}
	observation.LogicalState = stagelaser.StateUnknown
	observation.StateQuality = stagelaser.StateQualityUnknown
	observation.ResyncRequired = true
	raw, _ := json.Marshal(observation)
	device.Runtime.ObservedState = raw
	if err := firmwareMaintenanceStateReady(device); err == nil {
		t.Fatal("UNKNOWN StageLaser unexpectedly accepted")
	}

	device = readyStageLaserDevice(t)
	_ = json.Unmarshal(device.Runtime.ObservedState, &observation)
	observation.ArmState = stagelaser.ArmArmed
	raw, _ = json.Marshal(observation)
	device.Runtime.ObservedState = raw
	if err := firmwareMaintenanceStateReady(device); err == nil {
		t.Fatal("ARMED StageLaser unexpectedly accepted")
	}
}

func TestStageLaserFirmwareManifestRequiresRollbackAndExactArtifact(t *testing.T) {
	device := readyStageLaserDevice(t)
	artifact := qualifiedStageLaserArtifact(device)
	now := time.Date(2026, 10, 7, 11, 30, 0, 0, time.UTC)

	manifest, err := buildStageDeviceFirmwareManifest(device, artifact, now)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.RollbackRequired {
		t.Fatal("StageLaser manifest did not require rollback")
	}
	if manifest.ArtifactPath != deviceupdate.ArtifactPath(artifact.ArtifactID) {
		t.Fatalf("artifact path = %q", manifest.ArtifactPath)
	}
	if manifest.ExpiresAt.Sub(manifest.IssuedAt) != operatorFirmwareManifestTTL {
		t.Fatalf("manifest TTL = %s", manifest.ExpiresAt.Sub(manifest.IssuedAt))
	}

	artifact.ProfileID = "stagecore.other"
	if _, err := buildStageDeviceFirmwareManifest(device, artifact, now); err == nil {
		t.Fatal("cross-profile artifact unexpectedly accepted")
	}
}

func TestFirmwareMaintenanceRequiresOnlineReady(t *testing.T) {
	device := readyStageLaserDevice(t)
	device.Runtime.Connection = deviceexperience.ConnectionOffline
	if err := firmwareMaintenanceStateReady(device); err == nil {
		t.Fatal("offline device unexpectedly accepted")
	}
}

type revokingFirmwareMaintenanceSender struct {
	updates   *deviceupdate.LifecycleStore
	auth      *userauth.Service
	token     string
	failure   error
	keepOwner bool
}

type unusedFirmwareMaintenanceSender struct{}

func (unusedFirmwareMaintenanceSender) SendFirmwareMaintenance(context.Context, string) (deviceupdate.UpdateRecord, error) {
	return deviceupdate.UpdateRecord{}, errors.New("firmware send was not expected in artifact upload test")
}

func TestFirmwareArtifactUploadSuppressesResultAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: "55555555-5555-4555-8555-555555555581", ProfileID: "stagecore.test-firmware",
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Firmware Upload Response Mock",
		Platform: "test", Architecture: "test", ClientVersion: "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	updates, err := deviceupdate.NewLifecycleStore(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("fixture-only firmware bytes")
	sum := sha256.Sum256(payload)
	expectedSHA := hex.EncodeToString(sum[:])
	ownerRevoked := false
	handler := New(withOperatorStageDeviceFirmware(h.auth, devices, artifacts, updates,
		unusedFirmwareMaintenanceSender{}, stageStore, nil, stageDeviceFirmwareHooks{
			BeforeArtifactResponse: func() {
				ownerRevoked = true
				if err := h.auth.Logout(ctx, owner.Token); err != nil {
					t.Errorf("revoke Owner before artifact response: %v", err)
				}
			},
		})).Handler()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"version": "0.2.0", "source_revision": strings.Repeat("a", 40),
		"sha256": expectedSHA, "qualification": deviceupdate.QualificationQualified,
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("firmware", "firmware.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/stage-devices/"+device.ID+"/firmware-artifacts", body)
	request.RemoteAddr = "127.0.0.1:19451"
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set(csrfHeader, owner.CSRFToken)
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !ownerRevoked {
		t.Fatal("test did not revoke Owner after artifact publication and before response")
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || result["error_code"] != capability.DispatchAuthorityRevokedCode ||
		result["effect_may_have_been_committed"] != true || result["retryable"] != false ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoked Owner received artifact upload result or retryable response: HTTP %d %+v", response.Code, result)
	}
	if _, leaked := result["artifact"]; leaked {
		t.Fatalf("revoked Owner received firmware artifact metadata after publication: %+v", result)
	}
	items, err := artifacts.ListForDevice(device.ID)
	if err != nil || len(items) != 1 || items[0].SHA256 != expectedSHA {
		t.Fatalf("artifact publication was not retained truthfully after response suppression: %+v err=%v", items, err)
	}
}

func TestFirmwareManifestSuppressesResultAfterOwnerRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "55555555-5555-4555-8555-555555555582"
	device, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, ProfileID: "stagecore.test-firmware", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Firmware Manifest Response Mock", Platform: "test", Architecture: "test",
		ClientVersion: "0.1.0", ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO stage_device_runtime_state
		(device_id, connection_state, readiness, last_seen_at_us, observed_state_json, network_state_json)
		VALUES (?, 'ONLINE', 'READY', ?, '{}', '{}')`, deviceID, time.Now().UnixMicro()); err != nil {
		t.Fatal(err)
	}
	device, err = devices.GetDevice(ctx, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("fixture-only firmware manifest bytes")
	sum := sha256.Sum256(payload)
	artifact, err := artifacts.ImportQualified(deviceupdate.ArtifactMetadata{
		ArtifactID: "55555555-5555-4555-8555-555555555583", DeviceID: device.ID, ProfileID: device.ProfileID,
		Version: "0.2.0", SourceRevision: strings.Repeat("a", 40), Qualification: deviceupdate.QualificationQualified,
		SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(sum[:]), OriginalFilename: "firmware.bin",
		CreatedAt: time.Now().UTC(),
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	updates, err := deviceupdate.NewLifecycleStore(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerRevoked := false
	handler := New(withOperatorStageDeviceFirmware(h.auth, devices, artifacts, updates,
		unusedFirmwareMaintenanceSender{}, stageStore, nil, stageDeviceFirmwareHooks{
			BeforeManifestResponse: func() {
				ownerRevoked = true
				if err := h.auth.Logout(ctx, owner.Token); err != nil {
					t.Errorf("revoke Owner before manifest response: %v", err)
				}
			},
		})).Handler()
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/stage-devices/"+device.ID+"/firmware-updates",
		[]byte(`{"artifact_id":"`+artifact.ArtifactID+`"}`), owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !ownerRevoked {
		t.Fatal("test did not revoke Owner after manifest issuance and before response")
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || result["error_code"] != capability.DispatchAuthorityRevokedCode ||
		result["effect_may_have_been_committed"] != true || result["retryable"] != false ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoked Owner received issued firmware manifest or retryable response: HTTP %d %+v", response.Code, result)
	}
	if _, leaked := result["manifest"]; leaked {
		t.Fatalf("revoked Owner received firmware manifest after issuance: %+v", result)
	}
	var count int
	if err := h.db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM stage_device_firmware_updates WHERE state='ISSUED'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("manifest issuance was not retained after response suppression: count=%d err=%v", count, err)
	}
}

func TestFirmwareArtifactListSuppressesDataAfterOwnerRevocation(t *testing.T) {
	ctx := context.Background()
	h := newAuthHarness(t)
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "55555555-5555-4555-8555-555555555584"
	device, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, ProfileID: "stagecore.test-firmware", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Firmware List Response Mock", Platform: "test", Architecture: "test",
		ClientVersion: "0.1.0", ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("fixture-only firmware list bytes")
	sum := sha256.Sum256(payload)
	if _, err := artifacts.ImportQualified(deviceupdate.ArtifactMetadata{
		ArtifactID: "55555555-5555-4555-8555-555555555585", DeviceID: device.ID, ProfileID: device.ProfileID,
		Version: "0.2.0", SourceRevision: strings.Repeat("a", 40), Qualification: deviceupdate.QualificationQualified,
		SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(sum[:]), OriginalFilename: "firmware.bin",
		CreatedAt: time.Now().UTC(),
	}, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	updates, err := deviceupdate.NewLifecycleStore(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerRevoked := false
	handler := New(withOperatorStageDeviceFirmware(h.auth, devices, artifacts, updates,
		unusedFirmwareMaintenanceSender{}, store.New(h.db.DB, clock.Real{}), nil, stageDeviceFirmwareHooks{
			BeforeArtifactListResponse: func() {
				ownerRevoked = true
				if err := h.auth.Logout(ctx, owner.Token); err != nil {
					t.Errorf("revoke Owner before artifact list response: %v", err)
				}
			},
		})).Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/stage-devices/"+device.ID+"/firmware-artifacts", nil)
	request.RemoteAddr = "127.0.0.1:19451"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !ownerRevoked {
		t.Fatal("test did not revoke Owner after reading artifact list and before response")
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || result["error_code"] != capability.DispatchAuthorityRevokedCode ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoked Owner received firmware artifact list: HTTP %d %+v", response.Code, result)
	}
	if _, leaked := result["artifacts"]; leaked {
		t.Fatalf("revoked Owner received firmware artifact metadata: %+v", result)
	}
}

func TestFirmwareUpdateStatusSuppressesDataAfterOwnerRevocation(t *testing.T) {
	ctx := context.Background()
	h := newAuthHarness(t)
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "55555555-5555-4555-8555-555555555586"
	device, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, ProfileID: "stagecore.test-firmware", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Firmware Status Response Mock", Platform: "test", Architecture: "test",
		ClientVersion: "0.1.0", ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	updates, err := deviceupdate.NewLifecycleStore(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildStageDeviceFirmwareManifest(device, deviceupdate.ArtifactMetadata{
		ArtifactID: "55555555-5555-4555-8555-555555555587", DeviceID: device.ID, ProfileID: device.ProfileID,
		Version: "0.2.0", SourceRevision: strings.Repeat("a", 40), Qualification: deviceupdate.QualificationQualified,
		SizeBytes: 32, SHA256: strings.Repeat("b", 64),
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updates.Issue(ctx, manifest, "fixture-owner"); err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerRevoked := false
	handler := New(withOperatorStageDeviceFirmware(h.auth, devices, artifacts, updates,
		unusedFirmwareMaintenanceSender{}, store.New(h.db.DB, clock.Real{}), nil, stageDeviceFirmwareHooks{
			BeforeUpdateStatusResponse: func() {
				ownerRevoked = true
				if err := h.auth.Logout(ctx, owner.Token); err != nil {
					t.Errorf("revoke Owner before firmware status response: %v", err)
				}
			},
		})).Handler()
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/stage-devices/"+device.ID+"/firmware-updates/"+manifest.UpdateID, nil)
	request.RemoteAddr = "127.0.0.1:19451"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !ownerRevoked {
		t.Fatal("test did not revoke Owner after reading update status and before response")
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusForbidden || result["error_code"] != capability.DispatchAuthorityRevokedCode ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoked Owner received firmware update status: HTTP %d %+v", response.Code, result)
	}
	if _, leaked := result["update"]; leaked {
		t.Fatalf("revoked Owner received firmware update record: %+v", result)
	}
}

func (s revokingFirmwareMaintenanceSender) SendFirmwareMaintenance(ctx context.Context, updateID string) (deviceupdate.UpdateRecord, error) {
	if err := capability.CheckDispatch(ctx); err != nil {
		return deviceupdate.UpdateRecord{}, err
	}
	record, err := s.updates.MarkSent(ctx, updateID, "55555555-5555-4555-8555-555555555599", 1)
	if err != nil {
		return deviceupdate.UpdateRecord{}, err
	}
	// This represents a revocation racing immediately after the transport
	// accepted the command. The accepted firmware effect cannot be recalled.
	if !s.keepOwner {
		if err := s.auth.Logout(ctx, s.token); err != nil {
			return deviceupdate.UpdateRecord{}, err
		}
	}
	if s.failure != nil {
		return record, s.failure
	}
	return record, nil
}

func TestFirmwareSendDoesNotDeliverSuccessAfterOwnerSessionRevokedPostWrite(t *testing.T) {
	ctx := context.Background()
	h := newAuthHarness(t)
	stageStore := store.New(h.db.DB, clock.Real{})
	const deviceID = "55555555-5555-4555-8555-555555555599"

	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device := deviceexperience.Device{
		ID: deviceID, ProfileID: "stagecore.test-firmware", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Firmware Boundary Mock", Platform: "test", Architecture: "test",
		ClientVersion: "0.1.0", ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: []string{deviceupdate.FirmwareMaintenanceCapability}, Enabled: true,
	}
	if _, err := devices.RegisterUnassignedV2(ctx, device); err != nil {
		t.Fatal(err)
	}
	// Set a test-only ready state so this endpoint reaches the mocked send
	// boundary; no real device or runtime is involved.
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO stage_device_runtime_state
		(device_id, connection_state, readiness, last_seen_at_us, observed_state_json, network_state_json)
		VALUES (?, 'ONLINE', 'READY', ?, '{}', '{}')`, deviceID, time.Now().UnixMicro()); err != nil {
		t.Fatal(err)
	}
	device, err = devices.GetDevice(ctx, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	updates, err := deviceupdate.NewLifecycleStore(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildStageDeviceFirmwareManifest(device, qualifiedStageLaserArtifact(device), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updates.Issue(ctx, manifest, "test-owner"); err != nil {
		t.Fatal(err)
	}
	artifacts, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	fakeRuntime := revokingFirmwareMaintenanceSender{updates: updates, auth: h.auth, token: owner.Token}
	handler := New(WithOperatorStageDeviceFirmware(h.auth, devices, artifacts, updates, fakeRuntime, stageStore, nil)).Handler()
	request := authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/stage-devices/"+deviceID+"/firmware-updates/"+manifest.UpdateID+"/send",
		nil, owner.Token, owner.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "DISPATCH_AUTHORITY_REVOKED") ||
		response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoked Owner received firmware send success: HTTP %d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["effect_may_be_committed"] != true || body["effect_may_have_been_committed"] != true ||
		body["effect_may_have_been_sent"] != true || body["retryable"] != false {
		t.Fatalf("post-write revocation response must report irreversible boundary and prohibit retry: %+v", body)
	}
	if _, err := h.auth.ValidateCSRF(ctx, owner.Token, owner.CSRFToken); !errors.Is(err, userauth.ErrSessionInvalid) {
		t.Fatalf("test sender did not revoke Owner session: %v", err)
	}
	record, err := updates.Get(ctx, manifest.UpdateID)
	if err != nil || record.State != deviceupdate.UpdateSent {
		t.Fatalf("already transmitted firmware effect was not truthfully retained as SENT: %+v err=%v", record, err)
	}
}

func TestFirmwareSendReportsUncertainOutcomeAndFencesRevocation(t *testing.T) {
	for _, tc := range []struct {
		name           string
		revokeOwner    bool
		wantStatusCode int
	}{
		{name: "revoked_owner", revokeOwner: true, wantStatusCode: http.StatusForbidden},
		{name: "still_authorized_owner", wantStatusCode: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := newAuthHarness(t)
			stageStore := store.New(h.db.DB, clock.Real{})
			const deviceID = "55555555-5555-4555-8555-555555555599"
			devices, err := deviceexperience.NewRepository(h.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			device := deviceexperience.Device{
				ID: deviceID, ProfileID: "stagecore.test-firmware", Kind: deviceexperience.DeviceGeneric,
				DisplayName: "Firmware Error Boundary Mock", Platform: "test", Architecture: "test",
				ClientVersion: "0.1.0", ProtocolVersion: deviceexperience.ProtocolVersion2,
				Capabilities: []string{deviceupdate.FirmwareMaintenanceCapability}, Enabled: true,
			}
			if _, err := devices.RegisterUnassignedV2(ctx, device); err != nil {
				t.Fatal(err)
			}
			if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO stage_device_runtime_state
		(device_id, connection_state, readiness, last_seen_at_us, observed_state_json, network_state_json)
		VALUES (?, 'ONLINE', 'READY', ?, '{}', '{}')`, deviceID, time.Now().UnixMicro()); err != nil {
				t.Fatal(err)
			}
			device, err = devices.GetDevice(ctx, deviceID)
			if err != nil {
				t.Fatal(err)
			}
			updates, err := deviceupdate.NewLifecycleStore(h.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := buildStageDeviceFirmwareManifest(device, qualifiedStageLaserArtifact(device), time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := updates.Issue(ctx, manifest, "test-owner"); err != nil {
				t.Fatal(err)
			}
			artifacts, err := deviceupdate.NewArtifactRegistry(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			const privateDetail = "private simulated transport failure"
			fakeRuntime := revokingFirmwareMaintenanceSender{
				updates: updates, auth: h.auth, token: owner.Token, keepOwner: !tc.revokeOwner,
				failure: fmt.Errorf("%w: %s", devicechannel.ErrFirmwareMaintenanceOutcomeUncertain, privateDetail),
			}
			handler := New(WithOperatorStageDeviceFirmware(h.auth, devices, artifacts, updates, fakeRuntime, stageStore, nil)).Handler()
			request := authenticatedMutationRequest(t, http.MethodPost,
				"/api/v1/stage-devices/"+deviceID+"/firmware-updates/"+manifest.UpdateID+"/send",
				nil, owner.Token, owner.CSRFToken)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != tc.wantStatusCode {
				t.Fatalf("firmware error HTTP status = %d, want %d: %s", response.Code, tc.wantStatusCode, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("firmware error response is cacheable: headers=%v", response.Header())
			}
			if tc.revokeOwner {
				if !strings.Contains(response.Body.String(), capability.DispatchAuthorityRevokedCode) || strings.Contains(response.Body.String(), privateDetail) {
					t.Fatalf("revoked Owner received stale firmware error details: %s", response.Body.String())
				}
			} else if !strings.Contains(response.Body.String(), privateDetail) {
				t.Fatalf("authorized Owner did not receive the current firmware error: %s", response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["effect_may_be_committed"] != true || body["effect_may_have_been_committed"] != true ||
				body["effect_may_have_been_sent"] != true || body["retryable"] != false {
				t.Fatalf("uncertain firmware send was not reported safely: %+v", body)
			}
		})
	}
}

func TestQualifiedFirmwareArtifactUploadRBACCSRFAndIntegrity(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})

	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device := deviceexperience.Device{
		ID:              "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ProfileID:       stagelaser.ProfileID,
		Kind:            deviceexperience.DeviceGeneric,
		DisplayName:     "StageLaser Upload Test",
		Platform:        "esp32",
		Architecture:    "riscv32",
		ClientVersion:   "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities:    stagelaser.CapabilityKeys(),
		Enabled:         true,
	}
	device, err = devices.RegisterUnassignedV2(ctx, device)
	if err != nil {
		t.Fatal(err)
	}

	artifacts, err := deviceupdate.NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	updates, err := deviceupdate.NewLifecycleStore(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(
		devices,
		nil,
		devicechannel.WithFirmwareMaintenance(updates),
	)
	defer runtime.Close()

	handler := New(WithOperatorStageDeviceFirmware(
		h.auth,
		devices,
		artifacts,
		updates,
		runtime,
		stageStore,
		nil,
	)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.auth.CreateUser(
		ctx,
		"firmware-viewer",
		"firmware viewer password",
		userauth.RoleViewer,
	); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(
		ctx,
		"firmware-viewer",
		"firmware viewer password",
		"127.0.0.2",
	)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("qualified-stage-device-firmware")
	sum := sha256.Sum256(payload)
	expectedSHA := hex.EncodeToString(sum[:])
	sourceRevision := strings.Repeat("a", 40)
	path := "/api/v1/stage-devices/" + device.ID + "/firmware-artifacts"

	makeUpload := func(shaValue, qualification string) (*bytes.Buffer, string) {
		t.Helper()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for key, value := range map[string]string{
			"version":         "0.2.0",
			"source_revision": sourceRevision,
			"sha256":          shaValue,
			"qualification":   qualification,
		} {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		part, err := writer.CreateFormFile("firmware", "firmware.bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return body, writer.FormDataContentType()
	}
	doUpload := func(
		credentialToken, csrf, shaValue, qualification string,
	) *httptest.ResponseRecorder {
		t.Helper()
		body, contentType := makeUpload(shaValue, qualification)
		req := httptest.NewRequest(http.MethodPost, path, body)
		req.RemoteAddr = "127.0.0.1:19901"
		req.Header.Set("Content-Type", contentType)
		if csrf != "" {
			req.Header.Set(csrfHeader, csrf)
		}
		req.AddCookie(&http.Cookie{
			Name:  browserSessionCookie,
			Value: credentialToken,
		})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	t.Run("revoked_owner_gets_no_hash_details_after_upload_validation_error", func(t *testing.T) {
		revokedOwner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.9")
		if err != nil {
			t.Fatal(err)
		}
		body, contentType := makeUpload(strings.Repeat("0", 64), deviceupdate.QualificationQualified)
		request := httptest.NewRequest(http.MethodPost, path, body)
		request.RemoteAddr = "127.0.0.9:19901"
		request.Header.Set("Content-Type", contentType)
		request.Header.Set(csrfHeader, revokedOwner.CSRFToken)
		request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: revokedOwner.Token})
		request.Body = &firmwareUploadRevokeReader{ReadCloser: request.Body, revoke: func() {
			if err := h.auth.Logout(ctx, revokedOwner.Token); err != nil {
				t.Errorf("revoke Owner during firmware upload: %v", err)
			}
		}}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden ||
			!strings.Contains(response.Body.String(), capability.DispatchAuthorityRevokedCode) ||
			strings.Contains(response.Body.String(), "actual_sha256") {
			t.Fatalf("revoked Owner received firmware validation details: HTTP %d %s", response.Code, response.Body.String())
		}
	})

	missingCSRF := doUpload(owner.Token, "", expectedSHA, deviceupdate.QualificationQualified)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf(
			"missing CSRF status=%d want=403 body=%s",
			missingCSRF.Code,
			missingCSRF.Body.String(),
		)
	}

	viewerUpload := doUpload(
		viewer.Token,
		viewer.CSRFToken,
		expectedSHA,
		deviceupdate.QualificationQualified,
	)
	if viewerUpload.Code != http.StatusForbidden {
		t.Fatalf(
			"VIEWER upload status=%d want=403 body=%s",
			viewerUpload.Code,
			viewerUpload.Body.String(),
		)
	}

	badSHA := doUpload(
		owner.Token,
		owner.CSRFToken,
		strings.Repeat("b", 64),
		deviceupdate.QualificationQualified,
	)
	if badSHA.Code != http.StatusBadRequest ||
		!strings.Contains(
			badSHA.Body.String(),
			"STAGE_DEVICE_FIRMWARE_SHA256_MISMATCH",
		) {
		t.Fatalf("bad SHA status=%d body=%s", badSHA.Code, badSHA.Body.String())
	}

	notQualified := doUpload(
		owner.Token,
		owner.CSRFToken,
		expectedSHA,
		"UNQUALIFIED",
	)
	if notQualified.Code != http.StatusBadRequest ||
		!strings.Contains(
			notQualified.Body.String(),
			"STAGE_DEVICE_FIRMWARE_QUALIFICATION_REQUIRED",
		) {
		t.Fatalf(
			"unqualified upload status=%d body=%s",
			notQualified.Code,
			notQualified.Body.String(),
		)
	}

	success := doUpload(
		owner.Token,
		owner.CSRFToken,
		expectedSHA,
		deviceupdate.QualificationQualified,
	)
	if success.Code != http.StatusCreated {
		t.Fatalf("qualified upload status=%d body=%s", success.Code, success.Body.String())
	}
	var response struct {
		Artifact     deviceupdate.ArtifactMetadata `json:"artifact"`
		ArtifactPath string                        `json:"artifact_path"`
	}
	if err := json.Unmarshal(success.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Artifact.DeviceID != device.ID ||
		response.Artifact.ProfileID != device.ProfileID ||
		response.Artifact.Version != "0.2.0" ||
		response.Artifact.SourceRevision != sourceRevision ||
		response.Artifact.SHA256 != expectedSHA ||
		response.Artifact.Qualification != deviceupdate.QualificationQualified {
		t.Fatalf("artifact=%+v", response.Artifact)
	}
	if response.ArtifactPath != deviceupdate.ArtifactPath(response.Artifact.ArtifactID) {
		t.Fatalf("artifact_path=%q", response.ArtifactPath)
	}

	file, metadata, err := artifacts.OpenForDevice(
		response.Artifact.ArtifactID,
		device.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stored, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, payload) || metadata.SHA256 != expectedSHA {
		t.Fatalf("stored artifact integrity mismatch metadata=%+v", metadata)
	}

	body, contentType := makeUpload(expectedSHA, deviceupdate.QualificationQualified)
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.RemoteAddr = "127.0.0.1:19901"
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(csrfHeader, owner.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	req.Body = &firmwareUploadRevokeReader{ReadCloser: req.Body, revoke: func() {
		if err := h.auth.Logout(ctx, owner.Token); err != nil {
			t.Fatal(err)
		}
	}}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, req)
	if result.Code != http.StatusForbidden || !strings.Contains(result.Body.String(), "DISPATCH_AUTHORITY_REVOKED") {
		t.Fatalf("revoked upload returned HTTP %d, %s", result.Code, result.Body.String())
	}
	items, err := artifacts.ListForDevice(device.ID)
	if err != nil || len(items) != 1 || items[0].ArtifactID != response.Artifact.ArtifactID {
		t.Fatalf("revoked upload affected the firmware registry: %d artifacts, err=%v", len(items), err)
	}

	// Seed an already-qualified runtime observation in the isolated fixture.
	ready := readyStageLaserDevice(t).Runtime
	if _, err := devices.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: device.ID, Connection: ready.Connection, Readiness: ready.Readiness, ObservedState: ready.ObservedState,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, "UPDATE stage_device_runtime_state SET readiness = ? WHERE device_id = ?", ready.Readiness, device.ID); err != nil {
		t.Fatal(err)
	}
	owner, err = h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	issueManifest := func(revoke bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost,
			"/api/v1/stage-devices/"+device.ID+"/firmware-updates",
			strings.NewReader(`{"artifact_id":"`+response.Artifact.ArtifactID+`"}`))
		req.RemoteAddr = "127.0.0.1:19901"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, owner.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		if revoke {
			req.Body = &firmwareUploadRevokeReader{ReadCloser: req.Body, revoke: func() {
				if err := h.auth.Logout(ctx, owner.Token); err != nil {
					t.Fatal(err)
				}
			}}
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	if issued := issueManifest(false); issued.Code != http.StatusCreated {
		t.Fatalf("active session could not issue manifest: %d %s", issued.Code, issued.Body.String())
	}
	if revoked := issueManifest(true); revoked.Code != http.StatusForbidden ||
		!strings.Contains(revoked.Body.String(), "DISPATCH_AUTHORITY_REVOKED") {
		t.Fatalf("revoked session issued manifest: %d %s", revoked.Code, revoked.Body.String())
	}
	var count int
	if err := h.db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM stage_device_firmware_updates").Scan(&count); err != nil || count != 1 {
		t.Fatalf("revocation changed persisted firmware updates: %d, err=%v", count, err)
	}
}

type firmwareUploadRevokeReader struct {
	io.ReadCloser
	revoke func()
}

func (r *firmwareUploadRevokeReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 && r.revoke != nil {
		r.revoke()
		r.revoke = nil
	}
	return n, err
}
