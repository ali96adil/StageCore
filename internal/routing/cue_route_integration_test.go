package routing_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestInjectedInputTriggersExplicitCueThroughCueGo(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "M3 Cue Route", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: "10", Name: "Routed Cue", OrderIndex: 0, Enabled: true}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`), PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "TEST-CUE", SourceRef: "manual:test", EventType: "test.bool", ValueSchema: json.RawMessage(`{"type":"boolean"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cueID := cue.ID
	if _, err := s.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "TEST-CUE -> Routed Cue", InputID: input.ID, ConditionDefinition: json.RawMessage(`{"operator":"boolean_is","value":true}`), PriorityClass: domain.PriorityP2, ErrorPolicy: json.RawMessage(`{"on_error":"STOP_ROUTE"}`), Enabled: true}, []domain.RouteAction{{OrderIndex: 0, CueID: &cueID, Parameters: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "route cue")
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	commandID, _ := stageid.New()
	payload, _ := json.Marshal(routing.InjectTestPayload{InputID: input.ID, Value: json.RawMessage(`true`)})
	result := routing.New(s, registry).InjectTest(ctx, session.ID, contracts.CommandEnvelope{CommandID: commandID, CommandType: routing.InputInjectTestCommandType, SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(), ProjectID: runtimeSnapshot.ProjectID, RuntimeSnapshotID: runtimeSnapshot.ID, Issuer: "test.operator", Priority: "P2", Payload: payload})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}
	cueExecutions, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil || len(cueExecutions) != 1 {
		t.Fatalf("cue executions=%#v err=%v", cueExecutions, err)
	}
	if cueExecutions[0].CueID != cue.ID || cueExecutions[0].Result != domain.ExecutionCompleted || cueExecutions[0].TriggerSource == "" {
		t.Fatalf("cue execution=%#v", cueExecutions[0])
	}
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var foundRouteCompletion bool
	for _, event := range events {
		if event.EventType != "route.action.completed" {
			continue
		}
		var body struct {
			CueID         string `json:"cue_id"`
			CommandStatus string `json:"command_status"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil {
			t.Fatal(err)
		}
		if body.CueID == cue.ID && body.CommandStatus == string(contracts.CommandCompleted) {
			foundRouteCompletion = true
		}
	}
	if !foundRouteCompletion {
		t.Fatal("missing route.action.completed trace for routed cue")
	}
}
