package routing_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/simulator"
)

func TestConfiguredDigitalTwinFaultFlowsThroughDirectRouteOutput(t *testing.T) {
	f := newOSCOutputRouteFixture(t, 53000, json.RawMessage(`{"operator":"equals","value":1}`), true, nil)
	if _, err := f.h.DB.Exec(`UPDATE sessions SET session_type = 'SIMULATION' WHERE session_id = ?`, f.session.ID); err != nil {
		t.Fatal(err)
	}

	host := pluginhost.New("/unused/configured-simulation-must-not-start-osc", nil, nil, nil, oscManifest([]string{oscplugin.PermissionUDPSend}))
	defer host.Close()
	registry := capability.NewRegistry()
	if err := registry.Register(oscplugin.CapabilityOSCSend, oscplugin.New(host)); err != nil {
		t.Fatal(err)
	}
	twin := simulator.NewDigitalTwin()
	if err := twin.ConfigureFault(f.session.ID, simulator.FaultScenario{
		TargetRef:  "VIDEO-MAIN",
		Capability: oscplugin.CapabilityOSCSend,
		Behavior:   "REJECT",
		ErrorCode:  "SIMULATED_ROUTE_REJECTED",
		Uses:       1,
	}); err != nil {
		t.Fatal(err)
	}

	result := routing.NewSimulationSafeWithDigitalTwin(f.store, registry, twin).InjectTest(
		context.Background(),
		f.session.ID,
		injectCommand(t, f, json.RawMessage(`1`)),
	)
	if result.Status != contracts.CommandFailed {
		t.Fatalf("result=%#v", result)
	}
	if host.Ready() != nil {
		t.Fatal("configured SIMULATION route started physical OSC plugin")
	}

	snapshot := twin.Snapshot(f.session.ID)
	if len(snapshot.Targets) != 1 || snapshot.Targets[0].LastErrorCode != "SIMULATED_ROUTE_REJECTED" {
		t.Fatalf("digital twin snapshot=%#v", snapshot)
	}
	if len(snapshot.Faults) != 0 {
		t.Fatalf("one-shot route fault was not consumed: %#v", snapshot.Faults)
	}
}
