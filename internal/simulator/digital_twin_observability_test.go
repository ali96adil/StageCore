package simulator

import (
	"context"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type recordingSimulationStore struct {
	session domain.Session
	events  []contracts.EventEnvelope
}

func (s *recordingSimulationStore) GetSessionFoundation(_ context.Context, sessionID string) (domain.Session, error) {
	if s.session.ID != sessionID {
		return domain.Session{}, domain.ErrNotFound
	}
	return s.session, nil
}

func (s *recordingSimulationStore) AppendEvent(_ context.Context, _ *string, event contracts.EventEnvelope) (contracts.EventEnvelope, error) {
	event.Sequence = int64(len(s.events) + 1)
	s.events = append(s.events, event)
	return event, nil
}

type observableResolver struct {
	*recordingSimulationStore
}

func (r *observableResolver) SessionForActionExecution(context.Context, string) (domain.Session, error) {
	return r.session, nil
}

func (r *observableResolver) SessionForRuntimeSnapshotExecution(context.Context, string) (domain.Session, error) {
	return r.session, nil
}

type physicalProbe struct {
	calls int
}

func (p *physicalProbe) Execute(context.Context, capability.Request) capability.Result {
	p.calls++
	return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
}

func observableSimulationSession() domain.Session {
	return domain.Session{
		ID:                "simulation-observed",
		ProjectID:         "project-1",
		RuntimeSnapshotID: "snapshot-1",
		Type:              domain.SessionSimulation,
		Status:            domain.SessionActive,
		LifecycleState:    domain.SessionLifecycleActive,
	}
}

func TestDigitalTwinStateTruthIsVersionedAndSnapshotBound(t *testing.T) {
	twin := NewDigitalTwin()
	session := observableSimulationSession()
	if err := twin.BindSession(session); err != nil {
		t.Fatal(err)
	}
	if err := twin.SetTargetOnline(session.ID, "tablet.3", false); err != nil {
		t.Fatal(err)
	}

	snapshot := twin.Snapshot(session.ID)
	if snapshot.Version != DigitalTwinStateContractVersion1 || snapshot.RuntimeSnapshotID != session.RuntimeSnapshotID {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if len(snapshot.Targets) != 1 {
		t.Fatalf("targets=%#v", snapshot.Targets)
	}
	truth := snapshot.Targets[0].StateTruth
	if truth.Version != DigitalTwinStateContractVersion1 || truth.Scope != "SIMULATION_ONLY" {
		t.Fatalf("state truth=%#v", truth)
	}
	if truth.DesiredOnline == nil || *truth.DesiredOnline || truth.ObservedOnline == nil || *truth.ObservedOnline || truth.VerifiedOnline == nil || *truth.VerifiedOnline {
		t.Fatalf("online truth=%#v", truth)
	}
	if truth.Restorable || truth.RestorationReason != "checkpoint_not_captured" {
		t.Fatalf("restoration truth=%#v", truth)
	}

	other := session
	other.RuntimeSnapshotID = "snapshot-2"
	if err := twin.BindSession(other); err == nil {
		t.Fatal("runtime snapshot rebinding unexpectedly succeeded")
	}
}

func TestDigitalTwinRecordsScenarioAndExecutionEvents(t *testing.T) {
	stateStore := &recordingSimulationStore{session: observableSimulationSession()}
	twin := NewDigitalTwinWithStateStore(stateStore)
	if err := twin.ConfigureFault(stateStore.session.ID, FaultScenario{
		TargetRef: "lighting.front",
		Behavior:  "FAIL",
		ErrorCode: "SIMULATED_DIMMER_FAULT",
		Uses:      1,
	}); err != nil {
		t.Fatal(err)
	}

	req := twinRequest(stateStore.session.ID, "lighting.front", "osc.send")
	req.ProjectID = stateStore.session.ProjectID
	req.RuntimeSnapshotID = stateStore.session.RuntimeSnapshotID
	result := twin.Execute(context.Background(), req)
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SIMULATED_DIMMER_FAULT" {
		t.Fatalf("result=%#v", result)
	}

	wantTypes := []string{
		"simulation.fault.configured",
		"simulation.fault.consumed",
		"simulation.execution.failed",
	}
	if len(stateStore.events) != len(wantTypes) {
		t.Fatalf("events=%#v", stateStore.events)
	}
	for i, want := range wantTypes {
		event := stateStore.events[i]
		if event.EventType != want {
			t.Fatalf("event[%d]=%q want %q", i, event.EventType, want)
		}
		if event.ProjectID != stateStore.session.ProjectID || event.RuntimeSnapshotID != stateStore.session.RuntimeSnapshotID {
			t.Fatalf("event authority=%#v", event)
		}
		if event.Source != "stagecore.simulator.digital_twin" {
			t.Fatalf("event source=%q", event.Source)
		}
	}
}

func TestDigitalTwinRejectsMutationOutsideSimulation(t *testing.T) {
	session := observableSimulationSession()
	session.Type = domain.SessionRehearsal
	stateStore := &recordingSimulationStore{session: session}
	twin := NewDigitalTwinWithStateStore(stateStore)
	if err := twin.ConfigureFault(session.ID, FaultScenario{TargetRef: "x", Behavior: "FAIL"}); err == nil {
		t.Fatal("REHEARSAL mutation unexpectedly accepted")
	}
	if len(stateStore.events) != 0 {
		t.Fatalf("unexpected events=%#v", stateStore.events)
	}
}

func TestSessionExecutorAttachesCanonicalSimulationObservability(t *testing.T) {
	stateStore := &recordingSimulationStore{session: observableSimulationSession()}
	resolver := &observableResolver{recordingSimulationStore: stateStore}
	physical := &physicalProbe{}
	twin := NewDigitalTwin()
	executor := NewSessionExecutorWithDigitalTwin(resolver, physical, twin)

	result := executor.Execute(context.Background(), capability.Request{
		ExecutionID: "action-execution-1",
		Capability:  "sim.test",
		Target:      &capability.Target{Ref: "virtual.1", LogicalType: "virtual"},
	})
	if result.Result != domain.ExecutionCompleted {
		t.Fatalf("result=%#v", result)
	}
	if physical.calls != 0 {
		t.Fatalf("physical executor calls=%d", physical.calls)
	}
	if len(stateStore.events) != 1 || stateStore.events[0].EventType != "simulation.execution.completed" {
		t.Fatalf("events=%#v", stateStore.events)
	}
	if got := twin.Snapshot(stateStore.session.ID).RuntimeSnapshotID; got != stateStore.session.RuntimeSnapshotID {
		t.Fatalf("runtime snapshot binding=%q", got)
	}
}

func TestDigitalTwinMutationRequiresActiveLifecycleWhenObserved(t *testing.T) {
	session := observableSimulationSession()
	session.LifecycleState = domain.SessionLifecycleStopped
	stateStore := &recordingSimulationStore{session: session}
	twin := NewDigitalTwinWithStateStore(stateStore)
	if err := twin.SetTargetOnline(session.ID, "x", false); err == nil {
		t.Fatal("stopped simulation mutation unexpectedly accepted")
	}
}

func TestDigitalTwinPropagatesEventRecorderFailureForMutation(t *testing.T) {
	stateStore := &failingSimulationStore{session: observableSimulationSession()}
	twin := NewDigitalTwinWithStateStore(stateStore)
	if err := twin.ConfigureFault(stateStore.session.ID, FaultScenario{TargetRef: "x", Behavior: "FAIL"}); err == nil {
		t.Fatal("mutation unexpectedly succeeded when event recorder failed")
	}
	if faults := twin.Snapshot(stateStore.session.ID).Faults; len(faults) != 0 {
		t.Fatalf("unobserved fault mutation leaked into state: %#v", faults)
	}
}

type failingSimulationStore struct {
	session domain.Session
}

func (s *failingSimulationStore) GetSessionFoundation(_ context.Context, sessionID string) (domain.Session, error) {
	if s.session.ID != sessionID {
		return domain.Session{}, domain.ErrNotFound
	}
	return s.session, nil
}

func (s *failingSimulationStore) AppendEvent(context.Context, *string, contracts.EventEnvelope) (contracts.EventEnvelope, error) {
	return contracts.EventEnvelope{}, errors.New("event store unavailable")
}
