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

func TestRouteDraftHistoryUndoRedoPreservesRouteAndActionIdentity(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Route Draft history"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "GO Cue", OrderIndex: 0,
		Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithRouteDraftHistoryActor(ctx, "owner")
	cueID := cue.ID
	route, err := s.CreateRouteWithActions(owner, domain.Route{
		RevisionID: revision.ID, Name: "GO Route", InputID: input.ID,
		ConditionDefinition: json.RawMessage(`{"mode":"always"}`),
		TransformDefinition: json.RawMessage(`null`), ErrorPolicy: json.RawMessage(`{"mode":"FAIL_SOFT"}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}, []domain.RouteAction{{CueID: &cueID, Parameters: json.RawMessage(`{"source":"button"}`)}})
	if err != nil {
		t.Fatal(err)
	}
	original := route
	if len(original.Actions) != 1 || original.Actions[0].ID == "" {
		t.Fatalf("created route action lacks persistent identity: %+v", original)
	}
	status, err := s.RouteDraftHistory(owner, project.ID)
	if err != nil || !status.CanUndo || status.CanRedo || status.UndoOperation != "CREATE" {
		t.Fatalf("history after route create=%+v err=%v", status, err)
	}
	otherEditor, err := s.RouteDraftHistory(store.WithRouteDraftHistoryActor(ctx, "other-editor"), project.ID)
	if err != nil || otherEditor.CanUndo || otherEditor.CanRedo {
		t.Fatalf("Route history leaked across editors: %+v err=%v", otherEditor, err)
	}
	if _, err := s.UndoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("undo route create: %v", err)
	}
	if routes, err := s.ListRoutes(ctx, revision.ID); err != nil || len(routes) != 0 {
		t.Fatalf("undo create left routes=%+v err=%v", routes, err)
	}
	if _, err := s.RedoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("redo route create: %v", err)
	}
	routes, err := s.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 1 || !reflect.DeepEqual(routes[0], original) {
		t.Fatalf("redo create changed route/action identity or payload: routes=%+v want=%+v err=%v", routes, original, err)
	}

	updatedInput := int64(275)
	updated := original
	updated.Name = "GO Route revised"
	updated.Enabled = false
	updated.DelayMS = &updatedInput
	updated.Actions = []domain.RouteAction{{
		CueID: &cueID, Parameters: json.RawMessage(`{"source":"console"}`),
	}}
	updated, err = s.UpdateRouteWithActions(owner, updated, updated.Actions)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Actions[0].ID == original.Actions[0].ID {
		t.Fatal("fixture expected update to issue a new persisted Route action identity")
	}
	if _, err := s.UndoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("undo route edit: %v", err)
	}
	routes, err = s.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 1 || !reflect.DeepEqual(routes[0], original) {
		t.Fatalf("undo edit failed to restore exact Route state: routes=%+v want=%+v err=%v", routes, original, err)
	}
	if _, err := s.RedoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("redo route edit: %v", err)
	}
	routes, err = s.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 1 || !reflect.DeepEqual(routes[0], updated) {
		t.Fatalf("redo edit failed to restore exact Route state: routes=%+v want=%+v err=%v", routes, updated, err)
	}

	if err := s.DeleteRoute(owner, revision.ID, route.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("undo route delete: %v", err)
	}
	routes, err = s.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 1 || !reflect.DeepEqual(routes[0], updated) {
		t.Fatalf("undo delete failed to restore exact Route state: routes=%+v want=%+v err=%v", routes, updated, err)
	}
	if _, err := s.RedoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("redo route delete: %v", err)
	}
	if routes, err := s.ListRoutes(ctx, revision.ID); err != nil || len(routes) != 0 {
		t.Fatalf("redo delete left routes=%+v err=%v", routes, err)
	}
}

func TestRouteDraftHistoryRejectsUndoAfterOutOfBandEdit(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Route history conflict"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithRouteDraftHistoryActor(ctx, "owner")
	route, err := s.CreateRouteWithActions(owner, domain.Route{
		RevisionID: revision.ID, Name: "Original", InputID: input.ID,
		ConditionDefinition: json.RawMessage(`null`), TransformDefinition: json.RawMessage(`null`),
		ErrorPolicy: json.RawMessage(`{}`), PriorityClass: domain.PriorityP2, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE routes SET name = 'Out of band' WHERE route_id = ?`, route.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoRouteDraft(owner, project.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("undo after Route changed outside history = %v, want conflict", err)
	}
	current, err := s.ListRoutes(ctx, revision.ID)
	if err != nil || len(current) != 1 || current[0].Name != "Out of band" {
		t.Fatalf("conflicting undo changed the newer Route: routes=%+v err=%v", current, err)
	}
}

func TestRouteDraftHistoryUndoRespectsActiveShowConfigurationLock(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, publishedSnapshot, _ := createSessionFoundationFixture(t, s)
	draft, err := s.EnsureProjectDraft(ctx, project.ID, "owner", "route history SHOW lock regression")
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: draft.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithRouteDraftHistoryActor(ctx, "owner")
	route, err := s.CreateRouteWithActions(owner, domain.Route{
		RevisionID: draft.ID, Name: "Draft Route", InputID: input.ID,
		PriorityClass: domain.PriorityP2, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSession(ctx, publishedSnapshot.ID, domain.SessionShow, "Route history SHOW lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UndoRouteDraft(owner, project.ID); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("Undo during active SHOW err=%v, want configuration lock", err)
	}
	routes, err := s.ListRoutes(ctx, draft.ID)
	if err != nil || len(routes) != 1 || routes[0].ID != route.ID {
		t.Fatalf("locked Undo changed Draft Route: routes=%+v err=%v", routes, err)
	}
}

func TestRouteDraftHistoryCanUndoAfterBranchingFromRedo(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Route history branch"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID, Name: "GO", SourceRef: "osc:/go", EventType: "osc.message", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := store.WithRouteDraftHistoryActor(ctx, "owner")
	create := func(name string) domain.Route {
		t.Helper()
		route, err := s.CreateRouteWithActions(owner, domain.Route{
			RevisionID: revision.ID, Name: name, InputID: input.ID,
			PriorityClass: domain.PriorityP2, Enabled: true,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return route
	}

	first := create("First")
	create("Second")
	if _, err := s.UndoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Second before branching: %v", err)
	}
	third := create("Third")
	if _, err := s.UndoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("undo Third after branching from Redo: %v", err)
	}
	status, err := s.RouteDraftHistory(owner, project.ID)
	if err != nil || !status.CanUndo || !status.CanRedo {
		t.Fatalf("branch history after undo Third=%+v err=%v; want First undo and Third redo", status, err)
	}
	if _, err := s.UndoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("undo First after branching: %v", err)
	}
	routes, err := s.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 0 {
		t.Fatalf("undo First after branching left routes=%+v err=%v", routes, err)
	}
	if _, err := s.RedoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("redo First after branching: %v", err)
	}
	if _, err := s.RedoRouteDraft(owner, project.ID); err != nil {
		t.Fatalf("redo Third after branching: %v", err)
	}
	routes, err = s.ListRoutes(ctx, revision.ID)
	if err != nil || len(routes) != 2 || routes[0].ID != first.ID || routes[1].ID != third.ID {
		t.Fatalf("branch Redo failed to preserve retained Route history: routes=%+v err=%v", routes, err)
	}
}
