package store_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestEnsureProjectDraftClonesExecutionEnvironmentSnapshots(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 fork", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionEnvironmentFixture("fork-environment"), "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CreateExecutionEnvironmentSnapshot(ctx, environment.ID, executionEnvironmentSnapshotFixture(environment), "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	draft, err := s.EnsureProjectDraft(ctx, project.ID, "fork.operator", "continue editing")
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID == revision.ID || draft.ParentRevisionID == nil || *draft.ParentRevisionID != revision.ID || draft.Status != domain.RevisionDraft {
		t.Fatalf("draft=%+v", draft)
	}
	environments, err := s.ListExecutionEnvironmentManifests(ctx, draft.ID)
	if err != nil || len(environments) != 1 {
		t.Fatalf("environments=%+v err=%v", environments, err)
	}
	if environments[0].ID == environment.ID || environments[0].ContentSHA256 != environment.ContentSHA256 {
		t.Fatalf("cloned environment=%+v source=%+v", environments[0], environment)
	}
	snapshots, err := s.ListExecutionEnvironmentSnapshots(ctx, environments[0].ID)
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("snapshots=%+v err=%v", snapshots, err)
	}
	if snapshots[0].ID == snapshot.ID || snapshots[0].ContentSHA256 != snapshot.ContentSHA256 || snapshots[0].RevisionID != draft.ID {
		t.Fatalf("cloned snapshot=%+v source=%+v", snapshots[0], snapshot)
	}
	if snapshots[0].CreatedBy != "fork.operator" {
		t.Fatalf("cloned snapshot actor=%q", snapshots[0].CreatedBy)
	}
}
