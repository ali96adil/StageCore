package routing_test

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type routeFixture struct {
	h        *db.Handle
	store    *store.Store
	snapshot domain.RuntimeSnapshot
	session  domain.Session
	input    domain.InputDefinition
	route    domain.Route
}

func TestInjectTestRoutesToRealOSCAndPersistsTruthfulTrace(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	f := newOSCOutputRouteFixture(t, conn.LocalAddr().(*net.UDPAddr).Port, json.RawMessage(`{"operator":"equals","value":1}`), true, nil)

	pluginPath := buildRoutingOSCPlugin(t)
	host := pluginhost.New(pluginPath, nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}
	engine := routing.New(f.store, registry)
	command := injectCommand(t, f, json.RawMessage(`1`))
	result := engine.InjectTest(context.Background(), f.session.ID, command)
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}

	packet := make([]byte, 2048)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := conn.ReadFromUDP(packet)
	if err != nil {
		t.Fatal(err)
	}
	message, err := osc.DecodeMessage(packet[:n])
	if err != nil {
		t.Fatal(err)
	}
	if message.Address != "/route/go" || len(message.Arguments) != 1 || message.Arguments[0].Value.(int32) != 7 {
		t.Fatalf("message=%#v", message)
	}

	events, err := f.store.ListEvents(context.Background(), f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRouteEventTypes(t, events, []string{"input.received", "route.triggered", "route.action.completed"})
	var actionPayload struct {
		AckLevel string `json:"ack_level"`
		Result   string `json:"result"`
	}
	if err := json.Unmarshal(events[2].Payload, &actionPayload); err != nil {
		t.Fatal(err)
	}
	if actionPayload.AckLevel != string(contracts.AckTransportOnly) || actionPayload.Result != string(domain.ExecutionCompleted) {
		t.Fatalf("action trace=%#v", actionPayload)
	}
}

func TestNonMatchingAndDisabledRoutesDispatchNothingButTrace(t *testing.T) {
	for _, tc := range []struct {
		name        string
		condition   json.RawMessage
		enabled     bool
		wantDisposition string
	}{
		{name: "non-matching", condition: json.RawMessage(`{"operator":"equals","value":2}`), enabled: true, wantDisposition: "NOT_MATCHED"},
		{name: "disabled", condition: json.RawMessage(`{"operator":"equals","value":1}`), enabled: false, wantDisposition: "DISABLED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOSCOutputRouteFixture(t, 53000, tc.condition, tc.enabled, nil)
			host := pluginhost.New("/unused/no-dispatch", nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
			defer host.Close()
			registry := capability.NewRegistry()
			if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
				t.Fatal(err)
			}
			result := routing.New(f.store, registry).InjectTest(context.Background(), f.session.ID, injectCommand(t, f, json.RawMessage(`1`)))
			if result.Status != contracts.CommandCompleted {
				t.Fatalf("result=%#v", result)
			}
			if host.Ready() != nil {
				t.Fatal("route that does not dispatch must not start plugin")
			}
			events, _ := f.store.ListEvents(context.Background(), f.session.ID)
			assertRouteEventTypes(t, events, []string{"input.received", "route.evaluated"})
			var payload struct{ Disposition string `json:"disposition"` }
			if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Disposition != tc.wantDisposition {
				t.Fatalf("disposition=%q want %q", payload.Disposition, tc.wantDisposition)
			}
		})
	}
}

