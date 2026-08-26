package routing_test

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestRoutedInputDuplicateAfterRestartDoesNotReplayOSC(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	f := newOSCOutputRouteFixture(t, conn.LocalAddr().(*net.UDPAddr).Port, nil, true, nil)
	pluginPath := buildRoutingOSCPlugin(t)
	host := pluginhost.New(pluginPath, nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}
	command := injectCommand(t, f, json.RawMessage(`1`))
	first := routing.New(f.store, registry).InjectTest(context.Background(), f.session.ID, command)
	if first.Status != contracts.CommandCompleted {
		t.Fatalf("first=%#v", first)
	}
	readOSC(t, conn)
	eventsBefore, err := f.store.ListEvents(context.Background(), f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	dataRoot := filepath.Dir(filepath.Dir(f.h.Path))
	host.Close()
	if err := f.h.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := db.Open(context.Background(), db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedStore := store.New(reopened.DB, clock.Real{})
	newHost := pluginhost.New(pluginPath, nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
	defer newHost.Close()
	newRegistry := capability.NewRegistry()
	if err := newRegistry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(newHost)); err != nil {
		t.Fatal(err)
	}
	second := routing.New(reopenedStore, newRegistry).InjectTest(context.Background(), f.session.ID, command)
	if second.Status != contracts.CommandCompleted {
		t.Fatalf("duplicate after restart=%#v", second)
	}
	assertNoOSC(t, conn)
	eventsAfter, err := reopenedStore.ListEvents(context.Background(), f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("duplicate replayed route history: before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}
}

func TestRouteOutputFailureIsExplicitAndNotFalseSuccess(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "M3 Failure Route", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "TEST", SourceRef: "manual:test", EventType: "test.value", ValueSchema: json.RawMessage(`{"type":"number"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := s.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "BROKEN-OSC", TargetRef: "MISSING-TARGET", CapabilityKey: oscplugin.CapabilityOSCSend, ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "NORMAL"})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	if _, err := s.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "Broken Route", InputID: input.ID, PriorityClass: domain.PriorityP2, ErrorPolicy: json.RawMessage(`{"on_error":"STOP_ROUTE"}`), Enabled: true}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputID, Parameters: json.RawMessage(`{"address":"/never","arguments":[]}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "failure trace")
	if err != nil {
		t.Fatal(err)
	}
	host := pluginhost.New("/unused/missing-target-prevents-start", nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}
	fixture := routeFixture{h: h, store: s, snapshot: runtimeSnapshot, session: session, input: input}
	result := routing.New(s, registry).InjectTest(ctx, session.ID, injectCommand(t, fixture, json.RawMessage(`1`)))
	if result.Status != contracts.CommandFailed || result.Error == nil || result.Error.ErrorCode != "TARGET_NOT_FOUND" {
		t.Fatalf("result=%#v", result)
	}
	if host.Ready() != nil {
		t.Fatal("missing Snapshot target must fail before Plugin process start")
	}
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var failed bool
	for _, event := range events {
		if event.EventType != "route.action.failed" {
			continue
		}
		var payload struct {
			Result    string `json:"result"`
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Result == string(domain.ExecutionFailed) && payload.ErrorCode == "TARGET_NOT_FOUND" {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("missing truthful failed RouteAction trace; events=%v", routeEventTypes(events))
	}
}
