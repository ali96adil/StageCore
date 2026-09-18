package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorLightingConfigurationSavesAndReportsMatchingHealth(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Setup", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Front Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingController(h.auth, devices, stageStore)).Handler()
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{
		{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Front Warm", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
		{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Front Cold", Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
	}}
	body, _ := json.Marshal(lightingConfigurationRequest{
		Configuration: config,
		Aliases: map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID+"/lighting-controller/configuration/lighting-01", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:19501"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", res.Code, res.Body.String())
	}

	hash, err := lightingnode.ConfigurationHash(config)
	if err != nil {
		t.Fatal(err)
	}
	observedBytes, _ := json.Marshal(lightingnode.Observation{
		SchemaVersion: lightingnode.SchemaVersion1,
		CurrentLevels: map[string]float64{"warm_a": 0, "cold_a": 0},
		DMXHealthy: true, ConfigurationHash: hash, Authority: lightingnode.AuthorityStageCore,
	})
	if _, err := devices.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: "lighting-01", Connection: deviceexperience.ConnectionOnline,
		Readiness: deviceexperience.ReadinessReady, ObservedState: observedBytes,
	}); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/lighting-controller/configuration", nil)
	req.RemoteAddr = "127.0.0.1:19502"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		Nodes []lightingConfigurationNodeView `json:"nodes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Nodes) != 1 {
		t.Fatalf("nodes=%+v", payload.Nodes)
	}
	node := payload.Nodes[0]
	if !node.Configured || node.Health.Status != "READY" || node.Health.ConfigurationMatches == nil || !*node.Health.ConfigurationMatches {
		t.Fatalf("node health=%+v node=%+v", node.Health, node)
	}
	if node.Health.ExpectedConfigHash != hash || node.Health.ObservedConfigHash != hash || node.Observed == nil || !node.Observed.DMXHealthy {
		t.Fatalf("node=%+v", node)
	}
}

func TestOperatorLightingConfigurationWriteBlockedDuringShow(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting SHOW Lock", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	config := lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
		ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm", Kind: lightingnode.ChannelWarmWhite,
		MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
	}}}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID, Configuration: config,
		Aliases: map[string]string{"front_warm": "warm_a"},
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
	show, err := stageStore.CreateSession(ctx, published.ID, domain.SessionShow, "Lighting lock")
	if err != nil {
		t.Fatal(err)
	}
	defer stageStore.EndSession(ctx, show.ID, domain.SessionCompleted)

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingController(h.auth, devices, stageStore)).Handler()
	body, _ := json.Marshal(lightingConfigurationRequest{
		Configuration: config, Aliases: map[string]string{"front_warm": "warm_a"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID+"/lighting-controller/configuration/lighting-01", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:19503"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusLocked {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
