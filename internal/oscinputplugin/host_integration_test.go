package oscinputplugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/oscinputplugin"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestExternalOSCPuginInputFeedsRoutingEngine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "External OSC Input Route", CreatedBy: "test"})
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
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "external OSC input rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	engine := routing.New(s, registry)
	host := oscinputplugin.New(buildOSCPlugin(t), "127.0.0.1:0", nil, inputManifest(true), engine, session.ID)
	defer host.Close()
	if err := host.Start(ctx); err != nil {
		t.Fatal(err)
	}
	listenHost, listenPort, err := net.SplitHostPort(host.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(listenHost); ip == nil || !ip.IsLoopback() {
		t.Fatalf("plugin escaped loopback gate: %q", host.LocalAddr())
	}
	port, err := strconv.Atoi(listenPort)
	if err != nil || port < 1 {
		t.Fatalf("invalid plugin listen port %q", listenPort)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- host.Serve(ctx) }()

	// A malformed datagram is isolated by the external Plugin and must not kill
	// the input process or synthesize a routing event.
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte{0x01, 0x02, 0x03})
	_ = conn.Close()

	if _, err := (osc.Sender{}).Send(ctx, osc.Endpoint{Host: "127.0.0.1", Port: port}, osc.Message{Address: "/sensor/go", Arguments: []osc.Argument{{Type: "int32", Value: int32(1)}}}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		events, err := s.ListEvents(context.Background(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if hasCompletedRouteAction(events) && hasOSCInputSource(events) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for external routed OSC input; events=%v", eventTypes(events))
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
		t.Fatal("external OSC input host did not stop after cancellation")
	}
}

func TestOSCInputPermissionAndLoopbackGateBeforeProcessStart(t *testing.T) {
	engine := (*routing.Engine)(nil)
	denied := oscinputplugin.New("/does/not/matter", "127.0.0.1:9000", nil, inputManifest(false), engine, "session")
	if err := denied.Start(context.Background()); err == nil {
		t.Fatal("expected unavailable engine to reject start")
	}

	// Use a non-nil zero-value Engine so the security checks are reached without
	// requiring a database. All failures below must happen before process spawn.
	engine = &routing.Engine{}
	denied = oscinputplugin.New("/does/not/matter", "127.0.0.1:9000", nil, inputManifest(false), engine, "session")
	if err := denied.Start(context.Background()); !errors.Is(err, oscinputplugin.ErrPermissionDenied) {
		t.Fatalf("permission err=%v", err)
	}
	unsafe := oscinputplugin.New("/does/not/matter", "0.0.0.0:9000", nil, inputManifest(true), engine, "session")
	if err := unsafe.Start(context.Background()); err == nil {
		t.Fatal("expected non-loopback OSC input to be blocked before process start")
	}
	invalidPort := oscinputplugin.New("/does/not/matter", "127.0.0.1:not-a-port", nil, inputManifest(true), engine, "session")
	if err := invalidPort.Start(context.Background()); err == nil {
		t.Fatal("expected invalid OSC input port to be blocked before process start")
	}
}

func TestRoutingFailureStopsExternalOSCInputProcess(t *testing.T) {
	ctx := context.Background()
	engine := &routing.Engine{}
	host := oscinputplugin.New(buildOSCPlugin(t), "127.0.0.1:0", nil, inputManifest(true), engine, "missing-session")
	if err := host.Start(ctx); err != nil {
		t.Fatal(err)
	}
	listenHost, listenPort, err := net.SplitHostPort(host.LocalAddr())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(listenPort)
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- host.Serve(ctx) }()
	if _, err := (osc.Sender{}).Send(ctx, osc.Endpoint{Host: listenHost, Port: port}, osc.Message{Address: "/will/fail", Arguments: []osc.Argument{{Type: "int32", Value: int32(1)}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveDone:
		if err == nil {
			t.Fatal("expected routing failure")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OSC input host did not return after routing failure")
	}
	if host.LocalAddr() != "" {
		t.Fatalf("plugin remained ready after routing failure: %q", host.LocalAddr())
	}
}

func inputManifest(grant bool) oscinputplugin.Manifest {
	grants := []string(nil)
	if grant {
		grants = []string{oscplugin.PermissionUDPListen}
	}
	return oscinputplugin.Manifest{
		PluginID: oscplugin.PluginID,
		InputPermissions: map[string][]string{
			oscplugin.InputOSCReceive: {oscplugin.PermissionUDPListen},
		},
		GrantedPermissions: grants,
	}
}

func buildOSCPlugin(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	binary := filepath.Join(t.TempDir(), "stagecore-osc-plugin")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/stagecore-osc-plugin")
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build OSC plugin: %v\n%s", err, output)
	}
	return binary
}

func hasCompletedRouteAction(events []contracts.EventEnvelope) bool {
	for _, event := range events {
		if event.EventType == "route.action.completed" {
			return true
		}
	}
	return false
}

func hasOSCInputSource(events []contracts.EventEnvelope) bool {
	for _, event := range events {
		if event.EventType != "input.received" {
			continue
		}
		var payload struct {
			Source string `json:"source"`
		}
		if json.Unmarshal(event.Payload, &payload) == nil && payload.Source == "OSC" {
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
