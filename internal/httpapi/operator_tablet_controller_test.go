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
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorTabletControllerListsOnlyTabletPlayersAndGroups(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet Controller", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range []deviceexperience.Device{
		{ID: "tablet-02", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer, DisplayName: "Tablet 02", Platform: "android", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{deviceexperience.CapabilityTabletPlay}, GroupName: "actors", Enabled: true},
		{ID: "tablet-01", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer, DisplayName: "Tablet 01", Platform: "android", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{deviceexperience.CapabilityTabletPlay}, GroupName: "actors", Enabled: true},
		{ID: "display-01", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "Callboard", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, GroupName: "crew", Enabled: true},
	} {
		if _, err := devices.UpsertDevice(ctx, device); err != nil {
			t.Fatal(err)
		}
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorTabletController(h.auth, devices, runtime, stageStore)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/tablet-controller", nil)
	req.RemoteAddr = "127.0.0.1:19201"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Devices []deviceexperience.Device `json:"devices"`
		Groups  []string                  `json:"groups"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Devices) != 2 || response.Devices[0].ID != "tablet-01" || response.Devices[1].ID != "tablet-02" {
		t.Fatalf("devices=%+v", response.Devices)
	}
	if len(response.Groups) != 1 || response.Groups[0] != "actors" {
		t.Fatalf("groups=%v", response.Groups)
	}
}

func TestOperatorTabletControllerCueActionsCreateCanonicalAliasOnce(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet Cue Builder", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID:              "tablet-01",
		ProjectID:       project.ID,
		Kind:            deviceexperience.DeviceTabletPlayer,
		DisplayName:     "Tablet 01",
		Platform:        "android",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities:    []string{deviceexperience.CapabilityTabletPlay},
		GroupName:       "actors",
		Enabled:         true,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorTabletController(h.auth, devices, runtime, stageStore)).Handler()
	post := func() tabletCueActionDescriptor {
		body := bytes.NewBufferString(`{"device_ids":["tablet-01"],"command_type":"TABLET_PLAY","payload":{"media_number":7}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/tablet-controller/cue-actions", body)
		req.RemoteAddr = "127.0.0.1:19202"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, credential.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
		var response struct {
			Actions []tabletCueActionDescriptor `json:"actions"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Actions) != 1 {
			t.Fatalf("actions=%+v", response.Actions)
		}
		return response.Actions[0]
	}

	first := post()
	if first.DeviceID != "tablet-01" || first.CapabilityKey != deviceexperience.CapabilityTabletPlay || first.ExecutionMode != "PARALLEL_BARRIER" || first.Priority != "P1" {
		t.Fatalf("action=%+v", first)
	}
	var parameters map[string]any
	if err := json.Unmarshal(first.Parameters, &parameters); err != nil {
		t.Fatal(err)
	}
	if got := parameters["media_number"]; got != float64(7) {
		t.Fatalf("media_number=%v", got)
	}
	second := post()
	if second.TargetRef != first.TargetRef {
		t.Fatalf("target refs differ first=%q second=%q", first.TargetRef, second.TargetRef)
	}
	aliases, err := stageStore.ListAliases(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 {
		t.Fatalf("aliases=%+v", aliases)
	}
	if aliases[0].LogicalType != devicechannel.StageDeviceLogicalType || aliases[0].LogicalName != first.TargetRef || aliases[0].TargetRef != "tablet-01" {
		t.Fatalf("alias=%+v", aliases[0])
	}
	var config struct {
		DeviceID      string `json:"device_id"`
		CapabilityKey string `json:"capability_key"`
	}
	if err := json.Unmarshal(aliases[0].ProjectConfig, &config); err != nil {
		t.Fatal(err)
	}
	if config.DeviceID != "tablet-01" || config.CapabilityKey != deviceexperience.CapabilityTabletPlay {
		t.Fatalf("config=%+v", config)
	}
}

func TestLegacyTabletScopeMatchesAndroidObservedState(t *testing.T) {
	device := deviceexperience.Device{
		Runtime: &deviceexperience.RuntimeState{
			Connection:    deviceexperience.ConnectionOnline,
			ObservedState: json.RawMessage(`{"project_id":"project-1","runtime_snapshot_id":"snapshot-7","tablet_manifest_id":"manifest-a"}`),
		},
	}
	scope, err := tabletScope(device, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if scope.RuntimeSnapshotID != "snapshot-7" || scope.TabletManifestID != "manifest-a" {
		t.Fatalf("scope=%+v", scope)
	}
	device.Runtime.Connection = deviceexperience.ConnectionOffline
	if _, err := tabletScope(device, "project-1"); err == nil || err.Error() != "DEVICE_OFFLINE" {
		t.Fatalf("offline err=%v", err)
	}
}


func TestV2TabletScopeUsesHubAssignmentNotObservedProjectAuthority(t *testing.T) {
	device := deviceexperience.Device{
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProfileID:       deviceexperience.TabletPlayerProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Assignment: &deviceexperience.AssignmentRecord{
			ProjectID:         "project-hub",
			RuntimeSnapshotID: "snapshot-hub",
			Epoch:             4,
			State:             "ACTIVE",
		},
		Runtime: &deviceexperience.RuntimeState{
			Connection: deviceexperience.ConnectionOnline,
			Readiness:  deviceexperience.ReadinessReady,
			ObservedState: json.RawMessage(
				`{"project_id":"project-client-wrong","runtime_snapshot_id":"snapshot-client-wrong","tablet_manifest_id":"manifest-local"}`,
			),
		},
	}

	scope, err := tabletScope(device, "project-hub")
	if err != nil {
		t.Fatal(err)
	}
	if scope.ProjectID != "project-hub" ||
		scope.RuntimeSnapshotID != "snapshot-hub" ||
		scope.TabletManifestID != "manifest-local" {
		t.Fatalf("v2 scope must use Hub assignment and only accept manifest as a hint: %+v", scope)
	}
	if _, err := tabletScope(device, "project-client-wrong"); err == nil ||
		err.Error() != "HUB_ASSIGNMENT_SCOPE_MISMATCH" {
		t.Fatalf("client-observed Project escaped Hub authority: %v", err)
	}
}

func TestTabletDevicesIncludeOnlyActiveV2TabletAssignments(t *testing.T) {
	active := deviceexperience.Device{
		ID:              "tablet-active",
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Assignment: &deviceexperience.AssignmentRecord{
			ProjectID: "project-1", RuntimeSnapshotID: "snapshot-1",
			Epoch: 2, State: "ACTIVE",
		},
	}
	unassigned := deviceexperience.Device{
		ID:              "tablet-unassigned",
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Assignment: &deviceexperience.AssignmentRecord{Epoch: 1, State: "UNASSIGNED"},
	}
	legacy := deviceexperience.Device{
		ID:              "tablet-v1",
		Kind:            deviceexperience.DeviceTabletPlayer,
		ProtocolVersion: deviceexperience.ProtocolVersion1,
	}
	got := tabletDevices([]deviceexperience.Device{unassigned, active, legacy})
	if len(got) != 2 || got[0].ID != "tablet-active" || got[1].ID != "tablet-v1" {
		t.Fatalf("tablet inventory=%+v", got)
	}
}


func TestNormalizeTabletLivePayload(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantKey string
		wantVal string
		wantErr bool
	}{
		{name: "media key", raw: `{"media_key":"live.camera.01"}`, wantKey: "media_key", wantVal: "live.camera.01"},
		{name: "http url", raw: `{"url":"http://192.168.3.130:9081/api/v0/stream"}`, wantKey: "url", wantVal: "http://192.168.3.130:9081/api/v0/stream"},
		{name: "https url", raw: `{"url":"https://relay.example.test/live.mjpeg"}`, wantKey: "url", wantVal: "https://relay.example.test/live.mjpeg"},
		{name: "both", raw: `{"media_key":"camera","url":"http://relay/live"}`, wantErr: true},
		{name: "missing", raw: `{}`, wantErr: true},
		{name: "credentials", raw: `{"url":"http://user:pass@relay/live"}`, wantErr: true},
		{name: "wrong scheme", raw: `{"url":"file:///tmp/live.mjpeg"}`, wantErr: true},
		{name: "unknown field", raw: `{"url":"http://relay/live","token":"secret"}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeTabletCommandPayload(deviceexperience.CommandTabletLiveShow, json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]string
			if err := json.Unmarshal(got, &object); err != nil {
				t.Fatal(err)
			}
			if len(object) != 1 || object[tc.wantKey] != tc.wantVal {
				t.Fatalf("normalized payload=%v", object)
			}
		})
	}
}
