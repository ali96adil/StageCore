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
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorStageLaserCueActionTranslatorBuildsCanonicalDesiredState(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "StageLaser Cue Authoring", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "stagelaser-authoring-01"
	if _, err := devices.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		Kind: deviceexperience.DeviceGeneric,
		ProfileID: stagelaser.ProfileID,
		DisplayName: "Laser Left",
		Platform: "esp32",
		Architecture: "riscv32",
		ClientVersion: "0.1.0-test",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: stagelaser.CapabilityKeys(),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=?, runtime_snapshot_id='authoring-snapshot',
		    assignment_epoch=2, assignment_state='ACTIVE', updated_at_us=updated_at_us+1
		WHERE device_id=? AND assignment_state='UNASSIGNED'
	`, project.ID, deviceID); err != nil {
		t.Fatal(err)
	}

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorStageLaserController(h.auth, devices, stageStore)).Handler()

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost,
			"/api/v1/projects/"+project.ID+"/stagelaser-controller/cue-actions",
			bytes.NewBufferString(body))
		req.RemoteAddr = "127.0.0.1:19431"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, credential.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	res := post(`{"device_id":"`+deviceID+`","command_type":"LASER_SET_ON"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("SET ON status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Actions []stageLaserCueActionDescriptor `json:"actions"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Actions) != 1 {
		t.Fatalf("actions=%+v", response.Actions)
	}
	action := response.Actions[0]
	if action.DeviceID != deviceID ||
		action.CapabilityKey != stagelaser.CapabilityStateSet ||
		action.ExecutionMode != "PARALLEL_BARRIER" ||
		action.Priority != "P1" ||
		!strings.HasPrefix(action.TargetRef, "stagelaser.") {
		t.Fatalf("StageLaser ON action=%+v", action)
	}
	var desired map[string]string
	if err := json.Unmarshal(action.Parameters, &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || desired["state"] != "ON" {
		t.Fatalf("StageLaser desired-state payload=%s", action.Parameters)
	}

	aliases, err := stageStore.ListAliases(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 ||
		aliases[0].LogicalType != devicechannel.StageDeviceLogicalType ||
		aliases[0].TargetRef != deviceID {
		t.Fatalf("StageLaser alias=%+v", aliases)
	}
	var cfg map[string]string
	if err := json.Unmarshal(aliases[0].ProjectConfig, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 1 || cfg["device_id"] != deviceID {
		t.Fatalf("StageLaser alias config=%v", cfg)
	}

	res = post(`{"device_id":"`+deviceID+`","command_type":"LASER_FLASH_START","frequency_hz":2,"duration_ms":8000}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unsafe 2 Hz mechanical flash status=%d body=%s", res.Code, res.Body.String())
	}
	res = post(`{"device_id":"`+deviceID+`","command_type":"LASER_FLASH_START","frequency_hz":1,"duration_ms":8000}`)
	if res.Code != http.StatusOK {
		t.Fatalf("safe flash status=%d body=%s", res.Code, res.Body.String())
	}
	res = post(`{"device_id":"`+deviceID+`","command_type":"LASER_STATE_RESYNC"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("diagnostic resync became Cue-safe: status=%d body=%s", res.Code, res.Body.String())
	}
}
