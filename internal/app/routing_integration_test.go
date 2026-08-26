package app_test

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/app"
	"github.com/ali96adil/StageCore/internal/config"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOpenWiresRoutingEngineToProductOSCCapability(t *testing.T) {
	ctx := context.Background()
	receiver, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	root := t.TempDir()
	application, err := app.Open(ctx, config.Config{DataRoot: root, VaultRoot: filepath.Join(root, "vault"), Listen: "127.0.0.1:0", OSCPluginPath: buildProductOSCPlugin(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	if application.RoutingEngine == nil {
		t.Fatal("Hub composition did not initialize RoutingEngine")
	}

	project, revision, err := application.Store.CreateProject(ctx, store.CreateProjectParams{Name: "App Routing Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	aliasConfig, _ := json.Marshal(map[string]any{"osc": map[string]any{"host": "127.0.0.1", "port": receiver.LocalAddr().(*net.UDPAddr).Port}})
	if _, err := application.Store.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "VIDEO-MAIN", LogicalType: "OSC_ENDPOINT", ProjectConfig: aliasConfig}); err != nil {
		t.Fatal(err)
	}
	input, err := application.Store.CreateInput(ctx, domain.InputDefinition{RevisionID: revision.ID, Name: "APP-GO", SourceRef: "manual:test", EventType: "test.value", ValueSchema: json.RawMessage(`{"type":"boolean"}`), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := application.Store.CreateOutput(ctx, domain.OutputDefinition{RevisionID: revision.ID, Name: "APP-OSC", TargetRef: "VIDEO-MAIN", CapabilityKey: oscplugin.CapabilityOSCSend, ValueSchema: json.RawMessage(`{"type":"object"}`), Criticality: "NORMAL"})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	if _, err := application.Store.CreateRouteWithActions(ctx, domain.Route{RevisionID: revision.ID, Name: "APP-GO -> OSC", InputID: input.ID, ConditionDefinition: json.RawMessage(`{"operator":"boolean_is","value":true}`), PriorityClass: domain.PriorityP2, ErrorPolicy: json.RawMessage(`{"on_error":"STOP_ROUTE"}`), Enabled: true}, []domain.RouteAction{{OrderIndex: 0, OutputID: &outputID, Parameters: json.RawMessage(`{"address":"/app/route","arguments":[{"type":"string","value":"go"}]}`)}}); err != nil {
		t.Fatal(err)
	}
	if err := application.Store.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(application.Store).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := application.Store.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "App M3 rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(routing.InjectTestPayload{InputID: input.ID, Value: json.RawMessage(`true`)})
	result := application.RoutingEngine.InjectTest(ctx, session.ID, contracts.CommandEnvelope{CommandID: commandID, CommandType: routing.InputInjectTestCommandType, SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(), ProjectID: runtimeSnapshot.ProjectID, RuntimeSnapshotID: runtimeSnapshot.ID, Issuer: "test.operator", Priority: "P2", Payload: payload})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}
	packet := make([]byte, 2048)
	_ = receiver.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := receiver.ReadFromUDP(packet)
	if err != nil {
		t.Fatal(err)
	}
	message, err := osc.DecodeMessage(packet[:n])
	if err != nil {
		t.Fatal(err)
	}
	if message.Address != "/app/route" || len(message.Arguments) != 1 || message.Arguments[0].Value.(string) != "go" {
		t.Fatalf("message=%#v", message)
	}
}
