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

func TestInjectTestRequiresExplicitConfirmationForCriticalOutput(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Critical Route Test", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "TEST", SourceRef: "manual:test", EventType: "test.value", ValueSchema: json.RawMessage(`{"type":"boolean"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := s.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "CRITICAL-OUT", TargetRef: "SIM", CapabilityKey: "sim.test", ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "CRITICAL"})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	if _, err := s.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "Critical Test Route", InputID: input.ID, PriorityClass: domain.PriorityP2, ErrorPolicy: json.RawMessage(`{"on_error":"STOP_ROUTE"}`), Enabled: true}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputID, Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "critical route test")
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	engine := routing.New(s, registry)

	blocked := engine.InjectTest(ctx, session.ID, criticalInjectCommand(t, runtimeSnapshot, input.ID, false))
	if blocked.Status != contracts.CommandRejected || blocked.Error == nil || blocked.Error.ErrorCode != "SAFETY_CONFIRMATION_REQUIRED" || blocked.Error.Category != "SAFETY_BLOCK" {
		t.Fatalf("blocked=%#v", blocked)
	}
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasEventType(events, "route.action.completed") {
		t.Fatal("critical test route executed without explicit confirmation")
	}
	if !hasSafetyBlock(events) {
		t.Fatalf("missing safety-block trace; events=%v", routeEventTypes(events))
	}

	confirmed := engine.InjectTest(ctx, session.ID, criticalInjectCommand(t, runtimeSnapshot, input.ID, true))
	if confirmed.Status != contracts.CommandCompleted {
		t.Fatalf("confirmed=%#v", confirmed)
	}
	events, err = s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEventType(events, "route.action.completed") {
		t.Fatal("confirmed critical test route did not execute")
	}
}

func criticalInjectCommand(t *testing.T, runtimeSnapshot domain.RuntimeSnapshot, inputID string, confirm bool) contracts.CommandEnvelope {
	t.Helper()
	id, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(routing.InjectTestPayload{InputID: inputID, Value: json.RawMessage(`true`), ConfirmCritical: confirm})
	if err != nil {
		t.Fatal(err)
	}
	return contracts.CommandEnvelope{
		CommandID:         id,
		CommandType:       routing.InputInjectTestCommandType,
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          time.Now().UTC(),
		ProjectID:         runtimeSnapshot.ProjectID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer:            "test.operator",
		Priority:          "P2",
		Payload:           payload,
	}
}

func hasEventType(events []contracts.EventEnvelope, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func hasSafetyBlock(events []contracts.EventEnvelope) bool {
	for _, event := range events {
		if event.EventType != "route.action.failed" {
			continue
		}
		var payload struct {
			ErrorCode string `json:"error_code"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil && payload.ErrorCode == "SAFETY_CONFIRMATION_REQUIRED" {
			return true
		}
	}
	return false
}
