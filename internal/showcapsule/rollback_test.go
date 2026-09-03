package showcapsule

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestMaterializeRollsBackProjectAndNewVaultObjectsOnGraphCollision(t *testing.T) {
	ctx := context.Background()
	source := newCapsuleFixture(t)
	defer source.db.Close()
	exported, err := source.service.Export(ctx, source.projectID, t.TempDir(), BuildOptions{Mode: ExportSelfContained})
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Manifest.RuntimeSnapshot.Manifest.Targets) != 1 {
		t.Fatalf("fixture targets=%d", len(exported.Manifest.RuntimeSnapshot.Manifest.Targets))
	}
	collidingAliasID := exported.Manifest.RuntimeSnapshot.Manifest.Targets[0].AliasID

	destination := newEmptyCapsuleService(t)
	defer destination.db.Close()
	otherProject, _, err := destination.store.CreateProject(ctx, store.CreateProjectParams{Name: "Existing Project", CreatedBy: "owner", ChangeNote: "collision fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := destination.store.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ID: collidingAliasID,
		ProjectID: otherProject.ID,
		LogicalName: "EXISTING-TARGET",
		LogicalType: "TEST",
		TargetRef: "EXISTING-TARGET",
		ProjectConfig: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := destination.service.PlanImport(ctx, exported.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.MaterializationReady {
		t.Fatalf("preflight should leave deep identity collision to transaction: %+v", plan.Checks)
	}

	if _, err := destination.service.Materialize(ctx, exported.Path, MaterializeOptions{ImportedBy: "owner"}); err == nil {
		t.Fatal("materialization unexpectedly succeeded across alias identity collision")
	}
	if _, err := destination.store.GetProject(ctx, source.projectID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("failed materialization left portable Project behind: %v", err)
	}
	if _, err := destination.store.GetVaultObject(ctx, source.contentHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("failed materialization left newly staged Vault object metadata behind: %v", err)
	}
	if _, err := destination.store.GetProject(ctx, otherProject.ID); err != nil {
		t.Fatalf("rollback damaged pre-existing Project: %v", err)
	}
	alias, err := destination.store.ListAliases(ctx, otherProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alias) != 1 || alias[0].ID != collidingAliasID {
		t.Fatalf("pre-existing alias changed during rollback: %+v", alias)
	}
}
