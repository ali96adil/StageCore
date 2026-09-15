package simulator

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestSessionExecutorSimulationBypassesPhysicalDispatchAuthority(t *testing.T) {
	var authorityCalls atomic.Int32
	var physicalCalls atomic.Int32
	physical := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		physicalCalls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted}
	})
	gate := dispatchauthority.New(physical, dispatchauthority.SourceFunc(func(context.Context) (dispatchauthority.Snapshot, error) {
		authorityCalls.Add(1)
		return dispatchauthority.Snapshot{Mode: dispatchauthority.ModeStandby}, nil
	}))
	resolver := &fakeSessionResolver{actionSession: activeSession("simulation-authority", domain.SessionSimulation)}
	twin := NewDigitalTwin()
	executor := NewSessionExecutorWithDigitalTwin(resolver, gate, twin)
	parameters, err := json.Marshal(map[string]any{
		"simulation": map[string]any{"behavior": "COMPLETE", "message": "virtual only"},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "simulation-exec",
		Capability:  "osc.send",
		Parameters:  parameters,
		Target:      &capability.Target{Ref: "virtual-light", LogicalType: "osc"},
	})
	if result.Result != domain.ExecutionCompleted {
		t.Fatalf("result=%#v", result)
	}
	if authorityCalls.Load() != 0 {
		t.Fatalf("simulation consulted physical authority %d time(s), want 0", authorityCalls.Load())
	}
	if physicalCalls.Load() != 0 {
		t.Fatalf("simulation reached physical executor %d time(s), want 0", physicalCalls.Load())
	}
}

func TestSessionExecutorOperationalModesHonorPhysicalDispatchAuthority(t *testing.T) {
	for _, sessionType := range []domain.SessionType{domain.SessionRehearsal, domain.SessionShow} {
		t.Run(string(sessionType), func(t *testing.T) {
			var authorityCalls atomic.Int32
			var physicalCalls atomic.Int32
			physical := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
				physicalCalls.Add(1)
				return capability.Result{Result: domain.ExecutionCompleted}
			})
			gate := dispatchauthority.New(physical, dispatchauthority.SourceFunc(func(context.Context) (dispatchauthority.Snapshot, error) {
				authorityCalls.Add(1)
				return dispatchauthority.Snapshot{Mode: dispatchauthority.ModeStandby, HolderID: "hub-leader", Epoch: 9}, nil
			}))
			resolver := &fakeSessionResolver{actionSession: activeSession("operational-authority", sessionType)}
			executor := NewSessionExecutor(resolver, gate)

			result := executor.Execute(context.Background(), capability.Request{
				ExecutionID: "operational-exec",
				Capability:  "osc.send",
			})
			if result.Result != domain.ExecutionFailed || result.ErrorCode != "PHYSICAL_DISPATCH_NOT_LEADER" {
				t.Fatalf("result=%#v", result)
			}
			if authorityCalls.Load() != 1 {
				t.Fatalf("authority calls=%d, want 1", authorityCalls.Load())
			}
			if physicalCalls.Load() != 0 {
				t.Fatalf("standby reached physical executor %d time(s), want 0", physicalCalls.Load())
			}
		})
	}
}
