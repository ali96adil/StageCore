package showcapsule

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestSelfContainedMaterializePreservesRuntimeIdentity(t *testing.T) {
	ctx := context.Background()
	source := newCapsuleFixture(t)
	defer source.db.Close()

	exported, err := source.service.Export(ctx, source.projectID, t.TempDir(), BuildOptions{
		RuntimeSnapshotID: source.snapshotID,
		Mode: ExportSelfContained,
	})
	if err != nil {
		t.Fatal(err)
	}

	destination := newEmptyCapsuleService(t)
	defer destination.db.Close()
	plan, err := destination.service.PlanImport(ctx, exported.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.MaterializationReady {
		t.Fatalf("materialization unexpectedly blocked: %+v", plan.Checks)
	}
	if plan.ReplacementHostReady {
		t.Fatal("replacement host unexpectedly claims full readiness while device-local presentation state is excluded")
	}
	if len(plan.IncludedObjects) != 1 || plan.IncludedObjects[0].ContentSHA256 != source.contentHash {
		t.Fatalf("included objects=%+v", plan.IncludedObjects)
	}

	result, err := destination.service.Materialize(ctx, exported.Path, MaterializeOptions{ImportedBy: "capsule-test"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectID != source.projectID || result.RevisionID != source.revisionID || result.RuntimeSnapshotID != source.snapshotID {
		t.Fatalf("materialized identity=%+v", result)
	}
	if len(result.ImportedObjects) != 1 || result.ImportedObjects[0] != source.contentHash {
		t.Fatalf("imported objects=%v", result.ImportedObjects)
	}

	project, err := destination.store.GetProject(ctx, source.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if project.CurrentRevisionID != source.revisionID || project.Name != "Capsule Test Show" {
		t.Fatalf("restored project=%+v", project)
	}
	revision, err := destination.store.GetRevision(ctx, source.revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if revision.Status != domain.RevisionValidated || revision.RevisionNumber != 1 {
		t.Fatalf("restored revision=%+v", revision)
	}
	sourceSnapshot, err := source.store.GetRuntimeSnapshot(ctx, source.snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	restoredSnapshot, err := destination.store.GetRuntimeSnapshot(ctx, source.snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredSnapshot.ContentHash != sourceSnapshot.ContentHash || string(restoredSnapshot.Manifest) != string(sourceSnapshot.Manifest) {
		t.Fatalf("runtime snapshot changed across capsule restore\nsource=%+v\nrestored=%+v", sourceSnapshot, restoredSnapshot)
	}
	cues, err := destination.store.ListCues(ctx, source.revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].Name != "Opening" || cues[0].NotesSummary != "Stand by projection" || len(cues[0].Actions) != 1 {
		t.Fatalf("restored cues=%+v", cues)
	}
	roles, err := destination.store.ListMachineRoles(ctx, source.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 || roles[0].RoleKey != "video-main" {
		t.Fatalf("restored roles=%+v", roles)
	}
	media, err := destination.store.ListProjectMediaRequirements(ctx, source.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(media) != 1 || media[0].ContentHash != source.contentHash || media[0].MediaAssetID != source.mediaAssetID {
		t.Fatalf("restored media=%+v", media)
	}
	if file, _, err := destination.vault.OpenObject(ctx, source.contentHash); err != nil {
		t.Fatalf("restored Vault object: %v", err)
	} else {
		_ = file.Close()
	}

	beforeCues := len(cues)
	if _, err := destination.service.Materialize(ctx, exported.Path, MaterializeOptions{ImportedBy: "capsule-test"}); err == nil || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second identity-preserving import error=%v", err)
	}
	afterCues, err := destination.store.ListCues(ctx, source.revisionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterCues) != beforeCues {
		t.Fatalf("collision import mutated project: before=%d after=%d", beforeCues, len(afterCues))
	}
}

type emptyCapsuleFixture struct {
	service *Service
	db      *db.Handle
	store   *store.Store
	vault   *vault.Vault
}

func newEmptyCapsuleService(t *testing.T) emptyCapsuleFixture {
	t.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(handle.DB, clock.Fixed{Time: capsuleTestTime})
	stageVault, err := vault.Open(t.TempDir(), stageStore)
	if err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	service, err := New(stageStore, stageVault, clock.Fixed{Time: capsuleTestTime})
	if err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	return emptyCapsuleFixture{service: service, db: handle, store: stageStore, vault: stageVault}
}
