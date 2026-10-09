package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorLightingCommissioningUsesPublishedConfigAndLogicalIdentify(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Commissioning", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting 01",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
		ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm",
		Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
	}}}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID,
		Configuration: config, Aliases: map[string]string{"front_warm": "warm_a"},
	}, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingCommissioning(h.auth, devices, runtime, stageStore)).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/lighting-controller/nodes/lighting-01/apply-published-config", bytes.NewBufferString(`{}`))
	req.RemoteAddr = "127.0.0.1:19505"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("apply status=%d body=%s", res.Code, res.Body.String())
	}
	var apply struct {
		Command deviceexperience.DeviceCommand `json:"command"`
		RuntimeSnapshotID string `json:"runtime_snapshot_id"`
		ExpectedHash string `json:"expected_configuration_hash"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &apply); err != nil {
		t.Fatal(err)
	}
	if apply.RuntimeSnapshotID != published.ID || apply.Command.Envelope.CommandType != lightingnode.CommandConfigApply {
		t.Fatalf("apply=%+v", apply)
	}
	var applyPayload lightingnode.ConfigApplyPayload
	if err := json.Unmarshal(apply.Command.Envelope.Payload, &applyPayload); err != nil {
		t.Fatal(err)
	}
	if len(applyPayload.Configuration.Channels) != 1 || applyPayload.Configuration.Channels[0].ChannelKey != "warm_a" {
		t.Fatalf("apply payload=%+v", applyPayload)
	}
	hash, _ := lightingnode.ConfigurationHash(config)
	if apply.ExpectedHash != hash {
		t.Fatalf("expected hash=%q want=%q", apply.ExpectedHash, hash)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/lighting-controller/nodes/lighting-01/identify", bytes.NewBufferString(`{"alias":"front_warm","level":40,"duration_ms":900}`))
	req.RemoteAddr = "127.0.0.1:19506"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("identify status=%d body=%s", res.Code, res.Body.String())
	}
	var identify struct {
		Command deviceexperience.DeviceCommand `json:"command"`
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &identify); err != nil {
		t.Fatal(err)
	}
	if identify.Alias != "front_warm" || identify.Command.Envelope.CommandType != lightingnode.CommandIdentify {
		t.Fatalf("identify=%+v", identify)
	}
	var identifyPayload lightingnode.IdentifyPayload
	if err := json.Unmarshal(identify.Command.Envelope.Payload, &identifyPayload); err != nil {
		t.Fatal(err)
	}
	if identifyPayload.ChannelKey != "warm_a" || identifyPayload.Level != 40 || identifyPayload.DurationMS != 900 {
		t.Fatalf("identify payload=%+v", identifyPayload)
	}
}

func TestLightingCommissioningSnapshotUsesV2AssignmentInsteadOfLegacyProject(t *testing.T) {
	projectID := "project-owner"
	d := deviceexperience.Device{
		ID: "dmx-v2-test", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
		Assignment: &deviceexperience.AssignmentRecord{
			State: "ACTIVE", ProjectID: projectID,
			RuntimeSnapshotID: "published-snapshot", Epoch: 9,
		},
	}
	id, ok := lightingCommissioningSnapshot(d, projectID)
	if !ok || id != "published-snapshot" {
		t.Fatalf("ACTIVE projectless v2 device must resolve its committed snapshot, got %q ok=%v", id, ok)
	}
	if _, ok := lightingCommissioningSnapshot(d, "other-project"); ok {
		t.Fatal("cannot identify a v2 node through another Project")
	}
	d.Assignment.State = "UNASSIGNED"
	if _, ok := lightingCommissioningSnapshot(d, projectID); ok {
		t.Fatal("UNASSIGNED must not authorize identify")
	}
	d.Assignment.State = "BLOCKED"
	if _, ok := lightingCommissioningSnapshot(d, projectID); ok {
		t.Fatal("BLOCKED must not authorize identify")
	}
	d.Assignment.State = "ACTIVE"
	d.Assignment.RuntimeSnapshotID = ""
	if _, ok := lightingCommissioningSnapshot(d, projectID); ok {
		t.Fatal("ACTIVE without published-snapshot authority must not authorize identify")
	}
	d.Assignment.RuntimeSnapshotID = "published-snapshot"
	d.ProjectID = "stale-legacy-project"
	if _, ok := lightingCommissioningSnapshot(d, projectID); ok {
		t.Fatal("projectless v2 node must not use a legacy Project row")
	}
	d.ProjectID = ""
	d.Enabled = false
	if _, ok := lightingCommissioningSnapshot(d, projectID); ok {
		t.Fatal("disabled v2 lighting node must not authorize identify")
	}
}

func TestLightingCommissioningV2RoutesRequireAuthenticatedActiveSocket(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "V2 commissioning scope gate", CreatedBy: "owner"})
	if err != nil { t.Fatal(err) }
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil { t.Fatal(err) }
	const deviceID = "dmx-commissioning-v2"
	// Create the original Project-scoped binding through the supported v1
	// authoring API, then migrate the real identity to projectless v2.
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: deviceID, ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "V2 Lighting Node",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil { t.Fatal(err) }
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
		ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm",
		Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
	}}}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: deviceID, ProfileID: lightingnode.ProfileID,
		Configuration: config, Aliases: map[string]string{"front_warm": "warm_a"},
	}, "owner"); err != nil { t.Fatal(err) }
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil { t.Fatal(err) }
	if _, err := devices.MigrateLegacyLightingToV2Blocked(ctx, deviceID, project.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	handler := New(WithOperatorLightingCommissioning(h.auth, devices, runtime, stageStore)).Handler()
	request := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.RemoteAddr = "127.0.0.1:19555"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, credential.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	identifyPath := "/api/v1/projects/"+project.ID+"/lighting-controller/nodes/"+deviceID+"/identify"
	applyPath := "/api/v1/projects/"+project.ID+"/lighting-controller/nodes/"+deviceID+"/apply-published-config"
	if res := request(identifyPath, `{"alias":"front_warm","level":5,"duration_ms":500}`); res.Code != http.StatusNotFound {
		t.Fatalf("BLOCKED identify must be denied, status=%d body=%s", res.Code, res.Body.String())
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments SET assignment_state='ACTIVE',
		project_id=?, runtime_snapshot_id=?, assignment_epoch=9 WHERE device_id=?
	`, project.ID, published.ID, deviceID); err != nil { t.Fatal(err) }
	if res := request(identifyPath, `{"alias":"front_warm","level":5,"duration_ms":500}`); res.Code != http.StatusConflict ||
		!bytes.Contains(res.Body.Bytes(), []byte("LIGHTING_DEVICE_SCOPE_NOT_READY")) {
		t.Fatalf("ACTIVE without live v2 scope must be 409 not 404 or send an output, status=%d body=%s", res.Code, res.Body.String())
	}
	if res := request(applyPath, "{}"); res.Code != http.StatusConflict ||
		!bytes.Contains(res.Body.Bytes(), []byte("LIGHTING_DEVICE_SCOPE_NOT_READY")) {
		t.Fatalf("apply must require live authenticated v2 scope, status=%d body=%s", res.Code, res.Body.String())
	}
	if res := request("/api/v1/projects/other-project/lighting-controller/nodes/"+deviceID+"/identify", `{"alias":"front_warm","level":5}`); res.Code != http.StatusNotFound {
		t.Fatalf("wrong project must be denied, status=%d body=%s", res.Code, res.Body.String())
	}
	var count int
	if err := h.db.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM stage_device_commands WHERE device_id=?", deviceID).Scan(&count); err != nil { t.Fatal(err) }
	if count != 0 { t.Fatalf("unauthorized commissioning emitted %d commands", count) }
}
