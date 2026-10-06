package devicechannel_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"golang.org/x/net/websocket"
)

const stageLaserV2SnapshotID = "52222222-2222-4222-8222-222222222222"

type stageLaserV2AssignmentState struct {
	Type                 string `json:"type"`
	State                string `json:"state"`
	DeviceID             string `json:"device_id"`
	ProjectID            string `json:"project_id"`
	RuntimeSnapshotID    string `json:"runtime_snapshot_id"`
	AssignmentEpoch      int64  `json:"assignment_epoch"`
	ConnectionGeneration int64  `json:"connection_generation"`
	CommandsEnabled      bool   `json:"commands_enabled"`
	ScopeAckRequired     bool   `json:"scope_ack_required"`
	SafeOffRequired      bool   `json:"safe_off_required"`
	BlackoutRequired     bool   `json:"blackout_required"`
}

func safeStageLaserObservation() stagelaser.Observation {
	return stagelaser.Observation{
		SchemaVersion: stagelaser.SchemaVersion1,
		ControlContractVersion: stagelaser.ControlContractVersion,
		ArmState: stagelaser.ArmDisarmed,
		LogicalState: stagelaser.StateOff,
		StateQuality: stagelaser.StateQualityTracked,
		DriverKind: stagelaser.DriverMechanicalRelay,
		Limits: stagelaser.DefaultMechanicalLimits(),
	}
}

func publishStageLaserV2Snapshot(t *testing.T, f *runtimeFixture) {
	t.Helper()
	var revisionID string
	if err := f.dbHandle.DB.QueryRowContext(context.Background(),
		"SELECT current_revision_id FROM projects WHERE project_id = ?", f.projectID).
		Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]any{
		"schema_version": 5,
		"project_id": f.projectID,
		"revision_id": revisionID,
		"revision_number": 1,
		"cues": []any{},
		"targets": []any{
			map[string]any{
				"alias_id": "laser-left",
				"target_ref": "laser_left",
				"logical_type": stagelaser.LogicalTargetType,
				"configuration": map[string]any{"device_id": testDeviceID},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.dbHandle.DB.ExecContext(context.Background(), `
		INSERT INTO runtime_snapshots
		(runtime_snapshot_id, project_id, revision_id, snapshot_version,
		 created_at_us, created_by, content_hash, manifest_json, status)
		VALUES (?, ?, ?, 1, ?, 'test', ?, ?, 'PUBLISHED')
	`, stageLaserV2SnapshotID, f.projectID, revisionID,
		time.Now().UTC().UnixMicro(), strings.Repeat("5", 64), string(manifest)); err != nil {
		t.Fatal(err)
	}
}

func connectStageLaserV2(
	t *testing.T,
	f *runtimeFixture,
) (*websocket.Conn, stageLaserV2AssignmentState) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.hello", "schema_version": 1,
		"device_id": testDeviceID, "project_id": "",
		"device_kind": deviceexperience.DeviceGeneric,
		"profile_id": stagelaser.ProfileID,
		"display_name": "Laser Left",
		"platform": "esp32", "architecture": "riscv32",
		"client_version": "test-stagelaser-v2",
		"protocol_version": deviceexperience.ProtocolVersion2,
		"capabilities": stagelaser.CapabilityKeys(),
		"readiness": deviceexperience.ReadinessReady,
		"observed_state": safeStageLaserObservation(),
		"network_state": json.RawMessage(`{"transport":"WSS"}`),
	}); err != nil {
		_ = ws.Close()
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var assignment stageLaserV2AssignmentState
	err = websocket.JSON.Receive(ws, &assignment)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || assignment.Type != "assignment.state" ||
		assignment.DeviceID != testDeviceID ||
		assignment.AssignmentEpoch < 1 ||
		assignment.ConnectionGeneration < 1 ||
		assignment.CommandsEnabled {
		_ = ws.Close()
		t.Fatalf("invalid StageLaser v2 assignment state=%+v err=%v", assignment, err)
	}
	return ws, assignment
}

