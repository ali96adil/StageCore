package snapshot_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestBuilderCapturesRoutingDefinitionsImmutably(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Routing Snapshot Show"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{
		RevisionID: revision.ID,
		Name:       "TEST-GO",
		SourceRef:  "manual:test",
		EventType:  "test.value",
		ValueSchema: json.RawMessage(`{"type":"number"}`),
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := s.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID:    revision.ID,
		Name:          "VIDEO-GO",
		TargetRef:     "VIDEO-MAIN",
		CapabilityKey: "osc.send",
		ValueSchema:   json.RawMessage(`{"type":"object"}`),
		Criticality:   "NORMAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	debounce := int64(250)
	route, err := s.CreateRouteWithActions(ctx, domain.Route{
		RevisionID:          revision.ID,
		Name:                "TEST-GO -> VIDEO-GO",
		InputID:             input.ID,
		ConditionDefinition: json.RawMessage(`{"operator":"equals","value":1}`),
		TransformDefinition: json.RawMessage(`null`),
		DebounceMS:          &debounce,
		PriorityClass:       domain.PriorityP2,
		ErrorPolicy:         json.RawMessage(`{"on_error":"STOP_ROUTE"}`),
		Enabled:             true,
	}, []domain.RouteAction{{
		OrderIndex: 0,
		OutputID:   &outputID,
		Parameters: json.RawMessage(`{"address":"/route/go","arguments":[]}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 3 || len(manifest.Inputs) != 1 || len(manifest.Outputs) != 1 || len(manifest.Routes) != 1 {
		t.Fatalf("routing manifest=%#v", manifest)
	}
	if manifest.Routes[0].ID != route.ID || manifest.Routes[0].DebounceMS == nil || *manifest.Routes[0].DebounceMS != 250 {
		t.Fatalf("route snapshot=%#v", manifest.Routes[0])
	}
	if len(manifest.Routes[0].Actions) != 1 || manifest.Routes[0].Actions[0].OutputID == nil || *manifest.Routes[0].Actions[0].OutputID != output.ID {
		t.Fatalf("route actions=%#v", manifest.Routes[0].Actions)
	}

	// Mutate live definition tables after publish to prove the already-built
	// manifest remains independent from mutable project storage.
	if _, err := h.DB.ExecContext(ctx, `UPDATE routes SET enabled = 0, debounce_ms = 9999 WHERE route_id = ?`, route.ID); err != nil {
		t.Fatal(err)
	}
	if !manifest.Routes[0].Enabled || *manifest.Routes[0].DebounceMS != 250 {
		t.Fatal("captured routing manifest changed with live definition state")
	}
}
