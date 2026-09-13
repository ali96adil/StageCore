package simulator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type fakeSessionResolver struct {
	sessionType domain.SessionType
	err         error
	calls       int
}

func (f *fakeSessionResolver) SessionTypeForActionExecution(context.Context, string) (domain.SessionType, error) {
	f.calls++
	return f.sessionType, f.err
}

type recordingExecutor struct {
	calls  int
	last   capability.Request
	result capability.Result
}

func (e *recordingExecutor) Execute(_ context.Context, req capability.Request) capability.Result {
	e.calls++
	e.last = req
	return e.result
}

func TestSessionExecutorSimulationNeverCallsPhysicalExecutor(t *testing.T) {
	resolver := &fakeSessionResolver{sessionType: domain.SessionSimulation}
	physical := &recordingExecutor{result: capability.Result{
		Result:          domain.ExecutionCompleted,
		AckLevel:        contracts.AckDevice,
		ResponseSummary: "physical output",
	}}
	executor := NewSessionExecutor(resolver, physical)
	parameters, err := json.Marshal(map[string]any{
		"simulation":     map[string]any{"behavior": "COMPLETE", "message": "digital twin completion"},
		"real_parameter": "preserved-but-not-dispatched",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "action-execution-1",
		Capability:  "osc.send",
		Parameters:  parameters,
		Target:      &capability.Target{Ref: "lighting.front", LogicalType: "osc"},
	})

	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckNone {
		t.Fatalf("simulation result=%#v", result)
	}
	if result.ResponseSummary != "digital twin completion" {
		t.Fatalf("simulation summary=%q", result.ResponseSummary)
	}
	if physical.calls != 0 {
		t.Fatalf("physical executor was called %d time(s) during SIMULATION", physical.calls)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls=%d want=1", resolver.calls)
	}
}

func TestSessionExecutorSimulationFailureIsDeterministic(t *testing.T) {
	resolver := &fakeSessionResolver{sessionType: domain.SessionSimulation}
	physical := &recordingExecutor{}
	executor := NewSessionExecutor(resolver, physical)
	parameters, err := json.Marshal(map[string]any{
		"simulation": map[string]any{
			"behavior":   "FAIL",
			"error_code": "VIRTUAL_DEVICE_OFFLINE",
			"message":    "virtual target offline",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "action-execution-2",
		Capability:  "tablet.media.play",
		Parameters:  parameters,
	})

	if result.Result != domain.ExecutionFailed || result.ErrorCode != "VIRTUAL_DEVICE_OFFLINE" {
		t.Fatalf("simulation result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("physical executor was called %d time(s) during simulated failure", physical.calls)
	}
}

func TestSessionExecutorRehearsalDelegatesToPhysicalExecutor(t *testing.T) {
	resolver := &fakeSessionResolver{sessionType: domain.SessionRehearsal}
	physical := &recordingExecutor{result: capability.Result{
		Result:          domain.ExecutionCompleted,
		AckLevel:        contracts.AckDevice,
		ResponseSummary: "real rehearsal output",
	}}
	executor := NewSessionExecutor(resolver, physical)
	req := capability.Request{ExecutionID: "action-execution-3", Capability: "osc.send"}

	result := executor.Execute(context.Background(), req)
	if result.ResponseSummary != "real rehearsal output" || result.AckLevel != contracts.AckDevice {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 1 || physical.last.ExecutionID != req.ExecutionID {
		t.Fatalf("physical calls=%d last=%#v", physical.calls, physical.last)
	}
}

func TestSessionExecutorShowDelegatesToPhysicalExecutor(t *testing.T) {
	resolver := &fakeSessionResolver{sessionType: domain.SessionShow}
	physical := &recordingExecutor{result: capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}}
	executor := NewSessionExecutor(resolver, physical)

	result := executor.Execute(context.Background(), capability.Request{ExecutionID: "action-execution-4", Capability: "http.request"})
	if result.Result != domain.ExecutionCompleted || physical.calls != 1 {
		t.Fatalf("result=%#v physical calls=%d", result, physical.calls)
	}
}

func TestSessionExecutorFailsClosedWhenSessionModeCannotBeResolved(t *testing.T) {
	resolver := &fakeSessionResolver{err: errors.New("database unavailable")}
	physical := &recordingExecutor{result: capability.Result{Result: domain.ExecutionCompleted}}
	executor := NewSessionExecutor(resolver, physical)

	result := executor.Execute(context.Background(), capability.Request{ExecutionID: "action-execution-5", Capability: "script.run"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SESSION_MODE_UNAVAILABLE" {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("fail-closed gate delegated to physical executor %d time(s)", physical.calls)
	}
}

func TestSessionExecutorRequiresPersistedActionIdentity(t *testing.T) {
	resolver := &fakeSessionResolver{sessionType: domain.SessionShow}
	physical := &recordingExecutor{}
	executor := NewSessionExecutor(resolver, physical)

	result := executor.Execute(context.Background(), capability.Request{Capability: "osc.send"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "ACTION_EXECUTION_ID_REQUIRED" {
		t.Fatalf("result=%#v", result)
	}
	if resolver.calls != 0 || physical.calls != 0 {
		t.Fatalf("resolver calls=%d physical calls=%d", resolver.calls, physical.calls)
	}
}
