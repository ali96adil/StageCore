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

	const firstSnapshot = "11111111-2222-4333-8444-555555555551"
	const secondSnapshot = "11111111-2222-4333-8444-555555555552"
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

func TestTabletV2ActiveAssignmentMovesSafelyToAnotherProject(t *testing.T) {
	ctx := context.Background()
	repo, handle, firstProjectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Fixed{Time: phase4Time})
	firstProject, err := stageStore.GetProject(ctx, firstProjectID)
	if err != nil {
		t.Fatal(err)
	}
	secondProject, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Second Tablet Show", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	const firstSnapshot = "21111111-2222-4333-8444-555555555551"
	const secondSnapshot = "21111111-2222-4333-8444-555555555552"
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
			strings.Repeat("b", 64)); err != nil {
			t.Fatal(err)
		}
	}

	const deviceID = "tablet-v2-cross-project-01"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		Kind: deviceexperience.DeviceTabletPlayer,
		ProfileID: deviceexperience.TabletPlayerProfileID,
		DisplayName: "Reusable Tablet",
		Platform: "android",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: []string{"tablet.media.play"},
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	challengeA := strings.Repeat("1a", 32)
	firstCommit, err := repo.CommitTabletSafeAssignment(ctx, deviceexperience.VerifiedTabletAssignmentInput{
		AssignmentID: "21111111-1111-4111-8111-111111111111",
		DeviceID: deviceID,
		TargetProjectID: firstProject.ID,
		TargetRuntimeSnapshotID: firstSnapshot,
		ExpectedEpoch: 1,
		ConnectionGeneration: 10,
		Challenge: challengeA,
		AckDeviceID: deviceID,
		AckEpoch: 1,
		AckGeneration: 10,
		AckChallenge: challengeA,
		AckSafeState: true,
		ActorID: "owner",
	})
	if err != nil || firstCommit.NextState != "ACTIVE" || firstCommit.ToEpoch != 2 {
		t.Fatalf("first assignment=%+v err=%v", firstCommit, err)
	}

	move := deviceexperience.TabletAssignmentInput{
		DeviceID: deviceID,
		ExpectedProjectID: firstProject.ID,
		ExpectedRuntimeSnapshotID: firstSnapshot,
		TargetProjectID: secondProject.ID,
		TargetRuntimeSnapshotID: secondSnapshot,
		ExpectedEpoch: 2,
	}
	preflight, err := repo.PreflightTabletAssignment(ctx, move)
	if err != nil || preflight.AssignmentState != "ACTIVE" ||
		preflight.NextState != "ACTIVE" ||
		preflight.FromProjectID != firstProject.ID ||
		preflight.ToProjectID != secondProject.ID ||
		preflight.RequiredAction != "AUTHENTICATED_TABLET_SAFE_MEDIA_ACK" {
		t.Fatalf("cross-project tablet preflight=%+v err=%v", preflight, err)
	}

	challengeB := strings.Repeat("2b", 32)
	secondCommit, err := repo.CommitTabletSafeAssignment(ctx, deviceexperience.VerifiedTabletAssignmentInput{
		AssignmentID: "21111111-1111-4111-8111-111111111112",
		DeviceID: deviceID,
		ExpectedProjectID: firstProject.ID,
		ExpectedRuntimeSnapshotID: firstSnapshot,
		TargetProjectID: secondProject.ID,
		TargetRuntimeSnapshotID: secondSnapshot,
		ExpectedEpoch: 2,
		ConnectionGeneration: 11,
		Challenge: challengeB,
		AckDeviceID: deviceID,
		AckEpoch: 2,
		AckGeneration: 11,
		AckChallenge: challengeB,
		AckSafeState: true,
		ActorID: "owner",
	})
	if err != nil || secondCommit.NextState != "ACTIVE" ||
		secondCommit.FromEpoch != 2 || secondCommit.ToEpoch != 3 {
		t.Fatalf("cross-project tablet commit=%+v err=%v", secondCommit, err)
	}

	record, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || record.State != "ACTIVE" ||
		record.ProjectID != secondProject.ID ||
		record.RuntimeSnapshotID != secondSnapshot ||
		record.Epoch != 3 {
		t.Fatalf("tablet remained locked to old project: %+v err=%v", record, err)
	}

	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: firstProject.ID,
		RuntimeSnapshotID: firstSnapshot,
		DeviceID: deviceID,
		CommandType: "TABLET_PLAY",
		Issuer: "operator:test",
		Payload: json.RawMessage(`{"media_number":1}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("old Project retained tablet command authority after move: %v", err)
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
	const snapshotID = "11111111-2222-4333-8444-555555555553"
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
		VALUES ('11111111-2222-4333-8444-555555555554', ?, ?, 'SHOW', 'Active show', ?, 'ACTIVE')
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
