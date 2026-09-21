package deviceexperience_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func TestTransferReservationPersistsHashedSingleUseAttemptWithoutProjectMutation(t *testing.T) {
	ctx := context.Background()
	_, handle, target := newRepository(t)
	now := time.Now().UTC()
	repo, err := deviceexperience.NewRepository(handle.DB,
		deviceexperience.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	device := deviceexperience.Device{
		ID: "transfer-reservation-01", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Lighting", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}
	if _, err := repo.RegisterUnassignedV2(ctx, device); err != nil {
		t.Fatal(err)
	}
	intent := deviceexperience.TransferPreflightInput{
		DeviceID: device.ID, TargetProjectID: target, ExpectedEpoch: 1,
	}
	for _, tc := range []struct {
		name string
		generation int64
		channels uint16
		actor string
	}{
		{"missing generation", 0, 12, "owner"},
		{"invalid channels", 1, 0, "owner"},
		{"channels overflow", 1, 513, "owner"},
		{"missing actor", 1, 12, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := repo.ReserveTransferIntent(ctx, intent, tc.generation, tc.channels, tc.actor); !errors.Is(err, deviceexperience.ErrInvalidState) {
				t.Fatalf("invalid reservation accepted: %v", err)
			}
		})
	}
	res, err := repo.ReserveTransferIntent(ctx, intent, 7, 12, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if res.TransferID == "" || len(res.Challenge) != 64 ||
		res.TargetProjectID != target || res.ExpectedEpoch != 1 ||
		res.ConnectionGeneration != 7 || res.ExpectedChannels != 12 {
		t.Fatalf("invalid reservation response: %+v", res)
	}
	var storedHash, status, from, to string
	var epoch, generation, channels int64
	if err := handle.DB.QueryRowContext(ctx, `
		SELECT challenge_sha256, status, COALESCE(from_project_id,''),
		       COALESCE(to_project_id,''), expected_epoch,
		       connection_generation, expected_channels
		FROM stage_device_transfer_intents WHERE transfer_id = ?
	`, res.TransferID).Scan(&storedHash, &status, &from, &to, &epoch, &generation, &channels); err != nil {
		t.Fatal(err)
	}
	nonce, err := hex.DecodeString(res.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(nonce)
	if storedHash == res.Challenge || storedHash != hex.EncodeToString(digest[:]) ||
		status != "PENDING" || from != "" || to != target ||
		epoch != 1 || generation != 7 || channels != 12 {
		t.Fatalf("reservation metadata or hash invalid: status=%s from=%q to=%q epoch=%d generation=%d channels=%d hash=%q", status, from, to, epoch, generation, channels, storedHash)
	}
	assignment, err := repo.GetAssignmentRecord(ctx, device.ID)
	if err != nil || assignment.State != "UNASSIGNED" || assignment.ProjectID != "" ||
		assignment.Epoch != 1 {
		t.Fatalf("recording intent changed assignment: %+v err=%v", assignment, err)
	}
	if _, err := repo.ReserveTransferIntent(ctx, intent, 8, 12, "owner"); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("concurrent pending intent not rejected: %v", err)
	}
	// No code in this slice can promote PENDING to a device authorization,
	// even if a caller can issue raw SQL.
	if _, err := handle.DB.ExecContext(ctx,
		"UPDATE stage_device_transfer_intents SET status='COMMITTED' WHERE transfer_id=?", res.TransferID); err == nil {
		t.Fatal("raw SQL promoted unverified blackout into COMMITTED")
	}
	// Time expiry releases the reservation uniqueness slot and retains the
	// old challenge only as a non-executable hash in the historical row.
	now = now.Add(time.Minute)
	retry, err := repo.ReserveTransferIntent(ctx, intent, 9, 12, "owner")
	if err != nil || retry.TransferID == res.TransferID || retry.Challenge == res.Challenge {
		t.Fatalf("expired reservation not safely replaced: %+v err=%v", retry, err)
	}
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT status FROM stage_device_transfer_intents WHERE transfer_id=?", res.TransferID).Scan(&status); err != nil || status != "EXPIRED" {
		t.Fatalf("old intent status=%s err=%v", status, err)
	}
}

func TestTransferIntentSQLRejectsStaleEpoch(t *testing.T) {
	ctx := context.Background()
	repo, handle, target := newRepository(t)
	device := deviceexperience.Device{
		ID: "transfer-sql-guard-01", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Lighting", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}
	if _, err := repo.RegisterUnassignedV2(ctx, device); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().UnixMicro()
	for _, tc := range []struct {
		name string
		id string
		epoch int64
	}{
		{"old epoch", "33333333-3333-4333-8333-333333333333", 2},
		{"zero epoch", "44444444-4444-4444-8444-444444444444", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handle.DB.ExecContext(ctx, `
				INSERT INTO stage_device_transfer_intents
				(transfer_id,device_id,to_project_id,expected_epoch,connection_generation,
				 expected_channels,challenge_sha256,status,requested_by,
				 created_at_us,expires_at_us,updated_at_us)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			`, tc.id, device.ID,
				target, tc.epoch, 1, 12,
				"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"PENDING", "owner", now, now+30_000_000, now)
			if err == nil {
				t.Fatal("raw SQL bypassed assignment epoch")
			}
		})
	}
	var count int
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM stage_device_transfer_intents WHERE device_id=?", device.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected raw SQL changed intent rows: count=%d err=%v", count, err)
	}
}