func TestDebounceAndDuplicateCommandDoNotReplayOSC(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	debounce := int64(250)
	f := newOSCOutputRouteFixture(t, conn.LocalAddr().(*net.UDPAddr).Port, nil, true, &debounce)
	pluginPath := buildRoutingOSCPlugin(t)
	host := pluginhost.New(pluginPath, nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 8, 26, 14, 0, 0, 0, time.UTC)
	engine := routing.NewWithNow(f.store, registry, func() time.Time { return current })

	firstCommand := injectCommand(t, f, json.RawMessage(`1`))
	first := engine.InjectTest(context.Background(), f.session.ID, firstCommand)
	if first.Status != contracts.CommandCompleted {
		t.Fatalf("first=%#v", first)
	}
	readOSC(t, conn)

	// Repeating the exact same command is idempotent and cannot replay output.
	duplicate := engine.InjectTest(context.Background(), f.session.ID, firstCommand)
	if duplicate.Status != contracts.CommandCompleted {
		t.Fatalf("duplicate=%#v", duplicate)
	}
	assertNoOSC(t, conn)

	current = current.Add(100 * time.Millisecond)
	inside := engine.InjectTest(context.Background(), f.session.ID, injectCommand(t, f, json.RawMessage(`1`)))
	if inside.Status != contracts.CommandCompleted {
		t.Fatalf("inside=%#v", inside)
	}
	assertNoOSC(t, conn)

	current = current.Add(150 * time.Millisecond)
	boundary := engine.InjectTest(context.Background(), f.session.ID, injectCommand(t, f, json.RawMessage(`1`)))
	if boundary.Status != contracts.CommandCompleted {
		t.Fatalf("boundary=%#v", boundary)
	}
	readOSC(t, conn)

	events, _ := f.store.ListEvents(context.Background(), f.session.ID)
	var debounced bool
	for _, event := range events {
		if event.EventType != "route.evaluated" {
			continue
		}
		var payload struct{ Disposition string `json:"disposition"` }
		_ = json.Unmarshal(event.Payload, &payload)
		if payload.Disposition == "DEBOUNCED" {
			debounced = true
		}
	}
	if !debounced {
		t.Fatal("missing debounced route trace")
	}
}

func newOSCOutputRouteFixture(t *testing.T, port int, condition json.RawMessage, enabled bool, debounce *int64) routeFixture {
	t.Helper()
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "M3 Routing Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	aliasConfig, _ := json.Marshal(map[string]any{"osc": map[string]any{"host": "127.0.0.1", "port": port}})
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "VIDEO-MAIN", LogicalType: "OSC_ENDPOINT", ProjectConfig: aliasConfig}); err != nil {
		t.Fatal(err)
	}
	input, err := s.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "TEST-GO", SourceRef: "manual:test", EventType: "test.value", ValueSchema: json.RawMessage(`{"type":"number"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := s.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "VIDEO-GO", TargetRef: "VIDEO-MAIN", CapabilityKey: oscplugin.CapabilityOSCSend, ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "NORMAL"})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	params := json.RawMessage(`{"address":"/route/go","arguments":[{"type":"int32","value":7}]}`)
	route, err := s.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "TEST-GO -> VIDEO-GO", InputID: input.ID, ConditionDefinition: condition, TransformDefinition: json.RawMessage(`null`), DebounceMS: debounce, PriorityClass: domain.PriorityP2, ErrorPolicy: json.RawMessage(`{"on_error":"STOP_ROUTE"}`), Enabled: enabled}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputID, Parameters: params}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "M3 routing rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return routeFixture{h: h, store: s, snapshot: runtimeSnapshot, session: session, input: input, route: route}
}

func injectCommand(t *testing.T, f routeFixture, value json.RawMessage) contracts.CommandEnvelope {
	t.Helper()
	id, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(routing.InjectTestPayload{InputID: f.input.ID, Value: value})
	return contracts.CommandEnvelope{CommandID: id, CommandType: routing.InputInjectTestCommandType, SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(), ProjectID: f.snapshot.ProjectID, RuntimeSnapshotID: f.snapshot.ID, Issuer: "test.operator", Priority: "P2", Payload: payload}
}

func oscManifest(grants []string) pluginhost.Manifest {
	return pluginhost.Manifest{PluginID: oscplugin.PluginID, CapabilityPermissions: map[string][]string{oscplugin.CapabilityOSCSend: {oscplugin.PermissionUDPSend}}, GrantedPermissions: grants}
}

func buildRoutingOSCPlugin(t *testing.T) string {
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

func readOSC(t *testing.T, conn *net.UDPConn) {
	t.Helper()
	packet := make([]byte, 2048)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadFromUDP(packet); err != nil {
		t.Fatal(err)
	}
}

func assertNoOSC(t *testing.T, conn *net.UDPConn) {
	t.Helper()
	packet := make([]byte, 2048)
	_ = conn.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	if _, _, err := conn.ReadFromUDP(packet); err == nil {
		t.Fatal("unexpected routed OSC datagram")
	}
}

func assertRouteEventTypes(t *testing.T, events []contracts.EventEnvelope, want []string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count=%d want=%d types=%v", len(events), len(want), routeEventTypes(events))
	}
	for i := range want {
		if events[i].EventType != want[i] {
			t.Fatalf("events=%v want=%v", routeEventTypes(events), want)
		}
	}
}

func routeEventTypes(events []contracts.EventEnvelope) []string {
	out := make([]string, len(events))
	for i := range events {
		out[i] = events[i].EventType
	}
	return out
}
