package deviceexperience_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func seedV2Tablet(t *testing.T, online bool) (*deviceexperience.Repository, string, string) {
	t.Helper()
	ctx := context.Background()
	repo, h, projectID := newRepository(t)
	_, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "tablet-v2-stale", ProjectID: projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Old Tablet", Platform: "android", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"tablet.media.live.show"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.DB.ExecContext(ctx, `
		UPDATE stage_devices
		SET project_id = NULL, profile_id = 'stagecore.tablet-player',
		    protocol_version = 'stagecore.device/2'
		WHERE device_id = 'tablet-v2-stale'
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id = ?, assignment_epoch = 2, assignment_state = 'ACTIVE',
		    runtime_snapshot_id = 'snapshot-old'
		WHERE device_id = 'tablet-v2-stale'
	`, projectID); err != nil {
		t.Fatal(err)
	}
	state := deviceexperience.ConnectionOffline
	if online {
		state = deviceexperience.ConnectionOnline
	}
	if _, err := repo.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: "tablet-v2-stale", Connection: state, Readiness: deviceexperience.ReadinessReady,
	}); err != nil {
		t.Fatal(err)
	}
	return repo, projectID, "tablet-v2-stale"
}

func TestDecommissionOfflineTabletPreservesHistoryAndDisablesIdentity(t *testing.T) {
	ctx := context.Background()
	repo, projectID, deviceID := seedV2Tablet(t, false)

	record, err := repo.DecommissionOfflineTablet(ctx, deviceID, "owner-1", "clean reinstall")
	if err != nil {
		t.Fatal(err)
	}
	if record.ProjectID != projectID || record.RuntimeSnapshotID != "snapshot-old" ||
		record.AssignmentState != "ACTIVE" || record.AssignmentEpoch != 2 {
		t.Fatalf("decommission record=%+v", record)
	}

	device, err := repo.GetDevice(ctx, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if device.Enabled {
		t.Fatal("decommissioned Tablet identity remained enabled")
	}
	if device.Runtime == nil || device.Runtime.Connection != deviceexperience.ConnectionRevoked ||
		device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
		t.Fatalf("runtime after decommission=%+v", device.Runtime)
	}
}

func TestDecommissionOnlineTabletFailsClosed(t *testing.T) {
	ctx := context.Background()
	repo, _, deviceID := seedV2Tablet(t, true)

	_, err := repo.DecommissionOfflineTablet(ctx, deviceID, "owner-1", "should fail")
	if !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("online decommission err=%v", err)
	}
	device, loadErr := repo.GetDevice(ctx, deviceID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !device.Enabled {
		t.Fatal("online Tablet was disabled after rejected decommission")
	}
}
