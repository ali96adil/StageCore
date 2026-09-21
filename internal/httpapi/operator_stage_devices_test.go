package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorStageDeviceListAndOfflineCommandAreAuditable(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Stage Devices", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID:              "tablet-01",
		ProjectID:       project.ID,
		Kind:            deviceexperience.DeviceTabletPlayer,
		DisplayName:     "Tablet 01",
		Platform:        "android",
		Architecture:    "arm64",
		ClientVersion:   "1.0.0",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities:    []string{"tablet.media.play", "tablet.media.stop"},
		Enabled:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/stage-devices", nil)
	listReq.RemoteAddr = "127.0.0.1:19001"
	listReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		Devices []deviceexperience.Device `json:"devices"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Devices) != 1 || listed.Devices[0].ID != device.ID {
		t.Fatalf("listed=%+v", listed.Devices)
	}

	commandBody := bytes.NewBufferString(`{"command_type":"TABLET_PLAY","idempotency_key":"cue-1/tablet-01/play","payload":{"media":"01.mp4"}}`)
	commandReq := httptest.NewRequest(http.MethodPost, "/api/v1/stage-devices/"+device.ID+"/commands", commandBody)
	commandReq.RemoteAddr = "127.0.0.1:19002"
	commandReq.Header.Set("Content-Type", "application/json")
	commandReq.Header.Set(csrfHeader, credential.CSRFToken)
	commandReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	commandRes := httptest.NewRecorder()
	handler.ServeHTTP(commandRes, commandReq)
	if commandRes.Code != http.StatusOK {
		t.Fatalf("command status=%d body=%s", commandRes.Code, commandRes.Body.String())
	}
	var command deviceexperience.DeviceCommand
	if err := json.Unmarshal(commandRes.Body.Bytes(), &command); err != nil {
		t.Fatal(err)
	}
	if command.Status != contracts.CommandFailed || command.CompletedAt == nil {
		t.Fatalf("command=%+v", command)
	}
	persisted, err := devices.GetCommand(ctx, command.Envelope.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != contracts.CommandFailed || !bytes.Contains(persisted.Result, []byte("DEVICE_OFFLINE")) {
		t.Fatalf("persisted status=%s result=%s", persisted.Status, persisted.Result)
	}
}

func TestOperatorStageDeviceGroupCommandSharesCorrelation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Callboard Group", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range []deviceexperience.Device{
		{ID: "display-01", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "Left", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, GroupName: "actors", Enabled: true},
		{ID: "display-02", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "Right", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, GroupName: "actors", Enabled: true},
		{ID: "display-03", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "Crew", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, GroupName: "crew", Enabled: true},
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
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()
	body := bytes.NewBufferString(`{"group_name":"actors","command_type":"DISPLAY_MESSAGE","correlation_id":"corr-callboard-group","idempotency_key":"places","payload":{"message":"Places"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/stage-device-commands", body)
	req.RemoteAddr = "127.0.0.1:19004"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("group command status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		CorrelationID string `json:"correlation_id"`
		Results       []struct {
			DeviceID string                          `json:"device_id"`
			Command  *deviceexperience.DeviceCommand `json:"command"`
			Error    string                          `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CorrelationID != "corr-callboard-group" || len(response.Results) != 2 {
		t.Fatalf("response=%+v", response)
	}
	seen := map[string]bool{}
	commandIDs := map[string]bool{}
	for _, item := range response.Results {
		if item.Error != "" || item.Command == nil {
			t.Fatalf("result=%+v", item)
		}
		if item.Command.Status != contracts.CommandFailed || item.Command.Envelope.CorrelationID != response.CorrelationID {
			t.Fatalf("command=%+v", item.Command)
		}
		seen[item.DeviceID] = true
		commandIDs[item.Command.Envelope.CommandID] = true
	}
	if !seen["display-01"] || !seen["display-02"] || seen["display-03"] || len(commandIDs) != 2 {
		t.Fatalf("target expansion seen=%v commands=%v", seen, commandIDs)
	}
	var count int
	if err := h.db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM stage_device_commands WHERE correlation_id = ?`, response.CorrelationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("persisted group commands=%d want=2", count)
	}
}

func TestOperatorNetworkCockpitReturnsLatestTargetState(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	latency := 12.0
	if _, err := devices.RecordNetworkObservation(ctx, deviceexperience.NetworkObservation{
		TargetKind:     "HUB",
		TargetID:       "hub-local",
		Reachability:   deviceexperience.Reachable,
		TransportState: "READY",
		LatencyMS:      &latency,
	}); err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/cockpit", nil)
	req.RemoteAddr = "127.0.0.1:19003"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("cockpit status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Targets []deviceexperience.CockpitTarget `json:"targets"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Targets) != 1 || response.Targets[0].TargetID != "hub-local" || response.Targets[0].Readiness != deviceexperience.ReadinessReady {
		t.Fatalf("targets=%+v", response.Targets)
	}
}

func TestOperatorStageDeviceAssignmentMetadataIsReadOnlyAndAuthenticated(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Assignment Inventory", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-readonly-01", ProjectID: project.ID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()
	path := "/api/v1/stage-devices/lighting-readonly-01/assignment"

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:19009"
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, req)
	if unauthorized.Code == http.StatusOK {
		t.Fatal("unauthenticated assignment metadata was exposed")
	}
	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:19010"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("assignment GET status=%d body=%s", res.Code, res.Body.String())
	}
	var record deviceexperience.AssignmentRecord
	if err := json.Unmarshal(res.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.DeviceID != "lighting-readonly-01" || record.ProjectID != project.ID ||
		record.State != deviceexperience.AssignmentLegacy || record.Epoch != 1 {
		t.Fatalf("read-only assignment inventory=%+v", record)
	}
	if bytes.Contains(res.Body.Bytes(), []byte("ACTIVE")) {
		t.Fatalf("legacy enrollment falsely promoted to v2 active: %s", res.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/stage-devices/missing/assignment", nil)
	req.RemoteAddr = "127.0.0.1:19011"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, req)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown assignment status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestOperatorUnassignedV2InventoryRequiresPairingPermission(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Legacy Show", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "legacy-inventory-01", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Legacy Tablet", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: "unassigned-v2-inventory-01", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "New Lighting Node", ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()
	path := "/api/v1/stage-devices/unassigned"

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:19030"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code == http.StatusOK {
		t.Fatalf("unassigned identity inventory leaked without authentication: %s", res.Body.String())
	}

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "127.0.0.1:19031"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("paired-device inventory status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Devices []struct {
			DeviceID string `json:"device_id"`
			DisplayName string `json:"display_name"`
			AssignmentEpoch int64 `json:"assignment_epoch"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Devices) != 1 || response.Devices[0].DeviceID != "unassigned-v2-inventory-01" ||
		response.Devices[0].DisplayName != "New Lighting Node" || response.Devices[0].AssignmentEpoch != 1 {
		t.Fatalf("unassigned v2 inventory=%+v", response.Devices)
	}
	if bytes.Contains(res.Body.Bytes(), []byte("legacy-inventory-01")) ||
		bytes.Contains(res.Body.Bytes(), []byte("project_id")) {
		t.Fatalf("inventory included legacy devices or project authority: %s", res.Body.String())
	}
}
