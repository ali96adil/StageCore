package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestStageLaserActiveAssignmentRebindsSameProjectToNewPublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Fixed{Time: phase4Time})
	project, err := stageStore.GetProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}

	const deviceID = "stagelaser-rebind-01"
	const snapshotA = "41111111-2222-4333-8444-555555555551"
	const snapshotB = "41111111-2222-4333-8444-555555555552"
	manifestFor := func(snapshotVersion int) string {
		raw, err := json.Marshal(map[string]any{
			"schema_version": 5,
			"project_id": projectID,
			"revision_id": project.CurrentRevisionID,
			"revision_number": snapshotVersion,
			"targets": []any{map[string]any{
				"alias_id": "laser-left",
				"target_ref": "stagelaser.left",
				"logical_type": stagelaser.LogicalTargetType,
				"configuration": map[string]any{"device_id": deviceID},
			}},
			"cues": []any{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for index, id := range []string{snapshotA, snapshotB} {
		if _, err := handle.DB.ExecContext(ctx, `
			INSERT INTO runtime_snapshots
			(runtime_snapshot_id, project_id, revision_id, snapshot_version,
			 created_at_us, created_by, content_hash, manifest_json, status)
			VALUES (?, ?, ?, ?, ?, 'test', ?, ?, 'PUBLISHED')
		`, id, projectID, project.CurrentRevisionID, index+1,
			phase4Time.UnixMicro()+int64(index),
			strings.Repeat(string(rune('a'+index)), 64),
			manifestFor(index+1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
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

	safe := stagelaser.Observation{
		SchemaVersion: stagelaser.SchemaVersion1,
		ControlContractVersion: stagelaser.ControlContractVersion,
		ArmState: stagelaser.ArmDisarmed,
		LogicalState: stagelaser.StateOff,
		StateQuality: stagelaser.StateQualityTracked,
		DriverKind: stagelaser.DriverMechanicalRelay,
		Limits: stagelaser.DefaultMechanicalLimits(),
	}
	commit := func(id, challenge, expectedProject, expectedSnapshot, targetSnapshot string, epoch, generation int64) deviceexperience.StageLaserAssignmentCommit {
		record, err := repo.CommitStageLaserSafeAssignment(ctx, deviceexperience.VerifiedStageLaserAssignmentInput{
			AssignmentID: id,
			DeviceID: deviceID,
			ExpectedProjectID: expectedProject,
			ExpectedRuntimeSnapshotID: expectedSnapshot,
			TargetProjectID: projectID,
			TargetRuntimeSnapshotID: targetSnapshot,
			ExpectedEpoch: epoch,
			ConnectionGeneration: generation,
			Challenge: challenge,
			AckDeviceID: deviceID,
			AckEpoch: epoch,
			AckGeneration: generation,
			AckChallenge: challenge,
			AckObservation: safe,
			ActorID: "owner",
		})
		if err != nil {
			t.Fatalf("commit StageLaser scope %s: %v", targetSnapshot, err)
		}
		return record
	}

	first := commit(
		"41111111-1111-4111-8111-111111111111",
		strings.Repeat("1a", 32),
		"", "", snapshotA, 1, 10,
	)
	if first.ToEpoch != 2 || first.ToRuntimeSnapshotID != snapshotA {
		t.Fatalf("first assignment=%+v", first)
	}

	preflight, err := repo.PreflightStageLaserAssignment(ctx, deviceexperience.StageLaserAssignmentInput{
		DeviceID: deviceID,
		ExpectedProjectID: projectID,
		ExpectedRuntimeSnapshotID: snapshotA,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: snapshotB,
		ExpectedEpoch: 2,
	})
	if err != nil || preflight.AssignmentState != "ACTIVE" ||
		preflight.FromRuntimeSnapshotID != snapshotA ||
		preflight.ToRuntimeSnapshotID != snapshotB ||
		preflight.RequiredAction != "AUTHENTICATED_STAGELASER_SAFE_OFF_ACK" {
		t.Fatalf("StageLaser rebind preflight=%+v err=%v", preflight, err)
	}

	second := commit(
		"41111111-1111-4111-8111-111111111112",
		strings.Repeat("2b", 32),
		projectID, snapshotA, snapshotB, 2, 11,
	)
	if second.FromEpoch != 2 || second.ToEpoch != 3 ||
		second.FromRuntimeSnapshotID != snapshotA ||
		second.ToRuntimeSnapshotID != snapshotB {
		t.Fatalf("StageLaser rebind commit=%+v", second)
	}
	assignment, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || assignment.State != "ACTIVE" ||
		assignment.ProjectID != projectID ||
		assignment.RuntimeSnapshotID != snapshotB ||
		assignment.Epoch != 3 {
		t.Fatalf("rebound assignment=%+v err=%v", assignment, err)
	}

	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID,
		RuntimeSnapshotID: snapshotA,
		DeviceID: deviceID,
		CommandType: stagelaser.CommandArm,
		Issuer: "operator:test",
		Payload: json.RawMessage(`{}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("old snapshot retained StageLaser command authority: %v", err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID,
		RuntimeSnapshotID: snapshotB,
		DeviceID: deviceID,
		CommandType: stagelaser.CommandArm,
		Issuer: "operator:test",
		Payload: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatalf("new snapshot did not receive StageLaser command authority: %v", err)
	}
}

func TestStageLaserRebindRejectsCrossProjectShortcut(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Fixed{Time: phase4Time})
	project, err := stageStore.GetProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Other Laser Project", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "stagelaser-no-cross-project"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, Kind: deviceexperience.DeviceGeneric,
		ProfileID: stagelaser.ProfileID, DisplayName: "Laser",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: stagelaser.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=?, runtime_snapshot_id='source-snapshot', assignment_epoch=2,
		    assignment_state='ACTIVE', updated_at_us=updated_at_us+1
		WHERE device_id=?
	`, project.ID, deviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PreflightStageLaserAssignment(ctx, deviceexperience.StageLaserAssignmentInput{
		DeviceID: deviceID,
		ExpectedProjectID: project.ID,
		ExpectedRuntimeSnapshotID: "source-snapshot",
		TargetProjectID: other.ID,
		TargetRuntimeSnapshotID: "target-snapshot",
		ExpectedEpoch: 2,
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("cross-Project StageLaser shortcut accepted: %v", err)
	}
}