func TestV2ProjectDeleteRequiresUnassignmentWithoutHistoricalIntentFK(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	device := deviceexperience.Device{
		ID: "v2-project-lifetime-01", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Reusable Lighting", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2, Enabled: true,
	}
	if _, err := repo.RegisterUnassignedV2(ctx, device); err != nil {
		t.Fatal(err)
	}
	intent := deviceexperience.TransferPreflightInput{
		DeviceID: device.ID, TargetProjectID: projectID, ExpectedEpoch: 1,
	}
	res, err := repo.ReserveTransferIntent(ctx, intent, 1, lightingnode.MaxChannels, "owner")
	if err != nil {
		t.Fatal(err)
	}
	// Assert the targeted invariant directly: historical from/to references
	// are text, not FKs that pin a Project forever. Other product records
	// (such as revisions) may independently restrict whole-Project deletion.
	rows, err := handle.DB.QueryContext(ctx, "PRAGMA foreign_key_list(stage_device_transfer_intents)")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if from == "from_project_id" || from == "to_project_id" {
			rows.Close()
			t.Fatalf("historical transfer Project reference is restrictive FK: from=%q table=%q", from, table)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	var retained string
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT to_project_id FROM stage_device_transfer_intents WHERE transfer_id=?",
		res.TransferID).Scan(&retained); err != nil || retained != projectID {
		t.Fatalf("historical transfer intent lost target: %q err=%v", retained, err)
	}

	// Once a Project is the actual v2 sidecar owner, only the explicit
	// unassignment flow (new epoch + verified blackout) may release it.
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=?, assignment_state='BLOCKED', assignment_epoch=2
		WHERE device_id=?
	`, projectID, device.ID); err != nil {
		t.Fatal(err)
	}
	_, err = handle.DB.ExecContext(ctx, "DELETE FROM projects WHERE project_id=?", projectID)
	if err == nil || !strings.Contains(err.Error(), "STAGE_DEVICE_PROJECT_ASSIGNED") {
		t.Fatalf("v2 Project deletion was not fenced by Stage Device trigger: %v", err)
	}
	record, err := repo.GetAssignmentRecord(ctx, device.ID)
	if err != nil || record.ProjectID != projectID || record.State != "BLOCKED" || record.Epoch != 2 {
		t.Fatalf("failed delete changed v2 assignment: %+v err=%v", record, err)
	}
	// Database-only simulation of an explicit unassign; not an assertion
	// that any physical strip has gone dark. The Stage Device-specific guard
	// must then release, regardless of any unrelated Project lifecycle FKs.
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=NULL, assignment_state='UNASSIGNED', assignment_epoch=3
		WHERE device_id=?
	`, device.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, "DELETE FROM projects WHERE project_id=?", projectID);
		err != nil && strings.Contains(err.Error(), "STAGE_DEVICE_PROJECT_ASSIGNED") {
		t.Fatalf("unassigned v2 node still blocked Project deletion: %v", err)
	}
}
