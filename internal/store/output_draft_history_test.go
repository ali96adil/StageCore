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

func TestOutputDraftHistoryRecordsCreateAtomically(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Output Draft history"})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithOutputDraftHistoryActor(ctx, "owner")
	output, err := s.CreateOutput(owner, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Main OSC", TargetRef: "PROJECTOR-MAIN", CapabilityKey: "osc.send",
		ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "NORMAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	var operation, outputID string
	if err := handle.DB.QueryRowContext(ctx, `SELECT operation, output_id FROM output_draft_history_events WHERE revision_id = ? AND actor_id = ? AND sequence = 1`, revision.ID, "owner").Scan(&operation, &outputID); err != nil {
		t.Fatalf("read atomic Output create history for Project %s: %v", project.ID, err)
	}
	if operation != "CREATE" || outputID != output.ID {
		t.Fatalf("Output create history=(%q,%q), want CREATE and %q", operation, outputID, output.ID)
	}
	created := output
	status, err := s.OutputDraftHistory(owner, project.ID)
	if err != nil || !status.CanUndo || status.CanRedo || status.UndoOperation != "CREATE" {
		t.Fatalf("Output history after create=%+v err=%v", status, err)
	}
	if other, err := s.OutputDraftHistory(store.WithOutputDraftHistoryActor(ctx, "other-editor"), project.ID); err != nil || other.CanUndo || other.CanRedo {
		t.Fatalf("Output history leaked across editors: %+v err=%v", other, err)
	}
	output.Name = "Main OSC revised"
	output.TargetRef = "PROJECTOR-BACKUP"
	output.CapabilityKey = "osc.send"
	output.ValueSchema = json.RawMessage(`{"type":"string"}`)
	output.Criticality = "CRITICAL"
	updated, err := s.UpdateOutput(owner, output)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteOutput(owner, revision.ID, output.ID); err != nil {
		t.Fatal(err)
	}
	if outputs, err := s.ListOutputs(ctx, revision.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("Output delete left outputs=%+v err=%v", outputs, err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Output delete: %v", err)
	}
	outputs, err := s.ListOutputs(ctx, revision.ID)
	if err != nil || len(outputs) != 1 || !reflect.DeepEqual(outputs[0], updated) {
		t.Fatalf("undo delete did not restore Output: outputs=%+v want=%+v err=%v", outputs, updated, err)
	}
	if _, err := s.RedoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("redo Output delete: %v", err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Output delete again: %v", err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Output edit: %v", err)
	}
	outputs, err = s.ListOutputs(ctx, revision.ID)
	if err != nil || len(outputs) != 1 || !reflect.DeepEqual(outputs[0], created) {
		t.Fatalf("undo edit did not restore exact Output: outputs=%+v want=%+v err=%v", outputs, created, err)
	}
	if _, err := s.RedoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("redo Output edit: %v", err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("return Output to the created state: %v", err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	outputID = output.ID
	route, err := s.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "GO Route", InputID: input.ID, Enabled: true},
		[]domain.RouteAction{{OutputID: &outputID, Parameters: json.RawMessage(`{"value":1}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteOutput(owner, revision.ID, output.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("delete Output with Route reference=%v, want conflict", err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("undo Output create with Route reference=%v, want conflict", err)
	}
	if err := s.DeleteRoute(ctx, revision.ID, route.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Output create after removing Route: %v", err)
	}
	if outputs, err := s.ListOutputs(ctx, revision.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("Output create Undo left outputs=%+v err=%v", outputs, err)
	}
}

func TestOutputDraftHistoryBranchAfterUndoRetainsEarlierUndo(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Output history branch"})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithOutputDraftHistoryActor(ctx, "owner")
	original, err := s.CreateOutput(owner, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Original", TargetRef: "PROJECTOR", CapabilityKey: "osc.send",
		ValueSchema: json.RawMessage(`{}`), Criticality: "NORMAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	edited := original
	edited.Name = "Discarded redo"
	if _, err := s.UpdateOutput(owner, edited); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo edit before branching: %v", err)
	}
	branch, err := s.CreateOutput(owner, domain.OutputDefinition{
		RevisionID: revision.ID, Name: "Branch", TargetRef: "PROJECTOR", CapabilityKey: "osc.send",
		ValueSchema: json.RawMessage(`{}`), Criticality: "NORMAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.OutputDraftHistory(owner, project.ID)
	if err != nil || !status.CanUndo || status.CanRedo {
		t.Fatalf("branch history status=%+v err=%v", status, err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo branch: %v", err)
	}
	outputs, err := s.ListOutputs(ctx, revision.ID)
	if err != nil || len(outputs) != 1 || outputs[0].ID != original.ID || outputs[0].Name != original.Name {
		t.Fatalf("branch undo did not restore original Output: outputs=%+v err=%v", outputs, err)
	}
	status, err = s.OutputDraftHistory(owner, project.ID)
	if err != nil || !status.CanUndo || !status.CanRedo || status.UndoOperation != "CREATE" || status.RedoOperation != "CREATE" {
		t.Fatalf("history cursor lost the earlier Output create after branch: status=%+v err=%v", status, err)
	}
	if _, err := s.UndoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("undo earlier Output create after branch: %v", err)
	}
	if outputs, err := s.ListOutputs(ctx, revision.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("earlier Output create remained after Undo: outputs=%+v err=%v", outputs, err)
	}
	if _, err := s.RedoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("redo earlier Output create: %v", err)
	}
	if _, err := s.RedoOutputDraft(owner, project.ID); err != nil {
		t.Fatalf("redo branched Output create: %v", err)
	}
	outputs, err = s.ListOutputs(ctx, revision.ID)
	if err != nil || len(outputs) != 2 {
		t.Fatalf("Redo did not restore both original and branched Outputs: outputs=%+v err=%v", outputs, err)
	}
	foundOriginal, foundBranch := false, false
	for _, output := range outputs {
		foundOriginal = foundOriginal || output.ID == original.ID
		foundBranch = foundBranch || output.ID == branch.ID
	}
	if !foundOriginal || !foundBranch {
		t.Fatalf("Output IDs changed across branch Undo/Redo: original=%t branch=%t outputs=%+v", foundOriginal, foundBranch, outputs)
	}
}
