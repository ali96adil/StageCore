package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestAllM0DefinitionsSurviveDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Fixed{Time: fixedTime})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Restart Show"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID,
		Name:       "Intro",
		OrderIndex: 0,
		Enabled:    true,
	}, []domain.Action{{
		OrderIndex:    0,
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: "osc.send",
		Parameters:    json.RawMessage(`{"address":"/intro"}`),
		Enabled:       true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:     project.ID,
		LogicalName:   "VIDEO-MAIN",
		LogicalType:   "MACHINE_ROLE",
		TargetRef:     "VIDEO-MAIN",
		ProjectConfig: json.RawMessage(`{"role":"VIDEO-MAIN"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID,
		Name:       "TEST-GO",
		SourceRef:  "test",
		EventType:  "input.test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := s.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID:    revision.ID,
		Name:          "Video Go",
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: "osc.send",
	})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	route, err := s.CreateRouteWithActions(ctx, domain.Route{
		RevisionID:    revision.ID,
		Name:          "Test Route",
		InputID:       input.ID,
		PriorityClass: domain.PriorityP2,
		Enabled:       true,
	}, []domain.RouteAction{{
		OrderIndex: 0,
		OutputID:   &outputID,
		Parameters: json.RawMessage(`{"value":1}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	h, err = db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s = store.New(h.DB, clock.Real{})

	loadedProject, err := s.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedProject.CurrentRevisionID != revision.ID {
		t.Fatalf("current revision=%q want %q", loadedProject.CurrentRevisionID, revision.ID)
	}
	cues, err := s.ListCues(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].ID != cue.ID || len(cues[0].Actions) != 1 {
		t.Fatalf("cues after reopen=%#v", cues)
	}
	aliases, err := s.ListAliases(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 || aliases[0].ID != alias.ID {
		t.Fatalf("aliases after reopen=%#v", aliases)
	}
	inputs, err := s.ListInputs(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0].ID != input.ID {
		t.Fatalf("inputs after reopen=%#v", inputs)
	}
	outputs, err := s.ListOutputs(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(outputs) != 1 || outputs[0].ID != output.ID {
		t.Fatalf("outputs after reopen=%#v", outputs)
	}
	routes, err := s.ListRoutes(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].ID != route.ID || len(routes[0].Actions) != 1 {
		t.Fatalf("routes after reopen=%#v", routes)
	}
}

func TestRouteRejectsCrossRevisionReferences(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)

	_, revisionA, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Show A"})
	if err != nil {
		t.Fatal(err)
	}
	_, revisionB, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Show B"})
	if err != nil {
		t.Fatal(err)
	}

	inputA, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revisionA.ID,
		Name:       "A Input",
		SourceRef:  "test",
		EventType:  "input.test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateRouteWithActions(ctx, domain.Route{
		RevisionID:    revisionB.ID,
		Name:          "Illegal Input Route",
		InputID:       inputA.ID,
		PriorityClass: domain.PriorityP2,
		Enabled:       true,
	}, nil)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("cross-revision input err=%v, want ErrInvalidInput", err)
	}

	inputB, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revisionB.ID,
		Name:       "B Input",
		SourceRef:  "test",
		EventType:  "input.test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	outputA, err := s.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID:    revisionA.ID,
		Name:          "A Output",
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: "osc.send",
	})
	if err != nil {
		t.Fatal(err)
	}
	outputAID := outputA.ID
	_, err = s.CreateRouteWithActions(ctx, domain.Route{
		RevisionID:    revisionB.ID,
		Name:          "Illegal Output Route",
		InputID:       inputB.ID,
		PriorityClass: domain.PriorityP2,
		Enabled:       true,
	}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputAID}})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("cross-revision output err=%v, want ErrInvalidInput", err)
	}
	routes, err := s.ListRoutes(ctx, revisionB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 0 {
		t.Fatalf("invalid cross-revision route survived rollback: %#v", routes)
	}
}
