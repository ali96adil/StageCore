package deviceexperience_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

func TestLegacyLightingMigrationFencesV1AndAllowsV2Reconnect(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	const deviceID = "legacy-lighting-v2-migration"

	legacy := deviceexperience.Device{
		ID: deviceID,
		ProjectID: projectID,
		ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Front lighting",
		Platform: "esp32",
		Architecture: "xtensa",
		ClientVersion: "0.2.0-dev.1",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"lighting.blackout"},
		Enabled: true,
	}
	if _, err := repo.UpsertDevice(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	migration, err := repo.MigrateLegacyLightingToV2Blocked(
		ctx, deviceID, projectID, "owner-test",
	)
	if err != nil {
		t.Fatal(err)
	}
	if migration.DeviceID != deviceID || migration.ProjectID != projectID ||
		migration.FromEpoch != 1 || migration.ToEpoch != 2 ||
		migration.NextState != "BLOCKED" {
		t.Fatalf("migration=%+v", migration)
	}

	loaded, err := repo.GetDevice(ctx, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProjectID != "" ||
		loaded.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
		loaded.Assignment == nil ||
		loaded.Assignment.State != "BLOCKED" ||
		loaded.Assignment.ProjectID != projectID ||
		loaded.Assignment.Epoch != 2 ||
		loaded.Assignment.RuntimeSnapshotID != "" {
		t.Fatalf("migrated device=%+v", loaded)
	}

	legacy.DisplayName = "stale v1 reconnect"
	if _, err := repo.UpsertDevice(ctx, legacy); err == nil {
		t.Fatal("legacy reconnect regained authority after v2 migration")
	}

	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID,
		DeviceID: deviceID,
		CommandType: "LIGHTING_BLACKOUT",
		Issuer: "operator:test",
	}); err == nil {
		t.Fatal("legacy command escaped BLOCKED v2 migration fence")
	}

	reconnected, err := repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: deviceID,
		ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Front lighting",
		Platform: "esp32",
		Architecture: "xtensa",
		ClientVersion: "0.2.0-dev.1",
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Capabilities: []string{
			"lighting.state.read",
			"lighting.blackout",
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("v2 reconnect after migration failed: %v", err)
	}
	if reconnected.ProjectID != "" ||
		reconnected.Assignment == nil ||
		reconnected.Assignment.State != "BLOCKED" ||
		reconnected.Assignment.Epoch != 2 {
		t.Fatalf("v2 reconnect changed migration scope: %+v", reconnected)
	}

	var audits int
	if err := handle.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM stage_device_legacy_v2_migrations WHERE device_id=?",
		deviceID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("migration audit count=%d", audits)
	}

	if _, err := repo.MigrateLegacyLightingToV2Blocked(
		ctx, deviceID, projectID, "owner-test",
	); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("repeat migration err=%v", err)
	}
}
