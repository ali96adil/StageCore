package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorTabletAuthoringProjectsCanonicalCueBackToDeviceScene(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet Scenes", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "tablet-01", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Actor One", Platform: "android", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{deviceexperience.CapabilityTabletPlay}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	alias, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "tablet.tablet-01.play", LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef: "tablet-01", ProjectConfig: json.RawMessage(`{"device_id":"tablet-01","capability_key":"tablet.media.play"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "T1", Name: "Entrance Video", OrderIndex: 3,
		CueType: tabletSceneCueType, Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "PARALLEL_BARRIER", TargetRef: alias.LogicalName,
		CapabilityKey: deviceexperience.CapabilityTabletPlay, Parameters: json.RawMessage(`{"media_number":7}`),
		TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorTabletAuthoring(h.auth, devices, stageStore)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/tablet-controller/scenes", nil)
	req.RemoteAddr = "127.0.0.1:19301"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Scenes []tabletSceneView `json:"scenes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Scenes) != 1 {
		t.Fatalf("scenes=%+v", response.Scenes)
	}
	scene := response.Scenes[0]
	if scene.CueID != created.ID || scene.Name != "Entrance Video" || scene.OrderIndex != 3 || len(scene.Actions) != 1 {
		t.Fatalf("scene=%+v", scene)
	}
	action := scene.Actions[0]
	if action.DeviceID != "tablet-01" || action.DisplayName != "Actor One" || action.CommandType != deviceexperience.CommandTabletPlay {
		t.Fatalf("action=%+v", action)
	}
	var parameters map[string]any
	if err := json.Unmarshal(action.Parameters, &parameters); err != nil {
		t.Fatal(err)
	}
	if parameters["media_number"] != float64(7) {
		t.Fatalf("parameters=%+v", parameters)
	}
}

func TestOperatorTabletAuthoringExcludesMixedCue(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Mixed Cue", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, Name: "Mixed", OrderIndex: 0, CueType: tabletSceneCueType, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, TargetRef: "lighting.front", CapabilityKey: "dmx.level", Parameters: json.RawMessage(`{"level":50}`), Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorTabletAuthoring(h.auth, devices, stageStore)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/tablet-controller/scenes", nil)
	req.RemoteAddr = "127.0.0.1:19302"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Scenes []tabletSceneView `json:"scenes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Scenes) != 0 {
		t.Fatalf("mixed cue leaked into Tablet Scene editor: %+v", response.Scenes)
	}
}
