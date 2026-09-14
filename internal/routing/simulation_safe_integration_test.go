package routing_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/routing"
)

func TestSimulationSafeRoutingContainsDirectOutput(t *testing.T) {
	f := newOSCOutputRouteFixture(t, 53000, json.RawMessage(`{"operator":"equals","value":1}`), true, nil)
	if _, err := f.h.DB.Exec(`UPDATE sessions SET session_type = 'SIMULATION' WHERE session_id = ?`, f.session.ID); err != nil {
		t.Fatal(err)
	}

	host := pluginhost.New("/unused/simulation-must-not-start-osc", nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}

	result := routing.NewSimulationSafe(f.store, registry).InjectTest(
		context.Background(),
		f.session.ID,
		injectCommand(t, f, json.RawMessage(`1`)),
	)
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}
	if host.Ready() != nil {
		t.Fatal("SIMULATION route started the physical OSC plugin")
	}

	events, err := f.store.ListEvents(context.Background(), f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRouteEventTypes(t, events, []string{"input.received", "route.triggered", "simulation.execution.completed", "route.action.completed"})

	var simulationPayload struct {
		Scope    string `json:"scope"`
		AckLevel string `json:"ack_level"`
		Result   string `json:"result"`
	}
	if err := json.Unmarshal(events[2].Payload, &simulationPayload); err != nil {
		t.Fatal(err)
	}
	if simulationPayload.Scope != "SIMULATION_ONLY" || simulationPayload.AckLevel != string(contracts.AckNone) || simulationPayload.Result != string(domain.ExecutionCompleted) {
		t.Fatalf("simulation execution trace=%#v", simulationPayload)
	}

	var actionPayload struct {
		AckLevel string `json:"ack_level"`
		Result   string `json:"result"`
	}
	if err := json.Unmarshal(events[3].Payload, &actionPayload); err != nil {
		t.Fatal(err)
	}
	if actionPayload.AckLevel != string(contracts.AckNone) || actionPayload.Result != string(domain.ExecutionCompleted) {
		t.Fatalf("simulation route action trace=%#v", actionPayload)
	}
}
