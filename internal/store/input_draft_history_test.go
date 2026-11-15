package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestInputDraftHistoryRecordsCreateUpdateDeleteAtomically(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Input Draft history"})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithInputDraftHistoryActor(ctx, "owner")
	input, err := s.CreateInput(owner, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message",
		ValueSchema: json.RawMessage(`{"type":"object"}`), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var operation, inputID string
	if err := handle.DB.QueryRowContext(ctx, `SELECT operation, input_id FROM input_draft_history_events WHERE revision_id = ? AND actor_id = ? AND sequence = 1`, revision.ID, "owner").Scan(&operation, &inputID); err != nil {
		t.Fatalf("read atomic Input create history: %v", err)
	}
	if operation != "CREATE" || inputID != input.ID {
		t.Fatalf("Input create history=(%q,%q), want CREATE and %q", operation, inputID, input.ID)
	}
	created := input
	status, err := s.InputDraftHistory(owner, project.ID)
	if err != nil || !status.CanUndo || status.CanRedo || status.UndoOperation != "CREATE" {
		t.Fatalf("history after create=%+v err=%v", status, err)
	}
	if other, err := s.InputDraftHistory(store.WithInputDraftHistoryActor(ctx, "other-editor"), project.ID); err != nil || other.CanUndo || other.CanRedo {
		t.Fatalf("Input history leaked across editors: %+v err=%v", other, err)
	}

	input.Name = "GO revised"
	input.SourceRef = "osc:/revised"
	input.ValueSchema = json.RawMessage(`{"type":"number"}`)
	input.Enabled = false
	updated, err := s.UpdateInput(owner, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteInput(owner, revision.ID, input.ID); err != nil {
		t.Fatal(err)
	}
	if inputs, err := s.ListInputs(ctx, revision.ID); err != nil || len(inputs) != 0 {
		t.Fatalf("delete left inputs=%+v err=%v", inputs, err)
	}
	if _, err := s.UndoInputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Input delete: %v", err)
	}
	inputs, err := s.ListInputs(ctx, revision.ID)
	if err != nil || len(inputs) != 1 || !reflect.DeepEqual(inputs[0], updated) {
		t.Fatalf("undo delete did not restore Input: inputs=%+v want=%+v err=%v", inputs, updated, err)
	}
	if _, err := s.RedoInputDraft(owner, project.ID); err != nil {
		t.Fatalf("redo Input delete: %v", err)
	}
	if _, err := s.UndoInputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Input delete again: %v", err)
	}
	if _, err := s.UndoInputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Input edit: %v", err)
	}
	inputs, err = s.ListInputs(ctx, revision.ID)
	if err != nil || len(inputs) != 1 || !reflect.DeepEqual(inputs[0], created) {
		t.Fatalf("undo edit did not restore exact Input: inputs=%+v want=%+v err=%v", inputs, created, err)
	}
	if _, err := s.RedoInputDraft(owner, project.ID); err != nil {
		t.Fatalf("redo Input edit: %v", err)
	}
	inputs, err = s.ListInputs(ctx, revision.ID)
	if err != nil || len(inputs) != 1 || !reflect.DeepEqual(inputs[0], updated) {
		t.Fatalf("redo edit did not restore exact Input: inputs=%+v want=%+v err=%v", inputs, updated, err)
	}
	var events int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM input_draft_history_events WHERE revision_id = ? AND actor_id = ?`, revision.ID, "owner").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 3 {
		t.Fatalf("Input Draft history events=%d, want create/edit/delete retained after Undo/Redo", events)
	}
	if _, err := s.GetProject(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
}

func TestInputDraftHistoryRejectsOutOfBandEditsAndReferencedRestore(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Input Draft history conflict"})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithInputDraftHistoryActor(ctx, "owner")
	input, err := s.CreateInput(owner, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	route, err := s.CreateRouteWithActions(ctx, domain.Route{
		RevisionID: revision.ID, Name: "GO Route", InputID: input.ID, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoInputDraft(owner, project.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("undo Input create while a Route references it=%v, want conflict", err)
	}
	if err := s.DeleteRoute(ctx, revision.ID, route.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE input_definitions SET name = 'Out of band' WHERE input_id = ?`, input.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoInputDraft(owner, project.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("undo after out-of-band Input edit=%v, want conflict", err)
	}
	inputs, err := s.ListInputs(ctx, revision.ID)
	if err != nil || len(inputs) != 1 || inputs[0].Name != "Out of band" {
		t.Fatalf("conflicting undo changed Input: inputs=%+v err=%v", inputs, err)
	}
}

func TestInputDraftHistoryUndoRespectsShowLockAndKeepsBoundedHistory(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	project, publishedSnapshot, _ := createSessionFoundationFixture(t, s)
	draft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "qualify Input Draft history limits")
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithInputDraftHistoryActor(ctx, "owner")
	input, err := s.CreateInput(owner, domain.InputDefinition{
		RevisionID: draft.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	show, err := s.CreateSession(ctx, publishedSnapshot.ID, domain.SessionShow, "Input history lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoInputDraft(owner, project.ID); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("Input Undo during SHOW=%v, want configuration lock", err)
	}
	if err := s.EndSession(ctx, show.ID, domain.SessionAborted); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 55; index++ {
		input.Name = "GO " + string(rune('A'+index%26))
		if _, err := s.UpdateInput(owner, input); err != nil {
			t.Fatalf("record Input edit %d: %v", index+1, err)
		}
	}
	var count int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM input_draft_history_events WHERE revision_id = ? AND actor_id = ?`, draft.ID, "owner").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 50 {
		t.Fatalf("retained Input history events=%d, want 50", count)
	}
	for index := 0; index < 50; index++ {
		if _, err := s.UndoInputDraft(owner, project.ID); err != nil {
			t.Fatalf("undo retained Input event %d: %v", index+1, err)
		}
	}
	if _, err := s.UndoInputDraft(owner, project.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Undo beyond retained Input history=%v, want not found", err)
	}
	status, err := s.InputDraftHistory(owner, project.ID)
	if err != nil || status.CanUndo || !status.CanRedo {
		t.Fatalf("bounded Input history status=%+v err=%v", status, err)
	}
}
