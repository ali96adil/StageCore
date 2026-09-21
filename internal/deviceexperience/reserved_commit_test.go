package deviceexperience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func TestReservationCASAndAuditAreOneAtomicSQLiteCommit(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	const deviceID = "v2-reserved-audit-01"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Lighting", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := repo.ReserveTransferIntent(ctx, deviceexperience.TransferPreflightInput{
		DeviceID: deviceID, TargetProjectID: projectID, ExpectedEpoch: 1,
	}, 7, lightingnode.MaxChannels, "owner")
	if err != nil {
		t.Fatal(err)
	}
	// No process may force PENDING -> COMMITTED by raw SQL without a matching
	// audit, epoch CAS, and the exact reservation.
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_transfer_intents SET status='COMMITTED'
		WHERE transfer_id=?
	`, res.TransferID); err == nil {
		t.Fatal("SQL promoted a software-unverified intent")
	}
	input := deviceexperience.VerifiedTransferInput{
		ReservationID: res.TransferID,
		DeviceID: res.DeviceID, FromProjectID: res.ExpectedProjectID,
		ToProjectID: res.TargetProjectID, ExpectedEpoch: res.ExpectedEpoch,
		ConnectionGeneration: res.ConnectionGeneration, Challenge: res.Challenge,
		AckDeviceID: res.DeviceID, AckEpoch: res.ExpectedEpoch,
		AckGeneration: res.ConnectionGeneration, AckChallenge: res.Challenge,
		AckBlackout: true, AckChannelLevels: make([]uint8, lightingnode.MaxChannels),
		ActorID: "owner", IdempotencyKey: res.TransferID,
	}
	bad := input
	bad.ToProjectID = ""
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, bad); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("reservation allowed wrong target: %v", err)
	}
	bad = input
	bad.Challenge = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bad.AckChallenge = bad.Challenge
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, bad); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("reservation allowed wrong challenge: %v", err)
	}
	var auditCount int
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM stage_device_assignment_transfers WHERE device_id=?", deviceID).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("rejected reservation created audit count=%d err=%v", auditCount, err)
	}
	record, err := repo.CommitVerifiedBlackoutTransfer(ctx, input)
	if err != nil || record.TransferID != res.TransferID || record.ToEpoch != 2 ||
		record.NextState != "BLOCKED" || record.Reused {
		t.Fatalf("reserved commit=%+v err=%v", record, err)
	}
	var status string
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT status FROM stage_device_transfer_intents WHERE transfer_id=?", res.TransferID).Scan(&status); err != nil || status != "COMMITTED" {
		t.Fatalf("intent+audit were not committed atomically status=%s err=%v", status, err)
	}
	record, err = repo.CommitVerifiedBlackoutTransfer(ctx, input)
	if err != nil || !record.Reused || record.TransferID != res.TransferID {
		t.Fatalf("uncertain exact retry changed commit: %+v err=%v", record, err)
	}
	assigned, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || assigned.State != "BLOCKED" || assigned.ProjectID != projectID || assigned.Epoch != 2 {
		t.Fatalf("persisted transfer has wrong assignment: %+v err=%v", assigned, err)
	}
}

func TestExpiredOrCancelledReservationCannotCommitStaleACK(t *testing.T) {
	ctx := context.Background()
	_, handle, projectID := newRepository(t)
	now := time.Now().UTC()
	repo, err := deviceexperience.NewRepository(handle.DB,
		deviceexperience.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	const deviceID = "v2-reserved-expiry-01"
	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID, Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Lighting", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := repo.ReserveTransferIntent(ctx, deviceexperience.TransferPreflightInput{
		DeviceID: deviceID, TargetProjectID: projectID, ExpectedEpoch: 1,
	}, 7, lightingnode.MaxChannels, "owner")
	if err != nil {
		t.Fatal(err)
	}
	input := deviceexperience.VerifiedTransferInput{
		ReservationID: res.TransferID, DeviceID: deviceID,
		ToProjectID: projectID, ExpectedEpoch: 1,
		ConnectionGeneration: 7, Challenge: res.Challenge,
		AckDeviceID: deviceID, AckEpoch: 1, AckGeneration: 7,
		AckChallenge: res.Challenge, AckBlackout: true,
		AckChannelLevels: make([]uint8, lightingnode.MaxChannels),
		ActorID: "owner", IdempotencyKey: res.TransferID,
	}
	now = now.Add(time.Minute)
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, input); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("expired reservation accepted stale ACK: %v", err)
	}
	if err := repo.CancelTransferIntent(ctx, res.TransferID, deviceID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CommitVerifiedBlackoutTransfer(ctx, input); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("cancelled reservation was resumed: %v", err)
	}
	record, err := repo.GetAssignmentRecord(ctx, deviceID)
	if err != nil || record.ProjectID != "" || record.Epoch != 1 || record.State != "UNASSIGNED" {
		t.Fatalf("stale reservation modified state: %+v err=%v", record, err)
	}
}