func activateStageLaserV2Scope(
	t *testing.T,
	f *runtimeFixture,
	ws *websocket.Conn,
	state stageLaserV2AssignmentState,
) {
	t.Helper()
	if state.State != "ACTIVE" || state.ProjectID != f.projectID ||
		state.RuntimeSnapshotID != stageLaserV2SnapshotID ||
		!state.ScopeAckRequired || !state.SafeOffRequired {
		t.Fatalf("active StageLaser assignment state=%+v", state)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "stagelaser.assignment.scope_ack",
		"schema_version": 2,
		"device_id": testDeviceID,
		"project_id": f.projectID,
		"runtime_snapshot_id": stageLaserV2SnapshotID,
		"assignment_epoch": state.AssignmentEpoch,
		"connection_generation": state.ConnectionGeneration,
		"readiness": deviceexperience.ReadinessReady,
		"observed_state": safeStageLaserObservation(),
		"network_state": json.RawMessage(`{"transport":"WSS"}`),
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var ready struct {
		Type                 string `json:"type"`
		SchemaVersion        int    `json:"schema_version"`
		ProjectID            string `json:"project_id"`
		RuntimeSnapshotID    string `json:"runtime_snapshot_id"`
		AssignmentEpoch      int64  `json:"assignment_epoch"`
		ConnectionGeneration int64  `json:"connection_generation"`
		CommandsEnabled      bool   `json:"commands_enabled"`
	}
	err := websocket.JSON.Receive(ws, &ready)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || ready.Type != "runtime.ready" || ready.SchemaVersion != 2 ||
		ready.ProjectID != f.projectID ||
		ready.RuntimeSnapshotID != stageLaserV2SnapshotID ||
		ready.AssignmentEpoch != state.AssignmentEpoch ||
		ready.ConnectionGeneration != state.ConnectionGeneration ||
		!ready.CommandsEnabled {
		t.Fatalf("invalid StageLaser runtime.ready=%+v err=%v", ready, err)
	}
}

func TestStageLaserV2SafeAssignmentActivationAndCommandDispatch(t *testing.T) {
	f := newRuntimeFixture(t)
	publishStageLaserV2Snapshot(t, f)

	first, initial := connectStageLaserV2(t, f)
	if initial.State != "UNASSIGNED" || initial.ProjectID != "" ||
		initial.RuntimeSnapshotID != "" || !initial.SafeOffRequired ||
		initial.BlackoutRequired {
		t.Fatalf("initial StageLaser assignment=%+v", initial)
	}
	device, err := f.repo.GetDevice(context.Background(), testDeviceID)
	if err != nil || device.Runtime == nil ||
		device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
		t.Fatalf("projectless StageLaser escaped BLOCKER: device=%+v err=%v", device, err)
	}

	type outcome struct {
		record deviceexperience.StageLaserAssignmentCommit
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		record, err := f.runtime.ExecuteStageLaserAssignmentAuthorized(
			context.Background(),
			deviceexperience.StageLaserAssignmentInput{
				DeviceID: testDeviceID,
				TargetProjectID: f.projectID,
				TargetRuntimeSnapshotID: stageLaserV2SnapshotID,
				ExpectedEpoch: initial.AssignmentEpoch,
			},
			"owner",
			nil,
		)
		done <- outcome{record: record, err: err}
	}()

	_ = first.SetReadDeadline(time.Now().Add(3 * time.Second))
	var prepare struct {
		Type                    string `json:"type"`
		AssignmentID            string `json:"assignment_id"`
		AssignmentEpoch         int64  `json:"assignment_epoch"`
		ConnectionGeneration    int64  `json:"connection_generation"`
		Challenge               string `json:"challenge"`
		TargetProjectID         string `json:"target_project_id"`
		TargetRuntimeSnapshotID string `json:"target_runtime_snapshot_id"`
		SafeOffRequired         bool   `json:"safe_off_required"`
		RequiredArmState        string `json:"required_arm_state"`
		RequiredLogicalState    string `json:"required_logical_state"`
	}
	err = websocket.JSON.Receive(first, &prepare)
	_ = first.SetReadDeadline(time.Time{})
	if err != nil || prepare.Type != "stagelaser.assignment.prepare" ||
		prepare.AssignmentID == "" || len(prepare.Challenge) != 64 ||
		prepare.AssignmentEpoch != initial.AssignmentEpoch ||
		prepare.ConnectionGeneration != initial.ConnectionGeneration ||
		prepare.TargetProjectID != f.projectID ||
		prepare.TargetRuntimeSnapshotID != stageLaserV2SnapshotID ||
		!prepare.SafeOffRequired ||
		prepare.RequiredArmState != string(stagelaser.ArmDisarmed) ||
		prepare.RequiredLogicalState != string(stagelaser.StateOff) {
		t.Fatalf("invalid StageLaser assignment prepare=%+v err=%v", prepare, err)
	}

	if err := websocket.JSON.Send(first, map[string]any{
		"type": "stagelaser.assignment.safe_ack", "schema_version": 2,
		"device_id": testDeviceID,
		"assignment_id": prepare.AssignmentID,
		"assignment_epoch": prepare.AssignmentEpoch,
		"connection_generation": prepare.ConnectionGeneration,
		"challenge": prepare.Challenge,
		"observed_state": safeStageLaserObservation(),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.err != nil || result.record.NextState != "ACTIVE" ||
			result.record.ToEpoch != initial.AssignmentEpoch+1 ||
			result.record.ToProjectID != f.projectID ||
			result.record.ToRuntimeSnapshotID != stageLaserV2SnapshotID {
			t.Fatalf("StageLaser assignment commit=%+v err=%v", result.record, result.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("StageLaser safe-off assignment did not commit")
	}
	_ = first.Close()

	second, active := connectStageLaserV2(t, f)
	defer second.Close()
	if active.State != "ACTIVE" ||
		active.AssignmentEpoch != initial.AssignmentEpoch+1 ||
		active.ConnectionGeneration <= initial.ConnectionGeneration {
		t.Fatalf("active StageLaser reconnect=%+v initial=%+v", active, initial)
	}
	activateStageLaserV2Scope(t, f, second, active)

	deadline := time.Now().Add(10 * time.Second)
	command, err := f.runtime.Dispatch(context.Background(), deviceexperience.CreateCommandInput{
		ProjectID: f.projectID,
		RuntimeSnapshotID: stageLaserV2SnapshotID,
		DeviceID: testDeviceID,
		CommandType: stagelaser.CommandArm,
		Issuer: "operator:test",
		CorrelationID: "stagelaser-v2-corr-1",
		IdempotencyKey: "stagelaser-v2/arm/1",
		Payload: json.RawMessage(`{}`),
		DeadlineAt: &deadline,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = second.SetReadDeadline(time.Now().Add(3 * time.Second))
	var execute struct {
		Type          string `json:"type"`
		SchemaVersion int    `json:"schema_version"`
		Command       struct {
			CommandID         string `json:"command_id"`
			CommandType       string `json:"command_type"`
			ProjectID         string `json:"project_id"`
			RuntimeSnapshotID string `json:"runtime_snapshot_id"`
		} `json:"command"`
	}
	err = websocket.JSON.Receive(second, &execute)
	_ = second.SetReadDeadline(time.Time{})
	if err != nil || execute.Type != "command.execute" || execute.SchemaVersion != 2 ||
		execute.Command.CommandID != command.Envelope.CommandID ||
		execute.Command.CommandType != stagelaser.CommandArm ||
		execute.Command.ProjectID != f.projectID ||
		execute.Command.RuntimeSnapshotID != stageLaserV2SnapshotID {
		t.Fatalf("StageLaser command.execute=%+v err=%v", execute, err)
	}
	if err := websocket.JSON.Send(second, map[string]any{
		"type": "command.result", "schema_version": 2,
		"device_id": testDeviceID,
		"command_id": command.Envelope.CommandID,
		"status": "COMPLETED",
		"payload": json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
}
