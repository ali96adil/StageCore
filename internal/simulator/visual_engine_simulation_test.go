package simulator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

func TestSimulatorAdapterValidatesVisualEngineContract(t *testing.T) {
	adapter := Adapter{}

	valid := adapter.Execute(context.Background(), capability.Request{
		Capability: visualengine.CapabilityBlackout,
		Parameters: []byte(`{"contract_version":1,"enabled":true}`),
	})
	if valid.Result != domain.ExecutionCompleted || valid.ErrorCode != "" {
		t.Fatalf("valid visual result=%#v", valid)
	}
	if valid.ResponseSummary != "simulated visual command" {
		t.Fatalf("valid visual summary=%q", valid.ResponseSummary)
	}

	invalid := adapter.Execute(context.Background(), capability.Request{
		Capability: visualengine.CapabilityBlackout,
		Parameters: []byte(`{"contract_version":1}`),
	})
	if invalid.Result != domain.ExecutionFailed || invalid.ErrorCode != "SIM_VISUAL_COMMAND_INVALID" {
		t.Fatalf("invalid visual result=%#v", invalid)
	}
	if !strings.Contains(invalid.ResponseSummary, "enabled is required") {
		t.Fatalf("invalid visual summary=%q", invalid.ResponseSummary)
	}
}

func TestVisualCommandRunsOnlyInsideDigitalTwinAndRecordsEvidence(t *testing.T) {
	stateStore := &recordingSimulationStore{session: observableSimulationSession()}
	resolver := &observableResolver{recordingSimulationStore: stateStore}
	physical := &physicalProbe{}
	twin := NewDigitalTwin()
	executor := NewSessionExecutorWithDigitalTwin(resolver, physical, twin)

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "visual-action-execution-1",
		Capability:  visualengine.CapabilityBlackout,
		Target:      &capability.Target{Ref: "visual.main", LogicalType: "machine_role"},
		Parameters:  []byte(`{"contract_version":1,"enabled":true}`),
	})
	if result.Result != domain.ExecutionCompleted {
		t.Fatalf("visual simulation result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("physical renderer calls=%d", physical.calls)
	}
	if len(stateStore.events) != 1 || stateStore.events[0].EventType != "simulation.execution.completed" {
		t.Fatalf("events=%#v", stateStore.events)
	}
	if stateStore.events[0].Source != "stagecore.simulator.digital_twin" {
		t.Fatalf("event source=%q", stateStore.events[0].Source)
	}

	snapshot := twin.Snapshot(stateStore.session.ID)
	if len(snapshot.Targets) != 1 {
		t.Fatalf("targets=%#v", snapshot.Targets)
	}
	if snapshot.Targets[0].LastCapability != visualengine.CapabilityBlackout || snapshot.Targets[0].LastResult != domain.ExecutionCompleted {
		t.Fatalf("visual target state=%#v", snapshot.Targets[0])
	}
}

func TestInvalidVisualCommandFailsInsideDigitalTwinWithoutPhysicalRenderer(t *testing.T) {
	stateStore := &recordingSimulationStore{session: observableSimulationSession()}
	resolver := &observableResolver{recordingSimulationStore: stateStore}
	physical := &physicalProbe{}
	twin := NewDigitalTwin()
	executor := NewSessionExecutorWithDigitalTwin(resolver, physical, twin)

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "visual-action-execution-invalid",
		Capability:  visualengine.CapabilityBlackout,
		Target:      &capability.Target{Ref: "visual.main", LogicalType: "machine_role"},
		Parameters:  []byte(`{"contract_version":1}`),
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SIM_VISUAL_COMMAND_INVALID" {
		t.Fatalf("invalid visual simulation result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("physical renderer calls=%d", physical.calls)
	}
	if len(stateStore.events) != 1 || stateStore.events[0].EventType != "simulation.execution.failed" {
		t.Fatalf("events=%#v", stateStore.events)
	}
}

func TestVisualDigitalTwinFaultScenarios(t *testing.T) {
	tests := []struct {
		name       string
		capability string
		parameters []byte
		behavior   string
		errorCode  string
		wantResult domain.ExecutionResult
		wantCode   string
		timeout    bool
	}{
		{
			name:       "renderer unavailable",
			capability: visualengine.CapabilityBlackout,
			parameters: []byte(`{"contract_version":1,"enabled":true}`),
			behavior:   "UNAVAILABLE",
			errorCode:  "VISUAL_RENDERER_UNAVAILABLE",
			wantResult: domain.ExecutionFailed,
			wantCode:   "VISUAL_RENDERER_UNAVAILABLE",
		},
		{
			name:       "source unavailable",
			capability: visualengine.CapabilityPlay,
			parameters: []byte(`{"contract_version":1,"layer_id":"camera.main"}`),
			behavior:   "UNAVAILABLE",
			errorCode:  "VISUAL_SOURCE_UNAVAILABLE",
			wantResult: domain.ExecutionFailed,
			wantCode:   "VISUAL_SOURCE_UNAVAILABLE",
		},
		{
			name:       "managed media missing",
			capability: visualengine.CapabilityPreload,
			parameters: []byte(`{"contract_version":1,"layer_id":"hero","content_version_id":"content-v1","content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
			behavior:   "FAIL",
			errorCode:  "VISUAL_MEDIA_MISSING",
			wantResult: domain.ExecutionFailed,
			wantCode:   "VISUAL_MEDIA_MISSING",
		},
		{
			name:       "renderer timeout",
			capability: visualengine.CapabilityPlay,
			parameters: []byte(`{"contract_version":1,"layer_id":"hero"}`),
			behavior:   "TIMEOUT",
			wantResult: domain.ExecutionTimedOut,
			wantCode:   "TIMEOUT",
			timeout:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateStore := &recordingSimulationStore{session: observableSimulationSession()}
			resolver := &observableResolver{recordingSimulationStore: stateStore}
			physical := &physicalProbe{}
			twin := NewDigitalTwin()
			executor := NewSessionExecutorWithDigitalTwin(resolver, physical, twin)

			if err := twin.ConfigureFault(stateStore.session.ID, FaultScenario{
				TargetRef:  "visual.main",
				Capability: test.capability,
				Behavior:   test.behavior,
				ErrorCode:  test.errorCode,
				Uses:       1,
			}); err != nil {
				t.Fatal(err)
			}

			var ctx context.Context = context.Background()
			cancel := func() {}
			if test.timeout {
				ctx, cancel = context.WithTimeout(ctx, 5*time.Millisecond)
			}
			defer cancel()

			result := executor.Execute(ctx, capability.Request{
				ExecutionID: "visual-fault-execution",
				Capability:  test.capability,
				Target:      &capability.Target{Ref: "visual.main", LogicalType: "machine_role"},
				Parameters:  test.parameters,
			})
			if result.Result != test.wantResult || result.ErrorCode != test.wantCode {
				t.Fatalf("fault result=%#v", result)
			}
			if physical.calls != 0 {
				t.Fatalf("physical renderer calls=%d", physical.calls)
			}
			if len(stateStore.events) != 3 {
				t.Fatalf("events=%#v", stateStore.events)
			}
			if stateStore.events[2].EventType != map[domain.ExecutionResult]string{
				domain.ExecutionFailed:   "simulation.execution.failed",
				domain.ExecutionTimedOut: "simulation.execution.timed_out",
			}[test.wantResult] {
				t.Fatalf("execution event=%#v", stateStore.events[2])
			}
		})
	}
}
