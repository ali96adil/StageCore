package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorLightingCueActionsGroupAliasesPerNode(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Builder", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range []deviceexperience.Device{
		{
			ID: "lighting-a", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
			Kind: deviceexperience.DeviceGeneric, DisplayName: "Front Node",
			ProtocolVersion: deviceexperience.ProtocolVersion1,
			Capabilities: []string{lightingnode.CapabilityChannelsFade, lightingnode.CapabilityChannelsSet, lightingnode.CapabilityBlackout},
			Enabled: true,
		},
		{
			ID: "lighting-b", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
			Kind: deviceexperience.DeviceGeneric, DisplayName: "Back Node",
			ProtocolVersion: deviceexperience.ProtocolVersion1,
			Capabilities: []string{lightingnode.CapabilityChannelsFade, lightingnode.CapabilityChannelsSet, lightingnode.CapabilityBlackout},
			Enabled: true,
		},
	} {
		if _, err := devices.UpsertDevice(ctx, device); err != nil {
			t.Fatal(err)
		}
	}
	bindings := []lightingnode.ProjectBinding{
		{
			DeviceID: "lighting-a", ProfileID: lightingnode.ProfileID,
			Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{
				{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Front Warm", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
				{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Front Cold", Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
			}},
			Aliases: map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
		},
		{
			DeviceID: "lighting-b", ProfileID: lightingnode.ProfileID,
			Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{
				{ChannelKey: "back_a", ChannelNumber: 1, DisplayName: "Back Warm", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
			}},
			Aliases: map[string]string{"back_warm": "back_a"},
		},
	}
	for _, binding := range bindings {
		if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, binding, "owner"); err != nil {
			t.Fatal(err)
		}
	}

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingController(h.auth, devices, stageStore)).Handler()

	body := bytes.NewBufferString(`{
		"command_type":"LIGHTING_CHANNELS_FADE",
		"fade_ms":1200,
		"levels":{"front_warm":55,"front_cold":20,"back_warm":35}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/lighting-controller/cue-actions", body)
	req.RemoteAddr = "127.0.0.1:19401"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Actions []lightingCueActionDescriptor `json:"actions"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Actions) != 2 {
		t.Fatalf("actions=%+v", response.Actions)
	}
	sort.Slice(response.Actions, func(i, j int) bool { return response.Actions[i].DeviceID < response.Actions[j].DeviceID })
	if response.Actions[0].DeviceID != "lighting-a" || response.Actions[1].DeviceID != "lighting-b" {
		t.Fatalf("grouped actions=%+v", response.Actions)
	}
	for _, action := range response.Actions {
		if action.CapabilityKey != lightingnode.CapabilityChannelsFade || action.ExecutionMode != "PARALLEL_BARRIER" || action.Priority != "P1" {
			t.Fatalf("action=%+v", action)
		}
		var timeout struct {
			TimeoutMS int64 `json:"timeout_ms"`
		}
		if err := json.Unmarshal(action.TimeoutPolicy, &timeout); err != nil {
			t.Fatal(err)
		}
		if timeout.TimeoutMS != 4200 {
			t.Fatalf("timeout=%+v", timeout)
		}
		if strings.Contains(string(action.Parameters), "warm_a") || strings.Contains(string(action.Parameters), "cold_a") || strings.Contains(string(action.Parameters), "back_a") {
			t.Fatalf("raw channel key leaked into Cue parameters: %s", action.Parameters)
		}
	}

	var front lightingnode.CueFadePayload
	if err := json.Unmarshal(response.Actions[0].Parameters, &front); err != nil {
		t.Fatal(err)
	}
	if front.FadeMS != 1200 || len(front.Aliases) != 2 || front.Aliases["front_warm"] != 55 || front.Aliases["front_cold"] != 20 {
		t.Fatalf("front payload=%+v", front)
	}

	aliases, err := stageStore.ListAliases(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 2 {
		t.Fatalf("target aliases=%+v", aliases)
	}
	for _, alias := range aliases {
		if alias.LogicalType != devicechannel.StageDeviceLogicalType || !strings.HasPrefix(alias.LogicalName, "lighting.") {
			t.Fatalf("alias=%+v", alias)
		}
	}
}

func TestOperatorLightingCueProjectionRecombinesCanonicalNodeActions(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Projection", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lighting-a", "lighting-b"} {
		if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
			ID: id, ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
			Kind: deviceexperience.DeviceGeneric, DisplayName: id,
			ProtocolVersion: deviceexperience.ProtocolVersion1,
			Capabilities: []string{lightingnode.CapabilityChannelsFade}, Enabled: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []lightingnode.ProjectBinding{
		{
			DeviceID: "lighting-a", ProfileID: lightingnode.ProfileID,
			Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm A", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true}}},
			Aliases: map[string]string{"front_warm": "warm_a"},
		},
		{
			DeviceID: "lighting-b", ProfileID: lightingnode.ProfileID,
			Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{ChannelKey: "warm_b", ChannelNumber: 1, DisplayName: "Warm B", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true}}},
			Aliases: map[string]string{"back_warm": "warm_b"},
		},
	} {
		if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, binding, "owner"); err != nil {
			t.Fatal(err)
		}
	}
	for _, target := range []struct {
		name, device string
	}{
		{"lighting.a.fade", "lighting-a"},
		{"lighting.b.fade", "lighting-b"},
	} {
		cfg, _ := json.Marshal(map[string]string{"device_id": target.device, "capability_key": lightingnode.CapabilityChannelsFade})
		if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
			ProjectID: project.ID, LogicalName: target.name, LogicalType: devicechannel.StageDeviceLogicalType,
			TargetRef: target.device, ProjectConfig: cfg,
		}); err != nil {
			t.Fatal(err)
		}
	}
	created, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "LX1", Name: "Cool Front / Warm Back", OrderIndex: 4,
		CueType: lightingSceneCueType, Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{
		{
			OrderIndex: 0, ExecutionMode: "PARALLEL_BARRIER", TargetRef: "lighting.a.fade",
			CapabilityKey: lightingnode.CapabilityChannelsFade,
			Parameters: json.RawMessage(`{"fade_ms":1800,"aliases":{"front_warm":15}}`),
			TimeoutPolicy: json.RawMessage(`{"timeout_ms":4800}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: domain.PriorityP1, Enabled: true,
		},
		{
			OrderIndex: 1, ExecutionMode: "PARALLEL_BARRIER", TargetRef: "lighting.b.fade",
			CapabilityKey: lightingnode.CapabilityChannelsFade,
			Parameters: json.RawMessage(`{"fade_ms":1800,"aliases":{"back_warm":65}}`),
			TimeoutPolicy: json.RawMessage(`{"timeout_ms":4800}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: domain.PriorityP1, Enabled: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorLightingController(h.auth, devices, stageStore)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/lighting-controller/cues", nil)
	req.RemoteAddr = "127.0.0.1:19402"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Cues []lightingSceneView `json:"cues"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Cues) != 1 {
		t.Fatalf("cues=%+v", response.Cues)
	}
	cue := response.Cues[0]
	if cue.CueID != created.ID || cue.CommandType != lightingnode.CommandChannelsFade || cue.FadeMS != 1800 ||
		cue.Levels["front_warm"] != 15 || cue.Levels["back_warm"] != 65 || len(cue.DeviceIDs) != 2 {
		t.Fatalf("lighting cue=%+v", cue)
	}
}
