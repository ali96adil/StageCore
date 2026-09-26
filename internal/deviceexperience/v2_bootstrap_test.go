package deviceexperience_test

import (
	"context"
	"slices"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func TestV2ReconnectRefreshesOnlyAuthenticatedSoftwareMetadata(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)

	device := deviceexperience.Device{
		ID:              "v2-capability-refresh-01",
		Kind:            deviceexperience.DeviceGeneric,
		DisplayName:     "Reusable Lighting",
		ProfileID:       lightingnode.ProfileID,
		Platform:        "esp32",
		Architecture:    "xtensa",
		ClientVersion:   "0.2.0-blackout",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities:    []string{"lighting.state_probe/1"},
		GroupName:       "Lighting",
		LocationName:    "Front",
		Enabled:         true,
	}
	first, err := repo.RegisterUnassignedV2(ctx, device)
	if err != nil {
		t.Fatal(err)
	}
	if first.Assignment == nil || first.Assignment.State != "UNASSIGNED" {
		t.Fatalf("initial v2 assignment=%+v", first.Assignment)
	}

	// Model a Hub-committed ACTIVE scope. Reconnect must not change it.
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET project_id=?, assignment_epoch=2, assignment_state='ACTIVE',
		    runtime_snapshot_id='snapshot-active-v2', updated_at_us=updated_at_us+1
		WHERE device_id=? AND assignment_state='UNASSIGNED'
	`, projectID, device.ID); err != nil {
		t.Fatal(err)
	}

	reconnect := device
	reconnect.DisplayName = "Client attempted rename"
	reconnect.GroupName = "Client attempted group"
	reconnect.LocationName = "Client attempted location"
	reconnect.ClientVersion = "0.3.0-active"
	reconnect.Capabilities = []string{
		"lighting.channels.set",
		"lighting.channels.fade",
		"lighting.blackout",
		"lighting.identify",
		"lighting.state.read",
		"lighting.config.read",
		"lighting.config.apply",
		"lighting.state_probe/1",
	}
	loaded, err := repo.RegisterUnassignedV2(ctx, reconnect)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.DisplayName != device.DisplayName ||
		loaded.GroupName != device.GroupName ||
		loaded.LocationName != device.LocationName {
		t.Fatalf("v2 reconnect changed Hub-owned display metadata: %+v", loaded)
	}
	if loaded.ClientVersion != reconnect.ClientVersion ||
		len(loaded.Capabilities) != len(reconnect.Capabilities) {
		t.Fatalf("v2 software metadata did not refresh: version=%q caps=%v",
			loaded.ClientVersion, loaded.Capabilities)
	}
	for _, capability := range reconnect.Capabilities {
		if !slices.Contains(loaded.Capabilities, capability) {
			t.Fatalf("v2 software metadata missing capability %q: %v",
				capability, loaded.Capabilities)
		}
	}
	if loaded.Assignment == nil ||
		loaded.Assignment.State != "ACTIVE" ||
		loaded.Assignment.ProjectID != projectID ||
		loaded.Assignment.RuntimeSnapshotID != "snapshot-active-v2" ||
		loaded.Assignment.Epoch != 2 {
		t.Fatalf("v2 reconnect changed Hub-owned ACTIVE scope: %+v", loaded.Assignment)
	}
}
