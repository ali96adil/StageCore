package snapshot_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestLightingBindingIsRevisionScopedShowLockedAndSnapshotResolved(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	stageStore := store.New(h.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Container Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	binding := lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID,
		Configuration: lightingnode.Configuration{
			SchemaVersion: lightingnode.SchemaVersion1,
			Channels: []lightingnode.ChannelConfig{
				{
					ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Front Warm",
					Kind: lightingnode.ChannelWarmWhite, PhysicalZone: "front",
					MinimumLevel: 0, MaximumLevel: 80, Enabled: true,
				},
				{
					ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Front Cold",
					Kind: lightingnode.ChannelColdWhite, PhysicalZone: "front",
					MinimumLevel: 0, MaximumLevel: 85, Enabled: true,
				},
			},
		},
		Aliases: map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
	}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, binding, "test"); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshot1, manifest1, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if manifest1.SchemaVersion != snapshot.ManifestSchemaVersion {
		t.Fatalf("snapshot schema=%d want=%d", manifest1.SchemaVersion, snapshot.ManifestSchemaVersion)
	}
	resolved1 := manifest1.ResolveLightingChannel("front_warm")
	if resolved1 == nil || resolved1.DeviceID != "lighting-01" || resolved1.ChannelKey != "warm_a" || resolved1.ChannelNumber != 1 {
		t.Fatalf("resolved snapshot channel=%+v", resolved1)
	}

	draft2, err := stageStore.EnsureProjectDraft(ctx, project.ID, "test", "lighting remap")
	if err != nil {
		t.Fatal(err)
	}
	inherited, err := stageStore.ListLightingNodeBindings(ctx, draft2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inherited) != 1 || inherited[0].Aliases["front_warm"] != "warm_a" {
		t.Fatalf("inherited bindings=%+v", inherited)
	}

	show, err := stageStore.CreateSession(ctx, snapshot1.ID, domain.SessionShow, "lighting lock test")
	if err != nil {
		t.Fatal(err)
	}
	remapped := inherited[0]
	remapped.Configuration.Channels[0].ChannelNumber = 4
	if _, err := stageStore.SetLightingNodeBinding(ctx, draft2.ID, remapped, "test"); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("SHOW mutation err=%v", err)
	}
	if _, err := h.DB.ExecContext(ctx,
		"UPDATE lighting_node_revision_bindings SET aliases_json = '{}' WHERE revision_id = ? AND device_id = ?",
		draft2.ID, "lighting-01",
	); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct SQL SHOW mutation err=%v", err)
	}
	if err := stageStore.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}

	if _, err := stageStore.SetLightingNodeBinding(ctx, draft2.ID, remapped, "test"); err != nil {
		t.Fatal(err)
	}
	// The already-published snapshot remains bound to the old physical channel.
	if resolved := manifest1.ResolveLightingChannel("front_warm"); resolved == nil || resolved.ChannelNumber != 1 {
		t.Fatalf("published snapshot mapping changed: %+v", resolved)
	}

	if err := stageStore.SetRevisionStatus(ctx, draft2.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	_, manifest2, err := snapshot.NewBuilder(stageStore).Create(ctx, draft2.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if resolved := manifest2.ResolveLightingChannel("front_warm"); resolved == nil || resolved.ChannelNumber != 4 {
		t.Fatalf("new snapshot did not capture remap: %+v", resolved)
	}
}
