package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueDraftHistoryUndoRedoPreservesMixedLinkedCueIdentity(t *testing.T) {
	ctx := store.WithCueDraftHistoryActor(context.Background(), "owner")
	s, handle := newStore(t)
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, created_at_us, updated_at_us)
		VALUES ('owner', 'owner', 'unused', 'OWNER', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Cue draft history"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2A", Name: "Child", OrderIndex: 1,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{"operator_color":"#13579b"}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "DMX-1", CapabilityKey: "lighting.set",
		Parameters:    json.RawMessage(`{"channel":2,"value":80,"vendor_extension":{"keep":true}}`),
		TimeoutPolicy: json.RawMessage(`{"start_delay_ms":125}`), ErrorPolicy: json.RawMessage(`{"mode":"FAIL_SOFT"}`), Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Mixed parent", OrderIndex: 0,
		Enabled: true, NotesSummary: "original notes",
		ExecutionPolicy: json.RawMessage(`{"linked_cue_ids":["` + child.ID + `"],"linked_cue_mode":"TOGETHER"}`),
	}, []domain.Action{
		{OrderIndex: 0, ExecutionMode: "PARALLEL", TargetRef: "MAC", CapabilityKey: "osc.send", Parameters: json.RawMessage(`{"address":"/cue/fire"}`), Enabled: true},
		{OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetRef: "ABLETON", CapabilityKey: "midi.send", Parameters: json.RawMessage(`{"note":60}`), TimeoutPolicy: json.RawMessage(`{"start_delay_ms":250}`), Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	parentActionIDs := []string{parent.Actions[0].ID, parent.Actions[1].ID}
	childActionID := child.Actions[0].ID

	status, err := s.CueDraftHistory(ctx, project.ID)
	if err != nil || !status.CanUndo || status.CanRedo {
		t.Fatalf("history after create=%+v err=%v", status, err)
	}
	if _, err := s.UndoCueDraft(ctx, project.ID); err != nil {
		t.Fatalf("undo parent create: %v", err)
	}
	if _, err := s.GetCue(ctx, parent.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("undo create left parent Cue: err=%v", err)
	}
	if _, err := s.GetCue(ctx, child.ID); err != nil {
		t.Fatalf("undo create removed linked child: %v", err)
	}
	if _, err := s.RedoCueDraft(ctx, project.ID); err != nil {
		t.Fatalf("redo parent create: %v", err)
	}
	restoredParent, err := s.GetCue(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoredParent.ID != parent.ID || len(restoredParent.Actions) != 2 ||
		restoredParent.Actions[0].ID != parentActionIDs[0] || restoredParent.Actions[1].ID != parentActionIDs[1] ||
		string(restoredParent.ExecutionPolicy) != string(parent.ExecutionPolicy) || restoredParent.NotesSummary != parent.NotesSummary {
		t.Fatalf("redo create changed identity or Mixed Cue content: %+v", restoredParent)
	}
	if childAgain, err := s.GetCue(ctx, child.ID); err != nil || len(childAgain.Actions) != 1 || childAgain.Actions[0].ID != childActionID {
		t.Fatalf("linked child/action changed: cue=%+v err=%v", childAgain, err)
	}

	updated := restoredParent
	updated.Name = "Mixed parent edited"
	updated.NotesSummary = "edited notes"
	if _, err := s.ReplaceDraftCue(ctx, updated, restoredParent.Actions); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoCueDraft(ctx, project.ID); err != nil {
		t.Fatalf("undo edit: %v", err)
	}
	undone, err := s.GetCue(ctx, parent.ID)
	if err != nil || undone.Name != "Mixed parent" || undone.NotesSummary != "original notes" {
		t.Fatalf("undo edit=%+v err=%v", undone, err)
	}
	if _, err := s.RedoCueDraft(ctx, project.ID); err != nil {
		t.Fatalf("redo edit: %v", err)
	}
	redone, err := s.GetCue(ctx, parent.ID)
	if err != nil || redone.Name != "Mixed parent edited" || redone.Actions[0].ID != parentActionIDs[0] {
		t.Fatalf("redo edit=%+v err=%v", redone, err)
	}

	if _, err := s.ReorderDraftCues(ctx, revision.ID, []string{parent.ID, child.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoCueDraft(ctx, project.ID); err != nil {
		t.Fatalf("undo reorder: %v", err)
	}
	ordered, err := s.ListCues(ctx, revision.ID)
	if err != nil || len(ordered) != 2 || ordered[0].ID != parent.ID || ordered[1].ID != child.ID {
		t.Fatalf("undo reorder=%+v err=%v", ordered, err)
	}

	checkpoint, err := s.CreateCueEditorCheckpoint(ctx, project.ID, "owner", parent.ID, "Before delete",
		json.RawMessage(`{"version":1,"cue_id":"`+parent.ID+`","draft":{"name":"Mixed parent"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDraftCue(ctx, revision.ID, parent.ID); err != nil {
		t.Fatal(err)
	}
	deletedCheckpoints, err := s.ListCueEditorCheckpoints(ctx, project.ID, "owner", parent.ID)
	if err != nil || len(deletedCheckpoints) != 0 {
		t.Fatalf("deleted Cue still exposes checkpoints: checkpoints=%+v err=%v", deletedCheckpoints, err)
	}
	if _, err := s.UndoCueDraft(ctx, project.ID); err != nil {
		t.Fatalf("undo delete: %v", err)
	}
	recovered, err := s.GetCue(ctx, parent.ID)
	if err != nil || recovered.ID != parent.ID || len(recovered.Actions) != 2 || recovered.Actions[0].ID != parentActionIDs[0] {
		t.Fatalf("undo delete did not restore stable linked Mixed Cue: cue=%+v err=%v", recovered, err)
	}
	restoredCheckpoints, err := s.ListCueEditorCheckpoints(ctx, project.ID, "owner", parent.ID)
	if err != nil || len(restoredCheckpoints) != 1 || restoredCheckpoints[0].ID != checkpoint.ID ||
		!bytes.Equal(restoredCheckpoints[0].Document, checkpoint.Document) {
		t.Fatalf("Undo did not restore the named checkpoint: checkpoints=%+v err=%v", restoredCheckpoints, err)
	}
	status, err = s.CueDraftHistory(ctx, project.ID)
	if err != nil || !status.CanRedo {
		t.Fatalf("history after undo delete=%+v err=%v", status, err)
	}
	if _, err := s.RedoCueDraft(ctx, project.ID); err != nil {
		t.Fatalf("redo delete: %v", err)
	}
	if _, err := s.GetCue(ctx, parent.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("redo delete left parent Cue: err=%v", err)
	}
	deletedCheckpoints, err = s.ListCueEditorCheckpoints(ctx, project.ID, "owner", parent.ID)
	if err != nil || len(deletedCheckpoints) != 0 {
		t.Fatalf("Redo did not remove the Cue's checkpoint with the Cue: checkpoints=%+v err=%v", deletedCheckpoints, err)
	}
}

func TestCueDraftHistoryRejectsUndoAcrossAnotherEditorsChange(t *testing.T) {
	base := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(base, store.CreateProjectParams{Name: "Cue history concurrency"})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithCueDraftHistoryActor(base, "owner")
	other := store.WithCueDraftHistoryActor(base, "other-editor")
	cue, err := s.CreateCueWithActions(owner, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 0,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ownerEdit := cue
	ownerEdit.NotesSummary = "owner edit"
	if _, err := s.ReplaceDraftCue(owner, ownerEdit, nil); err != nil {
		t.Fatal(err)
	}
	otherEdit := ownerEdit
	otherEdit.NotesSummary = "other editor edit"
	if _, err := s.ReplaceDraftCue(other, otherEdit, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoCueDraft(owner, project.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("owner undo across another editor change err=%v, want conflict", err)
	}
	if _, err := s.UndoCueDraft(other, project.ID); err != nil {
		t.Fatalf("other editor undo: %v", err)
	}
	if _, err := s.UndoCueDraft(owner, project.ID); err != nil {
		t.Fatalf("owner undo after intervening edit was undone: %v", err)
	}
	restored, err := s.GetCue(base, cue.ID)
	if err != nil || restored.NotesSummary != "" {
		t.Fatalf("final undo=%+v err=%v", restored, err)
	}
}

func TestCueDraftHistoryPersistsAcrossRestartAndIsBounded(t *testing.T) {
	ctx := store.WithCueDraftHistoryActor(context.Background(), "owner")
	root := t.TempDir()
	h, err := db.Open(context.Background(), db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Persistent Cue history"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 0,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = db.Open(context.Background(), db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s = store.New(h.DB, clock.Real{})
	status, err := s.CueDraftHistory(ctx, project.ID)
	if err != nil || !status.CanUndo {
		t.Fatalf("history did not survive Hub database reopen: %+v err=%v", status, err)
	}
	for i := 1; i <= 55; i++ {
		current, err := s.GetCue(ctx, cue.ID)
		if err != nil {
			t.Fatal(err)
		}
		current.NotesSummary = strings.Repeat("take-", 1) + string(rune('A'+i%26)) + "-" + strings.Repeat("x", i)
		if _, err := s.ReplaceDraftCue(ctx, current, nil); err != nil {
			t.Fatalf("record edit %d: %v", i, err)
		}
	}
	for i := 0; i < 50; i++ {
		if _, err := s.UndoCueDraft(ctx, project.ID); err != nil {
			t.Fatalf("Undo %d within history limit: %v", i+1, err)
		}
	}
	if _, err := s.UndoCueDraft(ctx, project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Undo beyond retained history err=%v, want not found", err)
	}
	current, err := s.GetCue(ctx, cue.ID)
	if err != nil {
		t.Fatal(err)
	}
	current.NotesSummary = "new branch after undo"
	if _, err := s.ReplaceDraftCue(ctx, current, nil); err != nil {
		t.Fatal(err)
	}
	status, err = s.CueDraftHistory(ctx, project.ID)
	if err != nil || !status.CanUndo || status.CanRedo {
		t.Fatalf("new edit did not discard redo branch: status=%+v err=%v", status, err)
	}
}

func TestCueDraftHistoryNeverChangesValidatedSourceOrPublishedSnapshot(t *testing.T) {
	ctx := store.WithCueDraftHistoryActor(context.Background(), "owner")
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Immutable Cue history"})
	if err != nil {
		t.Fatal(err)
	}
	sourceCue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Published opening", OrderIndex: 0,
		Enabled: true, NotesSummary: "published note", ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "OSC-1", CapabilityKey: "osc.send",
		Parameters: json.RawMessage(`{"address":"/show/open"}`), Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.CreateRuntimeSnapshot(ctx, revision.ID, "owner", strings.Repeat("e", 64), json.RawMessage(`{"published":"immutable"}`))
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "edit after publish")
	if err != nil {
		t.Fatal(err)
	}
	draftCues, err := s.ListCues(ctx, draft.ID)
	if err != nil || len(draftCues) != 1 {
		t.Fatalf("Draft Cues=%+v err=%v", draftCues, err)
	}
	edit := draftCues[0]
	edit.Name = "Draft-only name"
	edit.NotesSummary = "draft-only note"
	if _, err := s.ReplaceDraftCue(ctx, edit, edit.Actions); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoCueDraft(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
	restoredDraft, err := s.GetCue(ctx, edit.ID)
	if err != nil || restoredDraft.Name != "Published opening" || restoredDraft.NotesSummary != "published note" {
		t.Fatalf("Draft Undo did not restore its source state: cue=%+v err=%v", restoredDraft, err)
	}
	unchangedSource, err := s.GetCue(ctx, sourceCue.ID)
	if err != nil || unchangedSource.Name != sourceCue.Name || unchangedSource.Actions[0].ID != sourceCue.Actions[0].ID {
		t.Fatalf("validated source Cue changed: cue=%+v err=%v", unchangedSource, err)
	}
	unchangedSnapshot, err := s.GetRuntimeSnapshot(ctx, snapshot.ID)
	if err != nil || unchangedSnapshot.ID != snapshot.ID || unchangedSnapshot.ContentHash != snapshot.ContentHash ||
		string(unchangedSnapshot.Manifest) != string(snapshot.Manifest) || unchangedSnapshot.RevisionID != revision.ID {
		t.Fatalf("Draft Undo changed Published Snapshot: snapshot=%+v err=%v", unchangedSnapshot, err)
	}
}

func TestCueDraftHistoryUndoRespectsActiveShowConfigurationLock(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, publishedSnapshot, _ := createSessionFoundationFixture(t, s)
	draft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "history show-lock regression")
	if err != nil {
		t.Fatal(err)
	}
	actorContext := store.WithCueDraftHistoryActor(ctx, "owner")
	cue, err := s.CreateCueWithActions(actorContext, domain.Cue{
		RevisionID: draft.ID, DisplayLabel: "3", Name: "Draft-only", OrderIndex: 2,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSession(ctx, publishedSnapshot.ID, domain.SessionShow, "history lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoCueDraft(actorContext, project.ID); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("Undo during active SHOW err=%v, want configuration lock", err)
	}
	if stored, err := s.GetCue(ctx, cue.ID); err != nil || stored.ID != cue.ID {
		t.Fatalf("locked Undo changed Draft Cue: cue=%+v err=%v", stored, err)
	}
}
