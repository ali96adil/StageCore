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

func TestDuplicateProjectAuthoringRemapsLinkedCuesAndAliases(t *testing.T) {
    ctx := context.Background()
    s, _ := newStore(t)
    original, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name:"Linked show", CreatedBy:"operator"})
    if err != nil { t.Fatal(err) }
    child, err := s.CreateCueWithActions(ctx, domain.Cue{
        RevisionID:revision.ID, DisplayLabel:"2", Name:"Audio", OrderIndex:2, Enabled:true,
        NotesSummary:"Audio note",
    }, nil)
    if err != nil { t.Fatal(err) }
    parent, err := s.CreateCueWithActions(ctx, domain.Cue{
        RevisionID:revision.ID, DisplayLabel:"1", Name:"Main", OrderIndex:1, Enabled:true,
        ExecutionPolicy:json.RawMessage(`{"linked_cue_ids":["`+child.ID+`"]}`),
    }, nil)
    if err != nil { t.Fatal(err) }
    _, err = s.CreateAlias(ctx, domain.ProjectDeviceAlias{
        ProjectID:original.ID, LogicalName:"front_cold_a", LogicalType:"LIGHTING",
        TargetRef:"fixture-01", ProjectConfig:json.RawMessage(`{"universe":1,"channel":1}`),
    })
    if err != nil { t.Fatal(err) }
    copied, draft, err := s.DuplicateProjectAuthoring(ctx, original.ID, "Linked show Copy", "operator")
    if err != nil { t.Fatal(err) }
    cues, err := s.ListCues(ctx, draft.ID)
    if err != nil { t.Fatal(err) }
    if len(cues) != 2 { t.Fatalf("want both cues, got %d", len(cues)) }
    var copiedParent, copiedChild domain.Cue
    for _, cue := range cues {
        if cue.Name == parent.Name { copiedParent = cue }
        if cue.Name == child.Name { copiedChild = cue }
    }
    if copiedParent.ID == "" || copiedChild.ID == "" || copiedParent.ID == parent.ID || copiedChild.ID == child.ID {
        t.Fatalf("destination must have fresh Cue IDs: %+v", cues)
    }
    var policy struct { LinkedCueIDs []string `json:"linked_cue_ids"` }
    if err := json.Unmarshal(copiedParent.ExecutionPolicy, &policy); err != nil { t.Fatal(err) }
    if len(policy.LinkedCueIDs) != 1 || policy.LinkedCueIDs[0] != copiedChild.ID {
        t.Fatalf("linked Cue ID not remapped: %+v", policy)
    }
    oldAliases, err := s.ListAliases(ctx, original.ID)
    if err != nil { t.Fatal(err) }
    newAliases, err := s.ListAliases(ctx, copied.ID)
    if err != nil { t.Fatal(err) }
    if len(oldAliases) != 1 || len(newAliases) != 1 || oldAliases[0].ID == newAliases[0].ID ||
       newAliases[0].LogicalName != oldAliases[0].LogicalName ||
       string(newAliases[0].ProjectConfig) != string(oldAliases[0].ProjectConfig) {
        t.Fatalf("alias cloning incorrect: old=%+v new=%+v", oldAliases, newAliases)
    }
    if session, err := s.ActiveSessionForProject(ctx, copied.ID); err != nil || session != nil {
        t.Fatalf("duplicate inherited an active session: %v, %v", session, err)
    }
}
