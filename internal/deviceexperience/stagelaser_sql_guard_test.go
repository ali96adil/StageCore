package deviceexperience_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/stagelaser"
)

func TestStageLaserSQLCommandGuardRequiresSafeAssignmentAudit(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	const deviceID = "stagelaser-v2-no-audit"
	const snapshotID = "41111111-2222-4333-8444-555555555551"

	if _, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		Kind: deviceexperience.DeviceGeneric,
		ProfileID: stagelaser.ProfileID,
		DisplayName: "Unaudited Laser",
		Platform: "esp32",
		Architecture: "riscv32",
		ClientVersion: "0.1.0-test",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: stagelaser.CapabilityKeys(),
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Model a storage caller trying to manufacture ACTIVE authority without the
	// authenticated DISARMED+OFF assignment handshake/audit.
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=?, runtime_snapshot_id=?, assignment_epoch=2,
		    assignment_state='ACTIVE', updated_at_us=updated_at_us+1
		WHERE device_id=? AND assignment_state='UNASSIGNED'
	`, projectID, snapshotID, deviceID); err != nil {
		t.Fatalf("test setup ACTIVE mutation failed: %v", err)
	}

	_, err := handle.DB.ExecContext(ctx, `
		INSERT INTO stage_device_commands
		(command_id, project_id, device_id, command_type, runtime_snapshot_id,
		 issued_at_us, issuer, priority, payload_json, status)
		VALUES (?, ?, ?, ?, ?, ?, 'test', 'P1', '{}', 'ACCEPTED')
	`,
		"41111111-1111-4111-8111-111111111111",
		projectID, deviceID, stagelaser.CommandSetOn, snapshotID,
		phase4Time.UnixMicro(),
	)
	if err == nil || !strings.Contains(err.Error(), "STAGE_DEVICE_ASSIGNMENT_FENCED") {
		t.Fatalf("unaudited StageLaser command escaped SQLite fence: %v", err)
	}
}
