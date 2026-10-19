package projectaccess_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	testTabletProfile   = "stagecore.tablet-player"
	testLightingProfile = "stagecore.esp32-dmx-lighting-node"
	testLaserProfile    = "stagecore.esp32-stagelaser"
)

type v2TestDevice struct {
	id, kind, profile string
}

func insertV2TestDevice(t *testing.T, f *fixture, d v2TestDevice) {
	t.Helper()
	ctx := context.Background()
	// Test-only SQL setup: NOT a simulated authenticated activation protocol.
	// The read resolver must deny ACTIVE metadata until the exact audit exists.
	_, err := f.db.DB.ExecContext(ctx, `
		INSERT INTO stage_devices
		(device_id, project_id, profile_id, device_kind, display_name,
		 protocol_version, enabled, created_at_us, updated_at_us)
		VALUES (?, NULL, ?, ?, ?, 'stagecore.device/2', 1, 1, 1)
	`, d.id, d.profile, d.kind, d.id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.db.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET assignment_state = 'UNASSIGNED', updated_at_us = 2
		WHERE device_id = ? AND assignment_state = 'LEGACY' AND project_id IS NULL
	`, d.id)
	if err != nil {
		t.Fatal(err)
	}
}

func assignV2TestDevice(t *testing.T, f *fixture, id, projectID, snapshotID string, epoch int64) {
	t.Helper()
	_, err := f.db.DB.ExecContext(context.Background(), `
		UPDATE stage_device_assignments
		SET assignment_state = 'ACTIVE', project_id = ?,
		    runtime_snapshot_id = ?, assignment_epoch = ?, updated_at_us = ?
		WHERE device_id = ?
	`, projectID, snapshotID, epoch, epoch+1, id)
	if err != nil {
		t.Fatal(err)
	}
}

// addV2TestAudit models only the persisted output of a trusted safe-state
// handshake. It is intentionally not an operational activation.
func addV2TestAudit(t *testing.T, f *fixture, d v2TestDevice, projectID, snapshotID string, epoch int64) {
	t.Helper()
	ctx := context.Background()
	auditID := fmt.Sprintf("11111111-2222-4333-8444-%012d", epoch*100+int64(len(d.id)))
	hash := strings.Repeat(string(rune('a'+epoch-2)), 64)
	// This test only needs a unique legal UUID-shaped ID per device/epoch.
	// The tuple is distinguished by a deterministic profile-specific suffix.
	switch d.profile {
	case testTabletProfile:
		_, err := f.db.DB.ExecContext(ctx, `
			INSERT INTO stage_device_tablet_assignment_audit
			(assignment_id, device_id, actor_id, from_project_id, to_project_id,
			 from_runtime_snapshot_id, to_runtime_snapshot_id,
			 from_epoch, to_epoch, connection_generation, challenge_sha256,
			 next_state, committed_at_us)
			VALUES (?, ?, 'owner', '', ?, '', ?, ?, ?, 1, ?, 'ACTIVE', ?)
		`, auditID, d.id, projectID, snapshotID, epoch-1, epoch, hash, epoch)
		if err != nil {
			t.Fatal(err)
		}
	case testLightingProfile:
		_, err := f.db.DB.ExecContext(ctx, `
			INSERT INTO stage_device_lighting_activation_audit
			(activation_id, device_id, actor_id, project_id, runtime_snapshot_id,
			 assignment_epoch, connection_generation, challenge_sha256,
			 configuration_sha256, committed_at_us)
			VALUES (?, ?, 'owner', ?, ?, ?, 1, ?, ?, ?)
		`, auditID, d.id, projectID, snapshotID, epoch, hash, strings.Repeat("b", 64), epoch)
		if err != nil {
			t.Fatal(err)
		}
	case testLaserProfile:
		_, err := f.db.DB.ExecContext(ctx, `
			INSERT INTO stage_device_stagelaser_assignment_audit
			(assignment_id, device_id, actor_id, project_id, runtime_snapshot_id,
			 from_epoch, to_epoch, connection_generation, challenge_sha256,
			 state_quality, committed_at_us)
			VALUES (?, ?, 'owner', ?, ?, ?, ?, 1, ?, 'CONFIRMED', ?)
		`, auditID, d.id, projectID, snapshotID, epoch-1, epoch, hash, epoch)
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("unknown profile in test")
	}
}

func createV2TestSnapshots(t *testing.T, f *fixture) (string, string) {
	t.Helper()
	ctx := context.Background()
	st := store.New(f.db.DB, clock.Real{})
	a, err := st.GetProject(ctx, f.projectA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.GetProject(ctx, f.projectB)
	if err != nil {
		t.Fatal(err)
	}
	snapA := "31111111-2222-4333-8444-555555555551"
	snapB := "31111111-2222-4333-8444-555555555552"
	for i, row := range []struct{ project, revision, snapshot string }{
		{f.projectA, a.CurrentRevisionID, snapA},
		{f.projectB, b.CurrentRevisionID, snapB},
	} {
		_, err := f.db.DB.ExecContext(ctx, `
			INSERT INTO runtime_snapshots
			(runtime_snapshot_id, project_id, revision_id, snapshot_version,
			 created_at_us, created_by, content_hash, manifest_json, status)
			VALUES (?, ?, ?, 1, 1, 'test', ?, '{}', 'PUBLISHED')
		`, row.snapshot, row.project, row.revision,
			strings.Repeat(string(rune('a'+i)), 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	return snapA, snapB
}

func TestV2ScopedDeviceReadRequiresActivePublishedScopeAndMatchingAudit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	snapA, snapB := createV2TestSnapshots(t, f)
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []v2TestDevice{
		{"tablet-safe-A", "TABLET_PLAYER", testTabletProfile},
		{"lighting-safe-A", "GENERIC", testLightingProfile},
		{"laser-safe-A", "GENERIC", testLaserProfile},
	} {
		t.Run(d.profile, func(t *testing.T) {
			insertV2TestDevice(t, f, d)
			if err := f.a.CheckActiveV2DeviceRead(ctx, f.operatorID, f.projectA, d.id);
				!errors.Is(err, projectaccess.ErrDenied) {
				t.Fatalf("unassigned V2 exposed: %v", err)
			}
			assignV2TestDevice(t, f, d.id, f.projectA, snapA, 2)
			if err := f.a.CheckActiveV2DeviceRead(ctx, f.operatorID, f.projectA, d.id);
				!errors.Is(err, projectaccess.ErrDenied) {
				t.Fatalf("ACTIVE without audited safe assignment exposed: %v", err)
			}
			addV2TestAudit(t, f, d, f.projectA, snapA, 2)
			resolved, err := f.a.ResolveActiveV2DeviceProject(ctx, d.id)
			if err != nil || resolved != f.projectA {
				t.Fatalf("valid V2 device Project = %s, %v", resolved, err)
			}
			if err := f.a.CheckActiveV2DeviceRead(ctx, f.operatorID, f.projectA, d.id); err != nil {
				t.Fatalf("Project A scoped read denied: %v", err)
			}
			if err := f.a.CheckActiveV2DeviceRead(ctx, f.operatorID, f.projectB, d.id);
				!errors.Is(err, projectaccess.ErrDenied) {
				t.Fatalf("Project A device exposed through B URL: %v", err)
			}
			if err := f.a.CheckActiveV2DeviceRead(ctx, f.ownerID, f.projectA, d.id); err != nil {
				t.Fatalf("Owner read unexpectedly denied: %v", err)
			}

			// Force a Project transfer in test SQL without the next safe
			// handshake: previously valid audit must instantly go stale.
			assignV2TestDevice(t, f, d.id, f.projectB, snapB, 3)
			if _, err := f.a.ResolveActiveV2DeviceProject(ctx, d.id);
				!errors.Is(err, projectaccess.ErrDenied) {
				t.Fatalf("stale epoch-2 audit authorized new epoch 3: %v", err)
			}
			addV2TestAudit(t, f, d, f.projectB, snapB, 3)
			if err := f.a.CheckActiveV2DeviceRead(ctx, f.operatorID, f.projectB, d.id);
				!errors.Is(err, projectaccess.ErrDenied) {
				t.Fatalf("Project A operator read newly assigned B device: %v", err)
			}
			if err := f.a.CheckActiveV2DeviceRead(ctx, f.ownerID, f.projectB, d.id); err != nil {
				t.Fatalf("Owner lost new Project B device: %v", err)
			}
			// BLOCKED is never an active ordinary Project device read.
			_, err = f.db.DB.ExecContext(ctx, `
				UPDATE stage_device_assignments
				SET assignment_state = 'BLOCKED', runtime_snapshot_id = ''
				WHERE device_id = ?
			`, d.id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.a.ResolveActiveV2DeviceProject(ctx, d.id);
				!errors.Is(err, projectaccess.ErrDenied) {
				t.Fatalf("BLOCKED device exposed: %v", err)
			}
		})
	}
}

func TestV2DeviceScopeDeniesUnknownLegacyDisabledAndCrossProjectSnapshot(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	snapA, snapB := createV2TestSnapshots(t, f)
	if _, err := f.a.ResolveActiveV2DeviceProject(ctx, "not-an-identity");
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("unknown device leaked: %v", err)
	}
	if _, err := f.a.ResolveActiveV2DeviceProject(ctx, "");
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("empty device accepted: %v", err)
	}
	d := v2TestDevice{"tablet-mismatched-snapshot", "TABLET_PLAYER", testTabletProfile}
	insertV2TestDevice(t, f, d)
	// Audit and claimed Project both A, but published Snapshot belongs B.
	assignV2TestDevice(t, f, d.id, f.projectA, snapB, 2)
	addV2TestAudit(t, f, d, f.projectA, snapB, 2)
	if _, err := f.a.ResolveActiveV2DeviceProject(ctx, d.id);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("foreign Snapshot accepted: %v", err)
	}
	// Repairing to a matching Snapshot cannot bypass a now-stale audit.
	_, err := f.db.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments SET runtime_snapshot_id = ?
		WHERE device_id = ?
	`, snapA, d.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ResolveActiveV2DeviceProject(ctx, d.id);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("old snapshot audit accepted after correction: %v", err)
	}
	_, err = f.db.DB.ExecContext(ctx, `
		UPDATE stage_devices SET enabled = 0 WHERE device_id = ?
	`, d.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ResolveActiveV2DeviceProject(ctx, d.id);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("disabled device accepted: %v", err)
	}
	// A legacy v1 ProjectID may be operationally meaningful to v1 but must
	// never be mistaken for a qualified v2 ACTIVE assignment.
	_, err = f.db.DB.ExecContext(ctx, `
		INSERT INTO stage_devices
		(device_id, project_id, device_kind, display_name, protocol_version,
		 enabled, created_at_us, updated_at_us)
		VALUES (?, ?, 'GENERIC', 'Legacy', 'stagecore.device/1', 1, 1, 1)
	`, "legacy-project-device", f.projectA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ResolveActiveV2DeviceProject(ctx, "legacy-project-device");
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("legacy project_id mistaken for V2 authority: %v", err)
	}
}
