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
	actionSession   domain.Session
	actionErr       error
	snapshotSession domain.Session
	snapshotErr     error
	actionCalls     int
	snapshotCalls   int
}

func (f *fakeSessionResolver) SessionForActionExecution(context.Context, string) (domain.Session, error) {
	f.actionCalls++
	return f.actionSession, f.actionErr
}

func (f *fakeSessionResolver) SessionForRuntimeSnapshotExecution(context.Context, string) (domain.Session, error) {
	f.snapshotCalls++
	return f.snapshotSession, f.snapshotErr
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

func activeSession(id string, sessionType domain.SessionType) domain.Session {
	return domain.Session{
		ID:                id,
		ProjectID:         "project-1",
		RuntimeSnapshotID: "snapshot-1",
		Type:              sessionType,
		Status:            domain.SessionActive,
	}
}

func TestSessionExecutorSimulationNeverCallsPhysicalExecutor(t *testing.T) {
	resolver := &fakeSessionResolver{actionSession: activeSession("simulation-1", domain.SessionSimulation)}
	physical := &recordingExecutor{result: capability.Result{
		Result:          domain.ExecutionCompleted,
		AckLevel:        contracts.AckDevice,
		ResponseSummary: "physical output",
	}}
	twin := NewDigitalTwin()
	executor := NewSessionExecutorWithDigitalTwin(resolver, physical, twin)
	parameters, err := json.Marshal(map[string]any{
		"simulation":     map[string]any{"behavior": "COMPLETE", "message": "digital twin completion"},
		"real_parameter": "preserved-but-not-dispatched",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "action-execution-1",
		SessionID:   "spoofed-session",
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
	if resolver.actionCalls != 1 || resolver.snapshotCalls != 0 {
		t.Fatalf("resolver action calls=%d snapshot calls=%d", resolver.actionCalls, resolver.snapshotCalls)
	}
	if len(twin.Snapshot("simulation-1").Targets) != 1 {
		t.Fatal("canonical simulation session did not receive twin state")
	}
	if len(twin.Snapshot("spoofed-session").Targets) != 0 {
		t.Fatal("caller-supplied session contaminated twin state")
	}
}

func TestSessionExecutorSimulationFailureIsDeterministic(t *testing.T) {
	resolver := &fakeSessionResolver{actionSession: activeSession("simulation-1", domain.SessionSimulation)}
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

func TestSessionExecutorRoutingFallsBackToRuntimeSnapshotAuthority(t *testing.T) {
	resolver := &fakeSessionResolver{
		actionErr:       domain.ErrNotFound,
		snapshotSession: activeSession("simulation-route", domain.SessionSimulation),
	}
	physical := &recordingExecutor{result: capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}}
	twin := NewDigitalTwin()
	executor := NewSessionExecutorWithDigitalTwin(resolver, physical, twin)

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID:       "route-execution-1",
		RuntimeSnapshotID: "snapshot-1",
		Capability:        "osc.send",
	})
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckNone {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("physical executor was called %d time(s) for simulated route output", physical.calls)
	}
	if resolver.actionCalls != 1 || resolver.snapshotCalls != 1 {
		t.Fatalf("resolver action calls=%d snapshot calls=%d", resolver.actionCalls, resolver.snapshotCalls)
	}
	if len(twin.Snapshot("simulation-route").Targets) != 1 {
		t.Fatal("route execution was not scoped to canonical simulation session")
	}
}

func TestSessionExecutorRehearsalDelegatesCanonicalSessionToPhysicalExecutor(t *testing.T) {
	resolver := &fakeSessionResolver{actionSession: activeSession("rehearsal-1", domain.SessionRehearsal)}
	physical := &recordingExecutor{result: capability.Result{
		Result:          domain.ExecutionCompleted,
		AckLevel:        contracts.AckDevice,
		ResponseSummary: "real rehearsal output",
	}}
	executor := NewSessionExecutor(resolver, physical)
	req := capability.Request{ExecutionID: "action-execution-3", SessionID: "spoofed", Capability: "osc.send"}

	result := executor.Execute(context.Background(), req)
	if result.ResponseSummary != "real rehearsal output" || result.AckLevel != contracts.AckDevice {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 1 || physical.last.ExecutionID != req.ExecutionID || physical.last.SessionID != "rehearsal-1" {
		t.Fatalf("physical calls=%d last=%#v", physical.calls, physical.last)
	}
	if physical.last.ProjectID != "project-1" || physical.last.RuntimeSnapshotID != "snapshot-1" {
		t.Fatalf("canonical physical authority=%#v", physical.last)
	}
}

func TestSessionExecutorShowDelegatesToPhysicalExecutor(t *testing.T) {
	resolver := &fakeSessionResolver{actionSession: activeSession("show-1", domain.SessionShow)}
	physical := &recordingExecutor{result: capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}}
	executor := NewSessionExecutor(resolver, physical)

	result := executor.Execute(context.Background(), capability.Request{ExecutionID: "action-execution-4", Capability: "http.request"})
	if result.Result != domain.ExecutionCompleted || physical.calls != 1 {
		t.Fatalf("result=%#v physical calls=%d", result, physical.calls)
	}
}

func TestSessionExecutorFailsClosedWhenSessionModeCannotBeResolved(t *testing.T) {
	resolver := &fakeSessionResolver{actionErr: errors.New("database unavailable")}
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

func TestSessionExecutorRejectsInactiveCanonicalSession(t *testing.T) {
	session := activeSession("simulation-1", domain.SessionSimulation)
	session.Status = domain.SessionCompleted
	resolver := &fakeSessionResolver{actionSession: session}
	physical := &recordingExecutor{}
	executor := NewSessionExecutor(resolver, physical)

	result := executor.Execute(context.Background(), capability.Request{ExecutionID: "action-execution-6", Capability: "osc.send"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SESSION_NOT_ACTIVE" {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("inactive session reached physical executor %d time(s)", physical.calls)
	}
}

func TestSessionExecutorRequiresPersistedExecutionAuthority(t *testing.T) {
	resolver := &fakeSessionResolver{actionSession: activeSession("show-1", domain.SessionShow)}
	physical := &recordingExecutor{}
	executor := NewSessionExecutor(resolver, physical)

	result := executor.Execute(context.Background(), capability.Request{Capability: "osc.send"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "ACTION_EXECUTION_ID_REQUIRED" {
		t.Fatalf("result=%#v", result)
	}
	if resolver.actionCalls != 0 || resolver.snapshotCalls != 0 || physical.calls != 0 {
		t.Fatalf("resolver action calls=%d snapshot calls=%d physical calls=%d", resolver.actionCalls, resolver.snapshotCalls, physical.calls)
	}
}
