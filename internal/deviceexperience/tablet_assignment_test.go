package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestTabletV2AssignmentUsesPublishedHubSnapshotAndExactCommandScope(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Fixed{Time: phase4Time})
	firstProject, err := stageStore.GetProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	secondProject, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Other Show", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	const firstSnapshot = "tablet-snapshot-project-one"
	const secondSnapshot = "tablet-snapshot-project-two"
	for _, row := range []struct {
		projectID, revisionID, snapshotID string
	}{
		{firstProject.ID, firstProject.CurrentRevisionID, firstSnapshot},
		{secondProject.ID, secondProject.CurrentRevisionID, secondSnapshot},
	} {
		if _, err := handle.DB.ExecContext(ctx, `
			INSERT INTO runtime_snapshots
			(runtime_snapshot_id, project_id, revision_id, snapshot_version,
			 created_at_us, created_by, content_hash, manifest_json, status)
			VALUES (?, ?, ?, 1, ?, 'test', ?, '{}', 'PUBLISHED')
		`, row.snapshotID, row.projectID, row.revisionID, phase4Time.UnixMicro(),
			strings.Repeat("a", 64)); err != nil {
			t.Fatalf("insert published snapshot %s: %v", row.snapshotID, err)
		}
	}

	const deviceID = "tablet-v2-assignment-01"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		Kind: deviceexperience.DeviceTabletPlayer,
		ProfileID: deviceexperience.TabletPlayerProfileID,
		DisplayName: "Tablet 01",
		Platform: "android",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: []string{"tablet.media.play"},
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	// A Snapshot from a different Project can never become authority merely
	// because the caller supplied it in an assignment request.
	if _, err := repo.PreflightTabletAssignment(ctx, deviceexperience.TabletAssignmentInput{
		DeviceID: deviceID,
		ExpectedEpoch: 1,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: secondSnapshot,
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("cross-project Runtime Snapshot accepted: %v", err)
	}

	intent := deviceexperience.TabletAssignmentInput{
		DeviceID: deviceID,
		ExpectedEpoch: 1,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: firstSnapshot,
	}
	preflight, err := repo.PreflightTabletAssignment(ctx, intent)
	if err != nil || preflight.NextState != "ACTIVE" ||
		preflight.RequiredAction != "AUTHENTICATED_TABLET_SAFE_MEDIA_ACK" {
		t.Fatalf("tablet preflight=%+v err=%v", preflight, err)
	}

	challenge := strings.Repeat("0a", 32)
	commit, err := repo.CommitTabletSafeAssignment(ctx, deviceexperience.VerifiedTabletAssignmentInput{
		AssignmentID: "11111111-1111-4111-8111-111111111111",
		DeviceID: deviceID,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: firstSnapshot,
		ExpectedEpoch: 1,
		ConnectionGeneration: 7,
		Challenge: challenge,
		AckDeviceID: deviceID,
		AckEpoch: 1,
		AckGeneration: 7,
		AckChallenge: challenge,
		AckSafeState: true,
		ActorID: "owner",
	})
	if err != nil || commit.NextState != "ACTIVE" || commit.ToEpoch != 2 {
		t.Fatalf("tablet assignment commit=%+v err=%v", commit, err)
	}

	record, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || record.State != "ACTIVE" || record.ProjectID != projectID ||
		record.RuntimeSnapshotID != firstSnapshot || record.Epoch != 2 {
		t.Fatalf("authoritative tablet assignment=%+v err=%v", record, err)
	}

	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID,
		RuntimeSnapshotID: secondSnapshot,
		DeviceID: deviceID,
		CommandType: "TABLET_PLAY",
		Issuer: "operator:test",
		Payload: json.RawMessage(`{"media_number":1}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("wrong Runtime Snapshot command escaped assignment fence: %v", err)
	}

	command, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID,
		RuntimeSnapshotID: firstSnapshot,
		DeviceID: deviceID,
		CommandType: "TABLET_PLAY",
		Issuer: "operator:test",
		Payload: json.RawMessage(`{"media_number":1}`),
	})
	if err != nil {
		t.Fatalf("exact Hub-owned tablet scope rejected: %v", err)
	}
	if command.Envelope.ProjectID != projectID ||
		command.Envelope.RuntimeSnapshotID != firstSnapshot {
		t.Fatalf("command scope=%+v", command.Envelope)
	}
}

func TestTabletAssignmentPreflightRejectsActiveShow(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Fixed{Time: phase4Time})
	project, err := stageStore.GetProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	const snapshotID = "tablet-snapshot-show-lock"
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO runtime_snapshots
		(runtime_snapshot_id, project_id, revision_id, snapshot_version,
		 created_at_us, created_by, content_hash, manifest_json, status)
		VALUES (?, ?, ?, 1, ?, 'test', ?, '{}', 'PUBLISHED')
	`, snapshotID, projectID, project.CurrentRevisionID, phase4Time.UnixMicro(),
		strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO sessions
		(session_id, project_id, runtime_snapshot_id, session_type, name,
		 started_at_us, status)
		VALUES ('tablet-show-lock-session', ?, ?, 'SHOW', 'Active show', ?, 'ACTIVE')
	`, projectID, snapshotID, phase4Time.UnixMicro()); err != nil {
		t.Fatal(err)
	}

	const deviceID = "tablet-v2-show-lock"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		Kind: deviceexperience.DeviceTabletPlayer,
		ProfileID: deviceexperience.TabletPlayerProfileID,
		DisplayName: "Show Locked Tablet",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: []string{"tablet.media.play"},
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PreflightTabletAssignment(ctx, deviceexperience.TabletAssignmentInput{
		DeviceID: deviceID,
		ExpectedEpoch: 1,
		TargetProjectID: projectID,
		TargetRuntimeSnapshotID: snapshotID,
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("tablet assignment changed during active SHOW: %v", err)
	}
}
