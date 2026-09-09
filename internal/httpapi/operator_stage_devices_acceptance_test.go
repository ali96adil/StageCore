package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorStageDeviceLocationCommandTargetsOnlyMatchingLocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Callboard Location", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range []deviceexperience.Device{
		{ID: "display-stage-left", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "Stage Left", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, LocationName: "backstage", Enabled: true},
		{ID: "display-stage-right", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "Stage Right", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, LocationName: "backstage", Enabled: true},
		{ID: "display-foh", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay, DisplayName: "FOH", ProtocolVersion: deviceexperience.ProtocolVersion1, Capabilities: []string{"display.message.show"}, LocationName: "foh", Enabled: true},
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
	body := bytes.NewBufferString(`{"location_name":"backstage","command_type":"DISPLAY_MESSAGE","correlation_id":"corr-location","idempotency_key":"places-backstage","payload":{"message":"Places"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/stage-device-commands", body)
	req.RemoteAddr = "127.0.0.1:19101"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("location command status=%d body=%s", res.Code, res.Body.String())
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
	if response.CorrelationID != "corr-location" || len(response.Results) != 2 {
		t.Fatalf("response=%+v", response)
	}
	seen := map[string]bool{}
	for _, item := range response.Results {
		if item.Error != "" || item.Command == nil {
			t.Fatalf("result=%+v", item)
		}
		if item.Command.Envelope.CorrelationID != response.CorrelationID || item.Command.Status != contracts.CommandFailed {
			t.Fatalf("command=%+v", item.Command)
		}
		seen[item.DeviceID] = true
	}
	if !seen["display-stage-left"] || !seen["display-stage-right"] || seen["display-foh"] {
		t.Fatalf("location expansion=%v", seen)
	}
}

func TestPhase4RuntimeRBACAndShowSafety(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Phase 4 SHOW Safety", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Cue One", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "active show"); err != nil {
		t.Fatal(err)
	}

	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	device, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "tablet-show-01", ProjectID: project.ID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "SHOW Tablet", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"tablet.media.play"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(devices, nil)
	defer runtime.Close()
	handler := New(WithOperatorStageDevices(h.auth, devices, runtime, stageStore)).Handler()

	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	for _, user := range []struct {
		id, username, role string
	}{
		{"00000000-0000-7000-8000-000000000031", "phase4-operator", "OPERATOR"},
		{"00000000-0000-7000-8000-000000000032", "phase4-viewer", "VIEWER"},
	} {
		if _, err := h.db.DB.ExecContext(ctx, `
			INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
			VALUES (?, ?, ?, ?, 1, 1, 1)
		`, user.id, user.username, passwordHash, user.role); err != nil {
			t.Fatal(err)
		}
	}
	operator, err := h.auth.Login(ctx, "phase4-operator", h.password, "127.0.0.31")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "phase4-viewer", h.password, "127.0.0.32")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	runtimeRequest := func(credentialToken, csrf string) *httptest.ResponseRecorder {
		body := bytes.NewBufferString(`{"command_type":"TABLET_PLAY","idempotency_key":"show-play","payload":{"media_ref":"01.mp4"}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/stage-devices/"+device.ID+"/commands", body)
		req.RemoteAddr = "127.0.0.1:19102"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, csrf)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credentialToken})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	operatorRuntime := runtimeRequest(operator.Token, operator.CSRFToken)
	if operatorRuntime.Code != http.StatusOK {
		t.Fatalf("OPERATOR runtime during SHOW status=%d body=%s", operatorRuntime.Code, operatorRuntime.Body.String())
	}
	var runtimeCommand deviceexperience.DeviceCommand
	if err := json.Unmarshal(operatorRuntime.Body.Bytes(), &runtimeCommand); err != nil {
		t.Fatal(err)
	}
	if runtimeCommand.Status != contracts.CommandFailed || runtimeCommand.CompletedAt == nil {
		t.Fatalf("offline SHOW runtime command=%+v", runtimeCommand)
	}

	viewerRuntime := runtimeRequest(viewer.Token, viewer.CSRFToken)
	if viewerRuntime.Code != http.StatusForbidden {
		t.Fatalf("VIEWER runtime status=%d want=403 body=%s", viewerRuntime.Code, viewerRuntime.Body.String())
	}

	viewerListReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/stage-devices", nil)
	viewerListReq.RemoteAddr = "127.0.0.1:19103"
	viewerListReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerListRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerListRes, viewerListReq)
	if viewerListRes.Code != http.StatusOK {
		t.Fatalf("VIEWER device read status=%d body=%s", viewerListRes.Code, viewerListRes.Body.String())
	}

	putSource := func(token, csrf string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+project.ID+"/live-video-sources/source-show", bytes.NewBufferString(`{}`))
		req.RemoteAddr = "127.0.0.1:19104"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, csrf)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	operatorConfig := putSource(operator.Token, operator.CSRFToken)
	if operatorConfig.Code != http.StatusForbidden {
		t.Fatalf("OPERATOR structural config status=%d want=403 body=%s", operatorConfig.Code, operatorConfig.Body.String())
	}
	ownerConfig := putSource(owner.Token, owner.CSRFToken)
	if ownerConfig.Code != http.StatusLocked || !strings.Contains(ownerConfig.Body.String(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("OWNER structural config during SHOW status=%d want=423 body=%s", ownerConfig.Code, ownerConfig.Body.String())
	}
}

func TestPhase4OperatorPolishExposesBroadTabletAndLocationTargeting(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/phase4-polish.js", nil)
	req.RemoteAddr = "127.0.0.1:19105"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("phase4-polish.js status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, required := range []string{"location_name", "TABLET_SELECT_MEDIA", "data-tablet-batch-command", "/stage-device-commands", "tabletBatchTarget"} {
		if !strings.Contains(body, required) {
			t.Fatalf("phase4-polish.js missing %q", required)
		}
	}
}
