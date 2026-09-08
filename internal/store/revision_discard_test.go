package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestDiscardProjectDraftRestoresValidatedParentAndAllowsNewDraft(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, first, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Discard Show", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, first.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	draft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "routing edit")
	if err != nil {
		t.Fatal(err)
	}
	if draft.ParentRevisionID == nil || *draft.ParentRevisionID != first.ID {
		t.Fatalf("draft parent=%v want=%s", draft.ParentRevisionID, first.ID)
	}

	restored, discarded, err := s.DiscardProjectDraft(ctx, project.ID, "owner", "operator cancelled edit")
	if err != nil {
		t.Fatal(err)
	}
	if !discarded || restored.ID != first.ID || restored.Status != domain.RevisionValidated {
		t.Fatalf("restored=%+v discarded=%v", restored, discarded)
	}
	loadedProject, err := s.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedProject.CurrentRevisionID != first.ID {
		t.Fatalf("current revision=%s want=%s", loadedProject.CurrentRevisionID, first.ID)
	}
	abandoned, err := s.GetRevision(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if abandoned.Status != domain.RevisionSuperseded {
		t.Fatalf("discarded revision status=%s want SUPERSEDED", abandoned.Status)
	}

	secondDraft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "new edit")
	if err != nil {
		t.Fatal(err)
	}
	if secondDraft.ID == draft.ID || secondDraft.ParentRevisionID == nil || *secondDraft.ParentRevisionID != first.ID {
		t.Fatalf("new draft=%+v", secondDraft)
	}
}

func TestDiscardProjectDraftIsIdempotentWhenCurrentRevisionValidated(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Validated Show"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	restored, discarded, err := s.DiscardProjectDraft(ctx, project.ID, "owner", "no draft")
	if err != nil {
		t.Fatal(err)
	}
	if discarded || restored.ID != revision.ID {
		t.Fatalf("restored=%+v discarded=%v", restored, discarded)
	}
}

func TestDiscardProjectDraftRejectsInitialParentlessDraft(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "New Show"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DiscardProjectDraft(ctx, project.ID, "owner", "cancel"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("discard initial Draft err=%v want conflict", err)
	}
}
