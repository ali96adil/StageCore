package deviceexperience_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestLegacyAssignmentSidecarRegistersWithoutActivatingV2(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)

	var version int
	if err := handle.DB.QueryRowContext(ctx, "SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 29 {
		t.Fatalf("assignment migration not installed: version=%d", version)
	}

	device := deviceexperience.Device{
		ID: "lighting-sidecar-01", ProjectID: projectID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Reusable lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1, Enabled: true,
	}
	if _, err := repo.UpsertDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	record, err := repo.GetAssignmentRecord(ctx, device.ID)
	if err != nil || record.ProjectID != projectID || record.Epoch != 1 ||
		record.State != deviceexperience.AssignmentLegacy || record.RuntimeSnapshotID != "" {
		t.Fatalf("legacy registration must not invent v2 authority: %+v err=%v", record, err)
	}
	if record.UpdatedAt.IsZero() {
		t.Fatal("legacy assignment has no update timestamp")
	}
	// A legitimate reconnect is permitted but must not convert LEGACY into ACTIVE.
	device.DisplayName = "Renamed lighting"
	if _, err := repo.UpsertDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	unchanged, err := repo.GetAssignmentRecord(ctx, device.ID)
	if err != nil || unchanged.ProjectID != record.ProjectID ||
		unchanged.Epoch != record.Epoch || unchanged.State != record.State {
		t.Fatalf("reconnect altered metadata: %+v err=%v", unchanged, err)
	}
	device.ProjectID = ""
	if _, err := repo.UpsertDevice(ctx, device); !errors.Is(err, deviceexperience.ErrInvalidDevice) {
		t.Fatalf("client reconnect must not unassign legacy node: %v", err)
	}
	stillOwned, err := repo.GetAssignmentRecord(ctx, device.ID)
	if err != nil || stillOwned.ProjectID != projectID {
		t.Fatalf("rejected hello changed sidecar: %+v err=%v", stillOwned, err)
	}

	unassigned := deviceexperience.Device{
		ID: "unassigned-sidecar-01", Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Unassigned legacy node", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Enabled: true,
	}
	if _, err := repo.UpsertDevice(ctx, unassigned); err != nil {
		t.Fatal(err)
	}
	u, err := repo.GetAssignmentRecord(ctx, unassigned.ID)
	if err != nil || u.ProjectID != "" || u.Epoch != 1 || u.State != deviceexperience.AssignmentLegacy {
		t.Fatalf("unbound registration gained authority: %+v err=%v", u, err)
	}
	if _, err := repo.GetAssignmentRecord(ctx, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown device metadata err=%v", err)
	}
}

func TestAssignmentMetadataSchemaRejectsInvalidTransitionAndEpoch(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	device := deviceexperience.Device{
		ID: "lighting-invariant-01", ProjectID: projectID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1, Enabled: true,
	}
	if _, err := repo.UpsertDevice(ctx, device); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		state string
		project any
		snapshot string
		epoch int64
	}{
		{"zero epoch", "LEGACY", projectID, "", 0},
		{"negative epoch", "LEGACY", projectID, "", -1},
		{"unknown state", "CREATED", projectID, "", 1},
		{"unassigned with project", "UNASSIGNED", projectID, "", 1},
		{"unassigned with snapshot", "UNASSIGNED", nil, "stale-snapshot", 1},
		{"active without project", "ACTIVE", nil, "", 1},
		{"blocked with snapshot", "BLOCKED", projectID, "old-snapshot", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := handle.DB.ExecContext(ctx, `
				UPDATE stage_device_assignments SET
				assignment_state = ?, project_id = ?, runtime_snapshot_id = ?,
				assignment_epoch = ?
				WHERE device_id = ?
			`, tc.state, tc.project, tc.snapshot, tc.epoch, device.ID)
			if err == nil {
				t.Fatal("invalid assignment metadata accepted by database constraint")
			}
		})
	}
	got, err := repo.GetAssignmentRecord(ctx, device.ID)
	if err != nil || got.State != deviceexperience.AssignmentLegacy || got.ProjectID != projectID || got.Epoch != 1 {
		t.Fatalf("invalid attempts changed legacy assignment: %+v err=%v", got, err)
	}
}
