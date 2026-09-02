package store_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestExecutionEnvironmentManifestFollowsRevisionForkWithoutMutatingSource(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 Fork", CreatedBy: "source.actor"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionEnvironmentFixture("video-main"), "source.actor")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	fork, err := s.EnsureProjectDraft(ctx, project.ID, "fork.actor", "continue F-025")
	if err != nil {
		t.Fatal(err)
	}
	if fork.ParentRevisionID == nil || *fork.ParentRevisionID != revision.ID || fork.Status != domain.RevisionDraft {
		t.Fatalf("fork=%+v", fork)
	}
	sourceItems, err := s.ListExecutionEnvironmentManifests(ctx, revision.ID)
	if err != nil || len(sourceItems) != 1 || sourceItems[0].ID != source.ID {
		t.Fatalf("source items=%+v err=%v", sourceItems, err)
	}
	forkItems, err := s.ListExecutionEnvironmentManifests(ctx, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forkItems) != 1 {
		t.Fatalf("fork items=%+v", forkItems)
	}
	cloned := forkItems[0]
	if cloned.ID == source.ID || cloned.RevisionID != fork.ID || cloned.Manifest.EnvironmentKey != source.Manifest.EnvironmentKey || cloned.ContentSHA256 != source.ContentSHA256 {
		t.Fatalf("cloned=%+v source=%+v", cloned, source)
	}
	if cloned.CreatedBy != "fork.actor" {
		t.Fatalf("cloned created_by=%q", cloned.CreatedBy)
	}
}
