package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestTransferPreflightRejectsStaleAndUntrustedStateWithoutMutation(t *testing.T) {
	ctx := context.Background()
	repo, handle, targetProjectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Real{})
	secondProject, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Second Show", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	device := deviceexperience.Device{
		ID: "v2-preflight-01", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "New Lighting", ProtocolVersion: deviceexperience.ProtocolVersion2,
		ProfileID: lightingnode.ProfileID, Enabled: true,
	}
	if _, err := repo.RegisterUnassignedV2(ctx, device); err != nil {
		t.Fatal(err)
	}
	intent := deviceexperience.TransferPreflightInput{
		DeviceID: device.ID, ExpectedEpoch: 1, TargetProjectID: targetProjectID,
	}
	result, err := repo.PreflightTransfer(ctx, intent)
	if err != nil || result.AssignmentState != "UNASSIGNED" || result.NextState != "BLOCKED" ||
		result.ToProjectID != targetProjectID || result.RequiredAction != "AUTHENTICATED_BLACKOUT_ACK" {
		t.Fatalf("preflight should only report required handshake: %+v err=%v", result, err)
	}
	record, err := repo.GetAssignmentRecord(ctx, device.ID)
	if err != nil || record.State != "UNASSIGNED" || record.ProjectID != "" || record.Epoch != 1 {
		t.Fatalf("preflight mutated assignment: %+v err=%v", record, err)
	}

	for _, tc := range []struct {
		name string
		intent deviceexperience.TransferPreflightInput
	}{
		{"stale epoch", deviceexperience.TransferPreflightInput{DeviceID: device.ID, ExpectedEpoch: 2, TargetProjectID: targetProjectID}},
		{"wrong previous project", deviceexperience.TransferPreflightInput{DeviceID: device.ID, ExpectedEpoch: 1, ExpectedProjectID: secondProject.ID, TargetProjectID: targetProjectID}},
		{"same project", deviceexperience.TransferPreflightInput{DeviceID: device.ID, ExpectedEpoch: 1}},
		{"missing project", deviceexperience.TransferPreflightInput{DeviceID: device.ID, ExpectedEpoch: 1, TargetProjectID: "missing-project"}},
		{"unknown device", deviceexperience.TransferPreflightInput{DeviceID: "missing", ExpectedEpoch: 1, TargetProjectID: targetProjectID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := repo.PreflightTransfer(ctx, tc.intent); err == nil {
				t.Fatal("unsafe preflight was accepted")
			}
		})
	}
	// A device must not transfer while any old command remains in flight.
	// Simulate a previously accepted command via the legacy repository path;
	// the database fence ensures new commands cannot enter in v2 state.
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET assignment_state='BLOCKED', project_id=?, assignment_epoch=2
		WHERE device_id = ?
	`, secondProject.ID, device.ID); err != nil {
		t.Fatal(err)
	}
	blocked := deviceexperience.TransferPreflightInput{
		DeviceID: device.ID, ExpectedEpoch: 2,
		ExpectedProjectID: secondProject.ID, TargetProjectID: targetProjectID,
	}
	if result, err := repo.PreflightTransfer(ctx, blocked); err != nil || result.FromProjectID != secondProject.ID {
		t.Fatalf("blocked v2 reassignment preflight=%+v err=%v", result, err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET assignment_state='ACTIVE', runtime_snapshot_id=''
		WHERE device_id = ?
	`, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PreflightTransfer(ctx, blocked); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("ACTIVE must remain blocked until v2 command authority is implemented: %v", err)
	}
	// Client updates are still forbidden after the preflight.
	device.DisplayName = "Self claimed"
	if _, err := repo.RegisterUnassignedV2(ctx, device); err == nil {
		t.Fatal("assigned v2 node reconnected via unassigned-only enrollment")
	}
}

func TestTransferPreflightRejectsPendingCommandsAndDisabledDevice(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	device := deviceexperience.Device{
		ID: "pending-v2-preflight", ProjectID: projectID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Legacy Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1, ProfileID: lightingnode.ProfileID, Enabled: true,
		Capabilities: []string{"tablet.media.play"},
	}
	if _, err := repo.UpsertDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	legacyCommand, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		DeviceID: device.ID, ProjectID: projectID,
		CommandType: "TABLET_PLAY", Issuer: "test",
		Payload: json.RawMessage(`{"media":"a.mp4"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an explicitly controlled, future v1 -> v2 migration leaving an
	// accepted old command; no preflight may override that pending command.
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_devices SET protocol_version='stagecore.device/2', project_id=NULL
		WHERE device_id = ?
	`, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET assignment_state='UNASSIGNED', project_id=NULL, assignment_epoch=2
		WHERE device_id = ?
	`, device.ID); err != nil {
		t.Fatal(err)
	}
	intent := deviceexperience.TransferPreflightInput{
		DeviceID: device.ID, ExpectedEpoch: 2, TargetProjectID: projectID,
	}
	if _, err := repo.PreflightTransfer(ctx, intent); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("accepted old command %s was not blocked: %v", legacyCommand.Envelope.CommandID, err)
	}
	if _, err := handle.DB.ExecContext(ctx,
		"UPDATE stage_device_commands SET status='FAILED' WHERE command_id=?",
		legacyCommand.Envelope.CommandID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PreflightTransfer(ctx, intent); err != nil {
		t.Fatalf("no pending command; v2 unassigned preflight should pass: %v", err)
	}
	if _, err := handle.DB.ExecContext(ctx,
		"UPDATE stage_devices SET enabled=0 WHERE device_id=?", device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PreflightTransfer(ctx, intent); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("disabled device accepted transfer: %v", err)
	}
}
