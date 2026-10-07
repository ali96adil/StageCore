package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		SchemaVersion: stagelaser.SchemaVersion1,
		FirmwareVersion: "0.1.0",
		ControlContractVersion: stagelaser.ControlContractVersion,
		ArmState: stagelaser.ArmDisarmed,
		LogicalState: stagelaser.StateOff,
		StateQuality: stagelaser.StateQualityTracked,
		DriverKind: stagelaser.DriverMechanicalRelay,
		Limits: stagelaser.DefaultMechanicalLimits(),
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	return deviceexperience.Device{
		ID: "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ProfileID: stagelaser.ProfileID,
		Kind: deviceexperience.DeviceGeneric,
		ClientVersion: "0.1.0",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled: true,
		Runtime: &deviceexperience.RuntimeState{
			Connection: deviceexperience.ConnectionOnline,
			Readiness: deviceexperience.ReadinessReady,
			ObservedState: raw,
		},
	}
}

func qualifiedStageLaserArtifact(device deviceexperience.Device) deviceupdate.ArtifactMetadata {
	return deviceupdate.ArtifactMetadata{
		ArtifactID: "018f2744-0cb0-7bf6-9637-4a3a467a7a31",
		DeviceID: device.ID,
		ProfileID: device.ProfileID,
		Version: "0.2.0",
		SourceRevision: strings.Repeat("a", 40),
		Qualification: deviceupdate.QualificationQualified,
		SizeBytes: 123456,
		SHA256: strings.Repeat("b", 64),
		CreatedAt: time.Date(2026, 10, 7, 11, 0, 0, 0, time.UTC),
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
			"version": "0.2.0",
			"source_revision": sourceRevision,
			"sha256": shaValue,
			"qualification": qualification,
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
			Name: browserSessionCookie,
			Value: credentialToken,
		})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

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
		Artifact deviceupdate.ArtifactMetadata `json:"artifact"`
		ArtifactPath string `json:"artifact_path"`
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
}
