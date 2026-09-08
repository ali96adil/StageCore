package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

func TestDiscardProjectDraftBlockedDuringShow(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "SHOW locked discard", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CreateRuntimeSnapshot(ctx, revision.ID, "owner", strings.Repeat("c", 64), json.RawMessage(`{"show":"locked"}`))
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "accidental edit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSession(ctx, snapshot.ID, domain.SessionShow, "Live SHOW"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.DiscardProjectDraft(ctx, project.ID, "owner", "attempt during SHOW"); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("discard during SHOW err=%v want ErrShowConfigurationLocked", err)
	}
	loaded, err := s.GetRevision(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.RevisionDraft {
		t.Fatalf("SHOW-blocked discard changed Draft status=%s", loaded.Status)
	}
	loadedProject, err := s.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedProject.CurrentRevisionID != draft.ID {
		t.Fatalf("SHOW-blocked discard changed current revision=%s want=%s", loadedProject.CurrentRevisionID, draft.ID)
	}
}

func TestDiscardProjectDraftPreservesPublishedRuntimeSnapshot(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Snapshot safe discard", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	manifest := json.RawMessage(`{"immutable":"phase4","version":1}`)
	snapshot, err := s.CreateRuntimeSnapshot(ctx, revision.ID, "owner", strings.Repeat("d", 64), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "accidental edit"); err != nil {
		t.Fatal(err)
	}
	if _, discarded, err := s.DiscardProjectDraft(ctx, project.ID, "owner", "restore validated"); err != nil || !discarded {
		t.Fatalf("discarded=%v err=%v", discarded, err)
	}
	after, err := s.GetRuntimeSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ID != snapshot.ID || after.ProjectID != snapshot.ProjectID || after.RevisionID != snapshot.RevisionID ||
		after.SnapshotVersion != snapshot.SnapshotVersion || after.CreatedBy != snapshot.CreatedBy ||
		after.ContentHash != snapshot.ContentHash || after.Status != snapshot.Status || string(after.Manifest) != string(snapshot.Manifest) ||
		!after.CreatedAt.Equal(snapshot.CreatedAt) {
		t.Fatalf("Runtime Snapshot changed across Draft discard\nbefore=%+v\nafter=%+v", snapshot, after)
	}
}
