package store_test

import (
    "context"
    "encoding/json"
    "testing"

    "github.com/ali96adil/StageCore/internal/domain"
    "github.com/ali96adil/StageCore/internal/store"
)

func TestDuplicateProjectAuthoringIndependentCueNotes(t *testing.T) {
    ctx := context.Background()
    s, _ := newStore(t)
    original, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name:"Original", Description:"Show description", CreatedBy:"operator"})
    if err != nil { t.Fatal(err) }
    cue, err := s.CreateCueWithActions(ctx, domain.Cue{
        RevisionID: revision.ID, DisplayLabel:"1", Name:"Opening", OrderIndex:1,
        Enabled:true, NotesSummary:"Lights ready\nGO on gesture", ExecutionPolicy:json.RawMessage(`{"start_delay_ms":2000}`),
    }, nil)
    if err != nil { t.Fatal(err) }
    duplicate, draft, err := s.DuplicateProjectAuthoring(ctx, original.ID, "Opening Copy", "operator")
    if err != nil { t.Fatal(err) }
    if duplicate.ID == original.ID || duplicate.Name != "Opening Copy" || duplicate.Description != original.Description {
        t.Fatalf("independent project metadata failed: %+v", duplicate)
    }
    if draft.ID == revision.ID || draft.Status != domain.RevisionDraft { t.Fatalf("bad copy revision: %+v", draft) }
    copied, err := s.ListCues(ctx, draft.ID)
    if err != nil { t.Fatal(err) }
    if len(copied) != 1 || copied[0].ID == cue.ID || copied[0].NotesSummary != cue.NotesSummary ||
       string(copied[0].ExecutionPolicy) != string(cue.ExecutionPolicy) {
       t.Fatalf("duplicate Cue not independent: %+v", copied)
    }
    freshSource, err := s.GetProject(ctx, original.ID)
    if err != nil || freshSource.CurrentRevisionID != revision.ID { t.Fatalf("original changed: %+v (%v)", freshSource, err) }
}

func TestDuplicateProjectAuthoringRejectsEmptyName(t *testing.T) {
    ctx := context.Background()
    s, _ := newStore(t)
    original, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name:"Source"})
    if err != nil { t.Fatal(err) }
    if _, _, err := s.DuplicateProjectAuthoring(ctx, original.ID, "  ", "operator"); err == nil {
        t.Fatal("expected invalid duplicate name to be rejected")
    }
}
