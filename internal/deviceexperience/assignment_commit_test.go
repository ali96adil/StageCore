package deviceexperience_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestVerifiedTransferCASPersistsBlockedEpochAndAuditWithoutAuthorizingCommands(t *testing.T) {
	ctx := context.Background()
	repo, handle, targetID := newRepository(t)
	projects := store.New(handle.DB, clock.Real{})
	second, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Other Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "v2-cas-lighting"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting",
		ProfileID: lightingnode.ProfileID, ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	input := deviceexperience.VerifiedTransferInput{
		DeviceID: deviceID, ToProjectID: targetID, ExpectedEpoch: 1,
		ConnectionGeneration: 7,
		Challenge: strings.Repeat("ab", 32),
		AckDeviceID: deviceID, AckEpoch: 1, AckGeneration: 7,
		AckChallenge: strings.Repeat("ab", 32), AckBlackout: true,
		AckChannelLevels: make([]uint8, lightingnode.MaxChannels),
		ActorID: "owner", IdempotencyKey: "transfer-01",
	}
	first, err := repo.CommitVerifiedBlackoutTransfer(ctx, input)
	if err != nil || first.Reused || first.ToEpoch != 2 || first.NextState != "BLOCKED" || first.TransferID == "" {
		t.Fatalf("commit produced incorrect blocked transfer: %+v err=%v", first, err)
	}
	record, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || record.ProjectID != targetID || record.Epoch != 2 ||
		record.State != "BLOCKED" || record.RuntimeSnapshotID != "" {
		t.Fatalf("committed device became active or lost epoch: %+v err=%v", record, err)
	}
	// The pre-existing v1 row remains project-independent and cannot execute.
	device, err := repo.GetDevice(ctx, deviceID)
	if err != nil || device.ProjectID != "" {
		t.Fatalf("legacy project row was modified: %+v err=%v", device, err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		DeviceID: deviceID, ProjectID: targetID,
		CommandType: lightingnode.CommandBlackout, Issuer: "operator",
	}); err == nil {
		t.Fatal("BLOCKED v2 device accepted ordinary project command")
	}
	listed, err := repo.ListDevices(ctx, targetID)
	if err != nil || len(listed) != 1 || listed[0].ID != deviceID ||
		listed[0].ProjectID != "" || listed[0].Assignment == nil ||
		listed[0].Assignment.ProjectID != targetID ||
		listed[0].Assignment.State != "BLOCKED" {
		t.Fatalf("Hub-owned target inventory did not display blocked device: %+v err=%v", listed, err)
	}
	var count int
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM stage_device_assignment_transfers WHERE device_id=?", deviceID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("missing atomic transfer audit: count=%d err=%v", count, err)
	}
	var rawChallenge string
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT challenge_sha256 FROM stage_device_assignment_transfers WHERE transfer_id=?", first.TransferID).Scan(&rawChallenge); err != nil ||
		rawChallenge == input.Challenge || len(rawChallenge) != 64 {
		t.Fatalf("raw challenge was stored or transfer lookup failed: %q err=%v", rawChallenge, err)
	}
	retry, err := repo.CommitVerifiedBlackoutTransfer(ctx, input)
	if err != nil || !retry.Reused || retry.TransferID != first.TransferID {
		t.Fatalf("uncertain commit replay differed: %+v err=%v", retry, err)
	}
	tampered := input
	tampered.ActorID = "another-actor"
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, tampered); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("idempotency key was reused by different actor: %v", err)
	}
	tampered = input
	tampered.IdempotencyKey = "second-key"
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, tampered); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("stale first epoch was accepted: %v", err)
	}
	// The same node is reusable in another Project without any NVS/project
	// bootstrap change. Commit remains BLOCKED pending actual node epoch ACK.
	toSecond := input
	toSecond.FromProjectID = targetID
	toSecond.ToProjectID = second.ID
	toSecond.ExpectedEpoch = 2
	toSecond.AckEpoch = 2
	toSecond.Challenge = strings.Repeat("cd", 32)
	toSecond.AckChallenge = toSecond.Challenge
	toSecond.IdempotencyKey = "transfer-02"
	reassigned, err := repo.CommitVerifiedBlackoutTransfer(ctx, toSecond)
	if err != nil || reassigned.FromEpoch != 2 || reassigned.ToEpoch != 3 || reassigned.NextState != "BLOCKED" {
		t.Fatalf("second Project transfer failed: %+v err=%v", reassigned, err)
	}
	oldInventory, err := repo.ListDevices(ctx, targetID)
	if err != nil || len(oldInventory) != 0 {
		t.Fatalf("old Project still lists transferred device: %+v err=%v", oldInventory, err)
	}
	newInventory, err := repo.ListDevices(ctx, second.ID)
	if err != nil || len(newInventory) != 1 || newInventory[0].Assignment == nil ||
		newInventory[0].Assignment.ProjectID != second.ID || newInventory[0].Assignment.Epoch != 3 {
		t.Fatalf("new Project inventory missing Hub-assigned device: %+v err=%v", newInventory, err)
	}
	// Unassign persists the next epoch; old project and old snapshot cannot
	// regain authority just because a v1 client or API returns.
	unassign := toSecond
	unassign.FromProjectID = second.ID
	unassign.ToProjectID = ""
	unassign.ExpectedEpoch = 3
	unassign.AckEpoch = 3
	unassign.Challenge = strings.Repeat("ef", 32)
	unassign.AckChallenge = unassign.Challenge
	unassign.IdempotencyKey = "transfer-03"
	cleared, err := repo.CommitVerifiedBlackoutTransfer(ctx, unassign)
	if err != nil || cleared.NextState != "UNASSIGNED" || cleared.ToEpoch != 4 {
		t.Fatalf("unassign transfer failed: %+v err=%v", cleared, err)
	}
	record, err = repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || record.ProjectID != "" || record.State != "UNASSIGNED" || record.Epoch != 4 {
		t.Fatalf("unassigned state retained old project: %+v err=%v", record, err)
	}
}

