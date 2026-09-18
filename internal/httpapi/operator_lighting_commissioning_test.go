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
