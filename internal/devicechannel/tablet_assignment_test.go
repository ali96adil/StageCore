package devicechannel_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"golang.org/x/net/websocket"
)

const tabletV2SnapshotID = "22222222-2222-4222-8222-222222222222"

type tabletV2AssignmentState struct {
	Type                   string `json:"type"`
	State                  string `json:"state"`
	DeviceID               string `json:"device_id"`
	ProjectID              string `json:"project_id"`
	RuntimeSnapshotID      string `json:"runtime_snapshot_id"`
	AssignmentEpoch        int64  `json:"assignment_epoch"`
	ConnectionGeneration   int64  `json:"connection_generation"`
	CommandsEnabled        bool   `json:"commands_enabled"`
	SafeMediaRequired      bool   `json:"safe_media_required"`
	ScopeAckRequired       bool   `json:"scope_ack_required"`
}

func publishTabletV2Snapshot(t *testing.T, f *runtimeFixture) {
	t.Helper()
	var revisionID string
	if err := f.dbHandle.DB.QueryRowContext(context.Background(),
		"SELECT current_revision_id FROM projects WHERE project_id = ?", f.projectID).
		Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.dbHandle.DB.ExecContext(context.Background(), `
		INSERT INTO runtime_snapshots
		(runtime_snapshot_id, project_id, revision_id, snapshot_version,
		 created_at_us, created_by, content_hash, manifest_json, status)
		VALUES (?, ?, ?, 1, ?, 'test', ?, '{}', 'PUBLISHED')
	`, tabletV2SnapshotID, f.projectID, revisionID, time.Now().UTC().UnixMicro(),
		strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
}

func connectTabletV2(t *testing.T, f *runtimeFixture, advertisedReadiness deviceexperience.Readiness) (*websocket.Conn, tabletV2AssignmentState) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.hello", "schema_version": 1,
		"device_id": testDeviceID, "project_id": "",
		"device_kind": deviceexperience.DeviceTabletPlayer,
		"profile_id": deviceexperience.TabletPlayerProfileID,
		"display_name": "Tablet 01",
		"platform": "android", "architecture": "arm64",
		"client_version": "test-v2",
		"protocol_version": deviceexperience.ProtocolVersion2,
		"capabilities": []string{"tablet.media.play", "tablet.media.stop", "tablet.media.blackout"},
		"readiness": advertisedReadiness,
		"observed_state": json.RawMessage(`{"player_ready":true}`),
		"network_state": json.RawMessage(`{"transport":"WSS"}`),
	}); err != nil {
		_ = ws.Close()
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var assignment tabletV2AssignmentState
	err = websocket.JSON.Receive(ws, &assignment)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || assignment.Type != "assignment.state" ||
		assignment.DeviceID != testDeviceID ||
		assignment.AssignmentEpoch < 1 ||
		assignment.ConnectionGeneration < 1 ||
		assignment.CommandsEnabled {
		_ = ws.Close()
		t.Fatalf("invalid tablet v2 assignment state=%+v err=%v", assignment, err)
	}
	return ws, assignment
}