func TestVerifiedTransferCASRejectsForgedIncompleteBlackoutAndInvalidConditions(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	const deviceID = "v2-cas-invalid"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, Kind: deviceexperience.DeviceGeneric, DisplayName: "Node",
		ProfileID: lightingnode.ProfileID, ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	baseline := deviceexperience.VerifiedTransferInput{
		DeviceID: deviceID, ToProjectID: projectID, ExpectedEpoch: 1, ConnectionGeneration: 1,
		Challenge: strings.Repeat("12", 32), AckDeviceID: deviceID, AckEpoch: 1,
		AckGeneration: 1, AckChallenge: strings.Repeat("12", 32), AckBlackout: true,
		AckChannelLevels: make([]uint8, lightingnode.MaxChannels),
		ActorID: "owner", IdempotencyKey: "test-invalid",
	}
	for _, tc := range []struct {
		name string
		change func(*deviceexperience.VerifiedTransferInput)
	}{
		{"fake acknowledged blackout", func(i *deviceexperience.VerifiedTransferInput) { i.AckBlackout = false }},
		{"partial channels", func(i *deviceexperience.VerifiedTransferInput) { i.AckChannelLevels = make([]uint8, lightingnode.MaxChannels-1) }},
		{"wrong device", func(i *deviceexperience.VerifiedTransferInput) { i.AckDeviceID = "another" }},
		{"old generation", func(i *deviceexperience.VerifiedTransferInput) { i.AckGeneration = 0 }},
		{"old epoch", func(i *deviceexperience.VerifiedTransferInput) { i.AckEpoch = 0 }},
		{"wrong challenge", func(i *deviceexperience.VerifiedTransferInput) { i.AckChallenge = strings.Repeat("34", 32) }},
		{"noncanonical challenge", func(i *deviceexperience.VerifiedTransferInput) { i.Challenge = "short"; i.AckChallenge = "short" }},
		{"nonzero output", func(i *deviceexperience.VerifiedTransferInput) { i.AckChannelLevels[5] = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := baseline
			candidate.AckChannelLevels = append([]uint8(nil), baseline.AckChannelLevels...)
			tc.change(&candidate)
			if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, candidate); !errors.Is(err, deviceexperience.ErrInvalidState) {
				t.Fatalf("untrusted ack was committed: %v", err)
			}
		})
	}
	var count int
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM stage_device_assignment_transfers").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected acks left audit records: count=%d err=%v", count, err)
	}
	if _, err := handle.DB.ExecContext(ctx, "UPDATE stage_devices SET enabled=0 WHERE device_id=?", deviceID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, baseline); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("disabled node transferred: %v", err)
	}
	var state string
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT assignment_state FROM stage_device_assignments WHERE device_id=?", deviceID).Scan(&state); err != nil ||
		state != "UNASSIGNED" {
		t.Fatalf("rejected transfer changed assignment: state=%s err=%v", state, err)
	}
	var absent string
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT transfer_id FROM stage_device_assignment_transfers LIMIT 1").Scan(&absent); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rejected transfer created audit entry: %s err=%v", absent, err)
	}
}

func TestVerifiedTransferCASRechecksActiveSHOWInsideCommit(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Real{})
	project, err := stageStore.GetProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, project.CurrentRevisionID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	show, err := stageStore.CreateSession(ctx, published.ID, domain.SessionShow, "live show")
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "v2-cas-show-guard"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting",
		ProfileID: lightingnode.ProfileID, ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	in := deviceexperience.VerifiedTransferInput{
		DeviceID: deviceID, ToProjectID: projectID, ExpectedEpoch: 1,
		ConnectionGeneration: 1, Challenge: strings.Repeat("66", 32),
		AckDeviceID: deviceID, AckEpoch: 1, AckGeneration: 1,
		AckChallenge: strings.Repeat("66", 32), AckBlackout: true,
		AckChannelLevels: make([]uint8, lightingnode.MaxChannels),
		ActorID: "owner", IdempotencyKey: "show-check",
	}
	if _, err := repo.PreflightTransfer(ctx, deviceexperience.TransferPreflightInput{
		DeviceID: deviceID, ExpectedEpoch: 1, TargetProjectID: projectID,
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("active SHOW unexpectedly passed preflight: %v", err)
	}
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, in); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("active SHOW unexpectedly permitted audit commit: %v", err)
	}
	rec, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || rec.State != "UNASSIGNED" || rec.Epoch != 1 || rec.ProjectID != "" {
		t.Fatalf("SHOW-rejected commit mutated assignment: %+v err=%v", rec, err)
	}
	if err := stageStore.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, in); err != nil {
		t.Fatalf("transfer should be possible only after SHOW exits: %v", err)
	}
}
