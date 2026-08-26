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
	if safetyHasEventType(events, "route.action.completed") {
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
	if !safetyHasEventType(events, "route.action.completed") {
		t.Fatal("confirmed critical test route did not execute")
	}
}

func TestCriticalPreflightBlocksEntireMixedRouteBeforeDebounceOrDispatch(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Mixed Critical Route Test", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "TEST", SourceRef: "manual:test", EventType: "test.value", ValueSchema: json.RawMessage(`{"type":"boolean"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	normal, err := s.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "NORMAL-OUT", TargetRef: "SIM", CapabilityKey: "sim.test", ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "NORMAL"})
	if err != nil {
		t.Fatal(err)
	}
	critical, err := s.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "CRITICAL-OUT", TargetRef: "SIM", CapabilityKey: "sim.test", ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "SAFETY_CRITICAL"})
	if err != nil {
		t.Fatal(err)
	}
	normalID, criticalID := normal.ID, critical.ID
	debounceMS := int64(500)
	if _, err := s.CreateRouteWithActions(ctx, domain.Route{
		RevisionID:    revision.ID,
		Name:          "Mixed Test Route",
		InputID:       input.ID,
		DebounceMS:    &debounceMS,
		PriorityClass: domain.PriorityP2,
		ErrorPolicy:   json.RawMessage(`{"on_error":"STOP_ROUTE"}`),
		Enabled:       true,
	}, []domain.RouteAction{
		{OrderIndex: 0, OutputID: &normalID, Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`)},
		{OrderIndex: 1, OutputID: &criticalID, Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "mixed critical route test")
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	engine := routing.New(s, registry)

	blocked := engine.InjectTest(ctx, session.ID, criticalInjectCommand(t, runtimeSnapshot, input.ID, false))
	if blocked.Status != contracts.CommandRejected || blocked.Error == nil || blocked.Error.ErrorCode != "SAFETY_CONFIRMATION_REQUIRED" {
		t.Fatalf("blocked=%#v", blocked)
	}
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countSafetyEventType(events, "route.action.completed") != 0 {
		t.Fatalf("mixed Route partially dispatched before safety block; events=%v", routeEventTypes(events))
	}
	if safetyHasEventType(events, "route.triggered") {
		t.Fatalf("unconfirmed mixed Route was marked triggered before safety block; events=%v", routeEventTypes(events))
	}
	if !hasSafetyDisposition(events, "SAFETY_BLOCKED") {
		t.Fatalf("missing route-level safety preflight trace; events=%v", routeEventTypes(events))
	}

	// The rejected attempt must not consume debounce state. A newly confirmed
	// command immediately after the block must execute both RouteActions.
	confirmed := engine.InjectTest(ctx, session.ID, criticalInjectCommand(t, runtimeSnapshot, input.ID, true))
	if confirmed.Status != contracts.CommandCompleted {
		t.Fatalf("confirmed=%#v", confirmed)
	}
	events, err = s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := countSafetyEventType(events, "route.action.completed"); got != 2 {
		t.Fatalf("confirmed mixed Route completed actions=%d want 2; events=%v", got, routeEventTypes(events))
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

func safetyHasEventType(events []contracts.EventEnvelope, eventType string) bool {
	return countSafetyEventType(events, eventType) > 0
}

func countSafetyEventType(events []contracts.EventEnvelope, eventType string) int {
	count := 0
	for _, event := range events {
		if event.EventType == eventType {
			count++
		}
	}
	return count
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

func hasSafetyDisposition(events []contracts.EventEnvelope, want string) bool {
	for _, event := range events {
		if event.EventType != "route.evaluated" {
			continue
		}
		var payload struct {
			Disposition string `json:"disposition"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil && payload.Disposition == want {
			return true
		}
	}
	return false
}
