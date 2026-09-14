package cueengine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
)

func TestConfiguredDigitalTwinFaultFlowsThroughCueExecution(t *testing.T) {
	errorPolicy, err := json.Marshal(map[string]any{"on_error": "FAIL_CUE"})
	if err != nil {
		t.Fatal(err)
	}
	action := domain.Action{
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "SIM",
		CapabilityKey: "osc.send",
		Parameters:    json.RawMessage(`{}`),
		TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy:   errorPolicy,
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
		OrderIndex:    0,
	}
	f := newFixture(t, []domain.Action{action})
	physical := &physicalExecutionProbe{}
	twin := simulator.NewDigitalTwin()
	if err := twin.ConfigureFault(f.session.ID, simulator.FaultScenario{
		Capability: "osc.send",
		Behavior:   "FAIL",
		ErrorCode:  "SIMULATED_OSC_DEVICE_FAULT",
		Message:    "virtual OSC target failed",
		Uses:       1,
	}); err != nil {
		t.Fatal(err)
	}
	engine := cueengine.NewWithExecutor(f.store, simulator.NewSessionExecutorWithDigitalTwin(f.store, physical, twin))

	result := engine.ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("physical executor was reached %d time(s) from configured simulation fault", physical.calls)
	}

	cueExecutions, err := f.store.ListCueExecutions(context.Background(), f.session.ID)
	if err != nil || len(cueExecutions) != 1 {
		t.Fatalf("cue executions=%#v err=%v", cueExecutions, err)
	}
	actionExecutions, err := f.store.ListActionExecutions(context.Background(), cueExecutions[0].ID)
	if err != nil || len(actionExecutions) != 1 {
		t.Fatalf("action executions=%#v err=%v", actionExecutions, err)
	}
	if actionExecutions[0].Result != domain.ExecutionFailed || actionExecutions[0].ErrorCode == nil || *actionExecutions[0].ErrorCode != "SIMULATED_OSC_DEVICE_FAULT" {
		t.Fatalf("action execution=%#v", actionExecutions[0])
	}

	snapshot := twin.Snapshot(f.session.ID)
	if len(snapshot.Targets) != 1 || snapshot.Targets[0].LastErrorCode != "SIMULATED_OSC_DEVICE_FAULT" {
		t.Fatalf("digital twin snapshot=%#v", snapshot)
	}
	if len(snapshot.Faults) != 0 {
		t.Fatalf("one-shot fault was not consumed: %#v", snapshot.Faults)
	}
}