func activateTabletV2Scope(t *testing.T, f *runtimeFixture, ws *websocket.Conn, state tabletV2AssignmentState) {
	t.Helper()
	if state.State != "ACTIVE" || state.ProjectID != f.projectID ||
		state.RuntimeSnapshotID != tabletV2SnapshotID ||
		!state.ScopeAckRequired {
		t.Fatalf("active assignment state=%+v", state)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "assignment.scope_ack", "schema_version": 2,
		"device_id": testDeviceID,
		"project_id": f.projectID,
		"runtime_snapshot_id": tabletV2SnapshotID,
		"assignment_epoch": state.AssignmentEpoch,
		"connection_generation": state.ConnectionGeneration,
		"readiness": deviceexperience.ReadinessReady,
		"observed_state": json.RawMessage(`{
			"project_id":"ignored-client-authority",
			"runtime_snapshot_id":"ignored-client-snapshot",
			"tablet_manifest_id":"local-manifest"
		}`),
		"network_state": json.RawMessage(`{"transport":"WSS"}`),
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var ready struct {
		Type                   string `json:"type"`
		SchemaVersion          int    `json:"schema_version"`
		ProjectID              string `json:"project_id"`
		RuntimeSnapshotID      string `json:"runtime_snapshot_id"`
		AssignmentEpoch        int64  `json:"assignment_epoch"`
		ConnectionGeneration   int64  `json:"connection_generation"`
		CommandsEnabled        bool   `json:"commands_enabled"`
	}
	err := websocket.JSON.Receive(ws, &ready)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || ready.Type != "runtime.ready" || ready.SchemaVersion != 2 ||
		ready.ProjectID != f.projectID ||
		ready.RuntimeSnapshotID != tabletV2SnapshotID ||
		ready.AssignmentEpoch != state.AssignmentEpoch ||
		ready.ConnectionGeneration != state.ConnectionGeneration ||
		!ready.CommandsEnabled {
		t.Fatalf("invalid tablet v2 runtime.ready=%+v err=%v", ready, err)
	}
}

func TestTabletV2SafeAssignmentActivationAndReconnectNeverReplayCommand(t *testing.T) {
	f := newRuntimeFixture(t)
	publishTabletV2Snapshot(t, f)

	// A client-advertised READY before Hub scope activation must be clamped to
	// BLOCKER. The first socket has inventory authority only.
	first, initial := connectTabletV2(t, f, deviceexperience.ReadinessReady)
	if initial.State != "UNASSIGNED" || initial.ProjectID != "" ||
		initial.RuntimeSnapshotID != "" || !initial.SafeMediaRequired {
		t.Fatalf("initial tablet assignment=%+v", initial)
	}
	device, err := f.repo.GetDevice(context.Background(), testDeviceID)
	if err != nil || device.Runtime == nil ||
		device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
		t.Fatalf("projectless v2 tablet escaped BLOCKER: device=%+v err=%v", device, err)
	}

	type outcome struct {
		record deviceexperience.TabletAssignmentCommit
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		record, err := f.runtime.ExecuteTabletAssignmentAuthorized(
			context.Background(),
			deviceexperience.TabletAssignmentInput{
				DeviceID: testDeviceID,
				ExpectedEpoch: initial.AssignmentEpoch,
				TargetProjectID: f.projectID,
				TargetRuntimeSnapshotID: tabletV2SnapshotID,
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
		SafeMediaRequired       bool   `json:"safe_media_required"`
	}
	err = websocket.JSON.Receive(first, &prepare)
	_ = first.SetReadDeadline(time.Time{})
	if err != nil || prepare.Type != "tablet.assignment.prepare" ||
		prepare.AssignmentID == "" || len(prepare.Challenge) != 64 ||
		prepare.AssignmentEpoch != initial.AssignmentEpoch ||
		prepare.ConnectionGeneration != initial.ConnectionGeneration ||
		prepare.TargetProjectID != f.projectID ||
		prepare.TargetRuntimeSnapshotID != tabletV2SnapshotID ||
		!prepare.SafeMediaRequired {
		t.Fatalf("invalid tablet assignment prepare=%+v err=%v", prepare, err)
	}

	if err := websocket.JSON.Send(first, map[string]any{
		"type": "tablet.assignment.safe_ack", "schema_version": 2,
		"device_id": testDeviceID,
		"assignment_id": prepare.AssignmentID,
		"assignment_epoch": prepare.AssignmentEpoch,
		"connection_generation": prepare.ConnectionGeneration,
		"challenge": prepare.Challenge,
		"safe_media": true,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.err != nil || result.record.NextState != "ACTIVE" ||
			result.record.ToEpoch != initial.AssignmentEpoch+1 ||
			result.record.ToProjectID != f.projectID ||
			result.record.ToRuntimeSnapshotID != tabletV2SnapshotID {
			t.Fatalf("tablet assignment commit=%+v err=%v", result.record, result.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("tablet safe-media assignment did not commit")
	}
	_ = first.Close()

	second, active := connectTabletV2(t, f, deviceexperience.ReadinessReady)
	defer second.Close()
	if active.State != "ACTIVE" ||
		active.AssignmentEpoch != initial.AssignmentEpoch+1 ||
		active.ConnectionGeneration <= initial.ConnectionGeneration {
		t.Fatalf("active tablet reconnect=%+v initial=%+v", active, initial)
	}
	activateTabletV2Scope(t, f, second, active)

	device, err = f.repo.GetDevice(context.Background(), testDeviceID)
	if err != nil || device.Runtime == nil ||
		device.Runtime.Readiness != deviceexperience.ReadinessReady ||
		device.Assignment == nil || device.Assignment.State != "ACTIVE" ||
		device.Assignment.ProjectID != f.projectID ||
		device.Assignment.RuntimeSnapshotID != tabletV2SnapshotID {
		t.Fatalf("activated tablet device=%+v err=%v", device, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	command, err := f.runtime.Dispatch(context.Background(), deviceexperience.CreateCommandInput{
		ProjectID: f.projectID,
		RuntimeSnapshotID: tabletV2SnapshotID,
		DeviceID: testDeviceID,
		CommandType: "TABLET_PLAY",
		Issuer: "operator:test",
		CorrelationID: "tablet-v2-corr-1",
		IdempotencyKey: "tablet-v2/play/1",
		Payload: json.RawMessage(`{"media_number":1}`),
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
			ProjectID         string `json:"project_id"`
			RuntimeSnapshotID string `json:"runtime_snapshot_id"`
		} `json:"command"`
	}
	err = websocket.JSON.Receive(second, &execute)
	_ = second.SetReadDeadline(time.Time{})
	if err != nil || execute.Type != "command.execute" || execute.SchemaVersion != 2 ||
		execute.Command.CommandID != command.Envelope.CommandID ||
		execute.Command.ProjectID != f.projectID ||
		execute.Command.RuntimeSnapshotID != tabletV2SnapshotID {
		t.Fatalf("v2 tablet command=%+v err=%v", execute, err)
	}

	// Reconnect while that command has no terminal result. The replacement
	// socket must reacquire scope but must never receive a replay.
	third, reconnect := connectTabletV2(t, f, deviceexperience.ReadinessReady)
	defer third.Close()
	activateTabletV2Scope(t, f, third, reconnect)
	_ = second.Close()

	_ = third.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var replay map[string]any
	if err := websocket.JSON.Receive(third, &replay); err == nil {
		t.Fatalf("pending tablet command replayed after reconnect: %+v", replay)
	}
	_ = third.SetReadDeadline(time.Time{})
}
