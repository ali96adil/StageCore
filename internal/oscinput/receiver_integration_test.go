package oscinput_test

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
	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/oscinput"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestUDPOSCInputFeedsSameRoutingEngine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "OSC Input Route", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "OSC-GO", SourceRef: "/sensor/go", EventType: "osc.message", ValueSchema: json.RawMessage(`{"type":"number"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := s.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "SIM-OUT", TargetRef: "SIM", CapabilityKey: "sim.test", ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "NORMAL"})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	if _, err := s.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "OSC-GO -> SIM", InputID: input.ID, ConditionDefinition: json.RawMessage(`{"operator":"equals","value":1}`), PriorityClass: domain.PriorityP2, ErrorPolicy: json.RawMessage(`{"on_error":"STOP_ROUTE"}`), Enabled: true}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputID, Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "OSC input rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	engine := routing.New(s, registry)
	receiver, err := oscinput.Listen("127.0.0.1:0", engine, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	serveDone := make(chan error, 1)
	go func() { serveDone <- receiver.Serve(ctx) }()

	addr := receiver.LocalAddr()
	if addr == nil {
		t.Fatal("receiver missing local address")
	}
	if _, err := (osc.Sender{}).Send(ctx, osc.Endpoint{Host: "127.0.0.1", Port: addr.Port}, osc.Message{Address: "/sensor/go", Arguments: []osc.Argument{{Type: "int32", Value: 1}}}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		events, err := s.ListEvents(context.Background(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if hasCompletedRouteAction(events) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for routed OSC input; events=%v", eventTypes(events))
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("OSC input receiver did not stop after context cancellation")
	}
}

func hasCompletedRouteAction(events []contracts.EventEnvelope) bool {
	for _, event := range events {
		if event.EventType == "route.action.completed" {
			return true
		}
	}
	return false
}

func eventTypes(events []contracts.EventEnvelope) []string {
	out := make([]string, len(events))
	for i := range events {
		out[i] = events[i].EventType
	}
	return out
}
