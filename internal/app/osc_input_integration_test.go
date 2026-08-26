package app_test

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestAppExternalOSCInputReachesRoutingRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	application, err := app.Open(ctx, config.Config{
		DataRoot:      root,
		VaultRoot:     filepath.Join(root, "vault"),
		Listen:        "127.0.0.1:0",
		OSCPluginPath: buildProductOSCPlugin(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()

	_, revision, err := application.Store.CreateProject(ctx, store.CreateProjectParams{Name: "App OSC Input", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := application.Store.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "OSC-APP-GO", SourceRef: "/app/input", EventType: "osc.message", ValueSchema: json.RawMessage(`{"type":"number"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := application.Store.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "SIM", TargetRef: "SIM", CapabilityKey: "sim.test", ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "NORMAL"})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	if _, err := application.Store.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "App OSC Route", InputID: input.ID, ConditionDefinition: json.RawMessage(`{"operator":"equals","value":7}`), PriorityClass: domain.PriorityP2, ErrorPolicy: json.RawMessage(`{"on_error":"STOP_ROUTE"}`), Enabled: true}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputID, Parameters: json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := application.Store.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(application.Store).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := application.Store.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "App external OSC input")
	if err != nil {
		t.Fatal(err)
	}

	listen, err := application.StartOSCInput(ctx, session.ID, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(listen)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 {
		t.Fatalf("listen=%q", listen)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- application.ServeOSCInput(ctx) }()

	if _, err := (osc.Sender{}).Send(ctx, osc.Endpoint{Host: "127.0.0.1", Port: port}, osc.Message{Address: "/app/input", Arguments: []osc.Argument{{Type: "int32", Value: int32(7)}}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		events, err := application.Store.ListEvents(context.Background(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if appHasRouteCompletion(events) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("external app OSC route did not complete; events=%v", appEventTypes(events))
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
		t.Fatal("App OSC input service did not stop")
	}
}

func appHasRouteCompletion(events []contracts.EventEnvelope) bool {
	for _, event := range events {
		if event.EventType == "route.action.completed" {
			return true
		}
	}
	return false
}

func appEventTypes(events []contracts.EventEnvelope) []string {
	out := make([]string, len(events))
	for i := range events {
		out[i] = events[i].EventType
	}
	return out
}
