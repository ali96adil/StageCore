package cueengine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
)

type physicalExecutionProbe struct {
	calls int
}

func (p *physicalExecutionProbe) Execute(context.Context, capability.Request) capability.Result {
	p.calls++
	return capability.Result{
		Result:          domain.ExecutionCompleted,
		AckLevel:        contracts.AckDevice,
		ResponseSummary: "physical executor reached",
	}
}

func TestSimulationSessionContainsRealCapabilityInsideDigitalTwinBoundary(t *testing.T) {
	action := domain.Action{
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "SIM",
		CapabilityKey: "osc.send",
		Parameters:    json.RawMessage(`{}`),
		TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy:   json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
		OrderIndex:    0,
	}
	// Keep the policy fixture valid JSON. It is written this way instead of
	// inheriting a simulator-only action helper because this action deliberately
	// carries a real capability identity.
	action.ErrorPolicy = json.RawMessage(`{"on_error":"FAIL_CUE"}`)
	f := newFixture(t, []domain.Action{action})
	physical := &physicalExecutionProbe{}
	engine := cueengine.NewWithExecutor(f.store, simulator.NewSessionExecutor(f.store, physical))

	result := engine.ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("physical executor was reached %d time(s) from SIMULATION", physical.calls)
	}

	cueExecutions, err := f.store.ListCueExecutions(context.Background(), f.session.ID)
	if err != nil || len(cueExecutions) != 1 {
		t.Fatalf("cue executions=%#v err=%v", cueExecutions, err)
	}
	actionExecutions, err := f.store.ListActionExecutions(context.Background(), cueExecutions[0].ID)
	if err != nil || len(actionExecutions) != 1 {
		t.Fatalf("action executions=%#v err=%v", actionExecutions, err)
	}
	if actionExecutions[0].Result != domain.ExecutionCompleted || actionExecutions[0].ResponseSummary != "simulated completion" {
		t.Fatalf("action execution=%#v", actionExecutions[0])
	}
}
