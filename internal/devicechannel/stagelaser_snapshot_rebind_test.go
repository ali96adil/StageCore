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

const stageLaserV2SnapshotB = "52222222-2222-4222-8222-222222222223"

func publishAdditionalStageLaserSnapshot(t *testing.T, f *runtimeFixture, snapshotID string, version int64) {
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
		"revision_number": version,
		"cues": []any{},
		"targets": []any{map[string]any{
			"alias_id": "laser-left",
			"target_ref": "laser_left",
			"logical_type": stagelaser.LogicalTargetType,
			"configuration": map[string]any{"device_id": testDeviceID},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.dbHandle.DB.ExecContext(context.Background(), `
		INSERT INTO runtime_snapshots
		(runtime_snapshot_id, project_id, revision_id, snapshot_version,
		 created_at_us, created_by, content_hash, manifest_json, status)
		VALUES (?, ?, ?, ?, ?, 'test', ?, ?, 'PUBLISHED')
	`, snapshotID, f.projectID, revisionID, version,
		time.Now().UTC().UnixMicro(), strings.Repeat("6", 64), string(manifest)); err != nil {
		t.Fatal(err)
	}
}

func activateStageLaserV2ScopeFor(
	t *testing.T,
	f *runtimeFixture,
	ws *websocket.Conn,
	state stageLaserV2AssignmentState,
	snapshotID string,
) {
	t.Helper()
	if state.State != "ACTIVE" || state.ProjectID != f.projectID ||
		state.RuntimeSnapshotID != snapshotID || !state.ScopeAckRequired ||
		!state.SafeOffRequired {
		t.Fatalf("active StageLaser assignment state=%+v", state)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "stagelaser.assignment.scope_ack",
		"schema_version": 2,
		"device_id": testDeviceID,
		"project_id": f.projectID,
		"runtime_snapshot_id": snapshotID,
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
		ready.ProjectID != f.projectID || ready.RuntimeSnapshotID != snapshotID ||
		ready.AssignmentEpoch != state.AssignmentEpoch ||
		ready.ConnectionGeneration != state.ConnectionGeneration ||
		!ready.CommandsEnabled {
		t.Fatalf("invalid StageLaser runtime.ready=%+v err=%v", ready, err)
	}
}

func TestStageLaserV2ActiveScopeSafelyRebindsToNewSnapshot(t *testing.T) {
	f := newRuntimeFixture(t)
	publishStageLaserV2Snapshot(t, f)
	publishAdditionalStageLaserSnapshot(t, f, stageLaserV2SnapshotB, 2)

	if _, err := f.repo.RegisterUnassignedV2(context.Background(), deviceexperience.Device{
		ID: testDeviceID,
		Kind: deviceexperience.DeviceGeneric,
		ProfileID: stagelaser.ProfileID,
		DisplayName: "Laser Left",
		Platform: "esp32",
		Architecture: "riscv32",
		ClientVersion: "test-stagelaser-v2",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: stagelaser.CapabilityKeys(),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	safe := safeStageLaserObservation()
	challenge := strings.Repeat("7a", 32)
	first, err := f.repo.CommitStageLaserSafeAssignment(context.Background(), deviceexperience.VerifiedStageLaserAssignmentInput{
		AssignmentID: "52222222-1111-4111-8111-111111111111",
		DeviceID: testDeviceID,
		TargetProjectID: f.projectID,
		TargetRuntimeSnapshotID: stageLaserV2SnapshotID,
		ExpectedEpoch: 1,
		ConnectionGeneration: 1,
		Challenge: challenge,
		AckDeviceID: testDeviceID,
		AckEpoch: 1,
		AckGeneration: 1,
		AckChallenge: challenge,
		AckObservation: safe,
		ActorID: "owner",
	})
	if err != nil || first.ToEpoch != 2 {
		t.Fatalf("seed StageLaser assignment=%+v err=%v", first, err)
	}

	ws, activeA := connectStageLaserV2(t, f)
	if activeA.State != "ACTIVE" || activeA.RuntimeSnapshotID != stageLaserV2SnapshotID ||
		activeA.AssignmentEpoch != 2 {
		_ = ws.Close()
		t.Fatalf("initial ACTIVE reconnect=%+v", activeA)
	}
	activateStageLaserV2ScopeFor(t, f, ws, activeA, stageLaserV2SnapshotID)

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
				ExpectedProjectID: f.projectID,
				ExpectedRuntimeSnapshotID: stageLaserV2SnapshotID,
				TargetProjectID: f.projectID,
				TargetRuntimeSnapshotID: stageLaserV2SnapshotB,
				ExpectedEpoch: activeA.AssignmentEpoch,
			},
			"owner",
			nil,
		)
		done <- outcome{record: record, err: err}
	}()

	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var prepare struct {
		Type                    string `json:"type"`
		AssignmentID            string `json:"assignment_id"`
		AssignmentEpoch         int64  `json:"assignment_epoch"`
		ConnectionGeneration    int64  `json:"connection_generation"`
		Challenge               string `json:"challenge"`
		TargetProjectID         string `json:"target_project_id"`
		TargetRuntimeSnapshotID string `json:"target_runtime_snapshot_id"`
		SafeOffRequired         bool   `json:"safe_off_required"`
	}
	err = websocket.JSON.Receive(ws, &prepare)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || prepare.Type != "stagelaser.assignment.prepare" ||
		prepare.AssignmentEpoch != activeA.AssignmentEpoch ||
		prepare.ConnectionGeneration != activeA.ConnectionGeneration ||
		prepare.TargetProjectID != f.projectID ||
		prepare.TargetRuntimeSnapshotID != stageLaserV2SnapshotB ||
		!prepare.SafeOffRequired {
		_ = ws.Close()
		t.Fatalf("StageLaser snapshot rebind prepare=%+v err=%v", prepare, err)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "stagelaser.assignment.safe_ack",
		"schema_version": 2,
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
		if result.err != nil || result.record.FromRuntimeSnapshotID != stageLaserV2SnapshotID ||
			result.record.ToRuntimeSnapshotID != stageLaserV2SnapshotB ||
			result.record.FromEpoch != 2 || result.record.ToEpoch != 3 {
			t.Fatalf("StageLaser snapshot rebind=%+v err=%v", result.record, result.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("StageLaser snapshot rebind did not commit")
	}
	_ = ws.Close()

	next, activeB := connectStageLaserV2(t, f)
	defer next.Close()
	if activeB.State != "ACTIVE" || activeB.RuntimeSnapshotID != stageLaserV2SnapshotB ||
		activeB.AssignmentEpoch != 3 ||
		activeB.ConnectionGeneration <= activeA.ConnectionGeneration {
		t.Fatalf("rebound StageLaser reconnect=%+v old=%+v", activeB, activeA)
	}
	activateStageLaserV2ScopeFor(t, f, next, activeB, stageLaserV2SnapshotB)
}
