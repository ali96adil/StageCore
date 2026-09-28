package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

func executionEnvironmentSnapshotWithRebuild(manifest store.ExecutionEnvironmentManifest) executionenv.Snapshot {
	snapshot := executionEnvironmentSnapshotFixture(manifest)
	snapshot.RebuildPlanVersion = executionenv.SnapshotRebuildPlanVersion
	snapshot.ReconstructionFingerprint = strings.Repeat("d", 64)
	snapshot.RebuildPlan = []executionenv.SnapshotRebuildStep{
		{Step: 1, Action: "Open VDMX", Status: "OBSERVED", ProvenanceClass: executionenv.SnapshotProvenanceObserved},
		{Step: 2, Action: "Recreate unsupported internals manually", Status: "MANUAL", ProvenanceClass: executionenv.SnapshotProvenanceUnsupported},
	}
	return snapshot
}

func TestExecutionEnvironmentRebuildPlanPersistenceAndSourceLineage(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 editable plan", CreatedBy: "test"})
	if err != nil { t.Fatal(err) }
	manifest, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionEnvironmentFixture("editable-plan"), "test.operator")
	if err != nil { t.Fatal(err) }
	snapshot, err := s.CreateExecutionEnvironmentSnapshot(ctx, manifest.ID, executionEnvironmentSnapshotWithRebuild(manifest), "test.operator")
	if err != nil { t.Fatal(err) }
	plan, err := executionenv.SeedAssistedRebuildPlan(snapshot.Snapshot)
	if err != nil { t.Fatal(err) }
	plan.Steps = append(plan.Steps, executionenv.AssistedRebuildStep{
		Step: 3, Action: "Verify projector output manually", Status: "MANUAL",
		ProvenanceClass: executionenv.SnapshotProvenanceUserDeclared,
	})
	created, err := s.UpsertExecutionEnvironmentRebuildPlan(ctx, manifest.ID, snapshot.ID, plan, "test.operator")
	if err != nil { t.Fatal(err) }
	if created.EnvironmentManifestID != manifest.ID ||
		created.SourceSnapshotID != snapshot.ID ||
		created.RevisionID != revision.ID ||
		created.CreatedBy != "test.operator" ||
		created.UpdatedBy != "test.operator" ||
		created.ContentSHA256 == "" ||
		len(created.Plan.Steps) != 3 {
		t.Fatalf("created=%+v", created)
	}

	edited := created.Plan
	edited.Steps[0].Action = "Open VDMX and choose the show workspace"
	if _, err := s.UpsertExecutionEnvironmentRebuildPlan(ctx, manifest.ID, snapshot.ID, edited, "editor"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("edited observed provenance err=%v", err)
	}
	edited.Steps[0].ProvenanceClass = executionenv.SnapshotProvenanceUserDeclared
	updated, err := s.UpsertExecutionEnvironmentRebuildPlan(ctx, manifest.ID, snapshot.ID, edited, "editor")
	if err != nil { t.Fatal(err) }
	if updated.ID != created.ID ||
		updated.CreatedBy != created.CreatedBy ||
		updated.UpdatedBy != "editor" ||
		updated.Plan.Steps[0].ProvenanceClass != executionenv.SnapshotProvenanceUserDeclared {
		t.Fatalf("updated=%+v created=%+v", updated, created)
	}

	if _, err := handle.DB.ExecContext(ctx,
		`UPDATE execution_environment_rebuild_plans SET content_sha256 = ? WHERE rebuild_plan_id = ?`,
		strings.Repeat("f", 64), updated.ID,
	); err != nil { t.Fatal(err) }
	if _, err := s.GetExecutionEnvironmentRebuildPlan(ctx, manifest.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("tampered rebuild plan err=%v", err)
	}
}

func TestExecutionEnvironmentRebuildPlanRejectsCrossEnvironmentSource(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 plan scope", CreatedBy: "test"})
	if err != nil { t.Fatal(err) }
	first, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionEnvironmentFixture("plan-a"), "test")
	if err != nil { t.Fatal(err) }
	second, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionEnvironmentFixture("plan-b"), "test")
	if err != nil { t.Fatal(err) }
	source, err := s.CreateExecutionEnvironmentSnapshot(ctx, first.ID, executionEnvironmentSnapshotWithRebuild(first), "test")
	if err != nil { t.Fatal(err) }
	plan, err := executionenv.SeedAssistedRebuildPlan(source.Snapshot)
	if err != nil { t.Fatal(err) }
	if _, err := s.UpsertExecutionEnvironmentRebuildPlan(ctx, second.ID, source.ID, plan, "test"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-environment source err=%v", err)
	}
}
