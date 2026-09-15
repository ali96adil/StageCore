package simulationcontrol_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/recovery"
	"github.com/ali96adil/StageCore/internal/simulationcontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type f020FaultHarness struct {
	store    *store.Store
	control  *simulationcontrol.Service
	twin     *simulator.DigitalTwin
	physical *physicalProbe
	project  domain.Project
	snapshot domain.RuntimeSnapshot
}

func newF020FaultHarness(t *testing.T, cueCount int, timeoutPolicy json.RawMessage) *f020FaultHarness {
	t.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })

	s := store.New(handle.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-020 fault qualification"})
	if err != nil {
		t.Fatal(err)
	}
	if len(timeoutPolicy) == 0 {
		timeoutPolicy = json.RawMessage(`{}`)
	}
	for i := 0; i < cueCount; i++ {
		action := domain.Action{
			OrderIndex:      0,
			ExecutionMode:   "SEQUENTIAL",
			TargetRef:       "SIM-TARGET",
			CapabilityKey:   "osc.send",
			Parameters:      json.RawMessage(`{}`),
			TimeoutPolicy:   append(json.RawMessage(nil), timeoutPolicy...),
			ErrorPolicy:     json.RawMessage(`{"on_error":"FAIL_CUE"}`),
			PriorityClass:   domain.PriorityP1,
			Enabled:         true,
		}
		if _, err := s.CreateCueWithActions(ctx, domain.Cue{
			RevisionID:  revision.ID,
			DisplayLabel: fmt.Sprintf("%d", i+1),
			Name:        fmt.Sprintf("Fault Cue %d", i+1),
			OrderIndex:  i,
			CueType:     "STANDARD",
			Criticality: "NORMAL",
			Enabled:     true,
		}, []domain.Action{action}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}

	physical := &physicalProbe{}
	twin, control := newF020SimulationControl(s, physical)
	return &f020FaultHarness{
		store: s, control: control, twin: twin, physical: physical,
		project: project, snapshot: runtimeSnapshot,
	}
}

func newF020SimulationControl(s *store.Store, physical *physicalProbe) (*simulator.DigitalTwin, *simulationcontrol.Service) {
	twin := simulator.NewDigitalTwin()
	executor := simulator.NewSessionExecutorWithDigitalTwin(s, physical, twin)
	engine := cueengine.NewWithExecutor(s, executor)
	return twin, simulationcontrol.New(s, engine, twin)
}

func f020RequestID(n int) string {
	return fmt.Sprintf("00000000-0000-7000-8000-%012d", n)
}

func TestF020DisconnectCheckpointRestartRequiresNewSimulationWithoutReplay(t *testing.T) {
	ctx := context.Background()
	h := newF020FaultHarness(t, 3, nil)

	oldSession, start := h.control.Start(ctx, simulationcontrol.StartRequest{
		ProjectID: h.project.ID,
		Name:      "fault recovery source",
		Issuer:    "operator",
		RequestID: f020RequestID(201),
		StartKind: domain.SessionStartBeginning,
	})
	if start.Status != contracts.CommandCompleted || oldSession.RuntimeSnapshotID != h.snapshot.ID {
		t.Fatalf("start session=%+v result=%+v", oldSession, start)
	}
	if first := h.control.Go(ctx, simulationcontrol.CueRequest{
		SessionID: oldSession.ID, Issuer: "operator", RequestID: f020RequestID(202),
	}); first.Status != contracts.CommandCompleted {
		t.Fatalf("first GO=%+v", first)
	}

	if err := h.control.ConfigureFault(ctx, oldSession.ID, simulator.FaultScenario{
		Capability: "osc.send", Behavior: "DISCONNECT", Uses: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if disconnected := h.control.Go(ctx, simulationcontrol.CueRequest{
		SessionID: oldSession.ID, Issuer: "operator", RequestID: f020RequestID(203),
	}); disconnected.Status != contracts.CommandFailed {
		t.Fatalf("disconnect GO=%+v", disconnected)
	}
	if h.physical.calls != 0 {
		t.Fatalf("SIMULATION reached physical executor %d time(s)", h.physical.calls)
	}
	beforeCheckpoint := h.twin.Snapshot(oldSession.ID)
	if len(beforeCheckpoint.Targets) != 1 || beforeCheckpoint.Targets[0].Online {
		t.Fatalf("disconnected twin=%+v", beforeCheckpoint)
	}

	checkpoint, err := h.control.CaptureCheckpoint(ctx, oldSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldExecutions, err := h.store.ListCueExecutions(ctx, oldSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldExecutions) != 2 {
		t.Fatalf("old cue executions=%d want 2", len(oldExecutions))
	}

	reconciled, err := h.store.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled != 1 {
		t.Fatalf("reconciled Sessions=%d want 1", reconciled)
	}
	oldAfterRestart, err := h.store.GetSessionFoundation(ctx, oldSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if oldAfterRestart.Status != domain.SessionAborted || oldAfterRestart.LifecycleState != domain.SessionLifecycleAborted || oldAfterRestart.EndReason != "HUB_RESTART_INTERRUPTED" {
		t.Fatalf("old Session after restart=%+v", oldAfterRestart)
	}
	if oldAfterRestart.StateTruth.RestorationStatus != domain.SessionRestorationManualConfirmationRequired ||
		!oldAfterRestart.StateTruth.ManualConfirmationRequired ||
		oldAfterRestart.StateTruth.DesiredStateRef == nil || *oldAfterRestart.StateTruth.DesiredStateRef != checkpoint.ID ||
		oldAfterRestart.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("old recovery truth=%+v", oldAfterRestart.StateTruth)
	}
	assertF020RecoveryDecision(t, h.store, oldSession.ID, recovery.DispositionManualConfirmation, checkpoint.ID, true)

	// Simulate a fresh Hub process: a new in-memory Digital Twin is created, and
	// only the durable checkpoint is allowed to reconstruct virtual state.
	freshTwin, freshControl := newF020SimulationControl(h.store, h.physical)
	newSession, restored := freshControl.Start(ctx, simulationcontrol.StartRequest{
		ProjectID:    h.project.ID,
		Name:         "fault recovery target",
		Issuer:       "operator",
		RequestID:    f020RequestID(204),
		StartKind:    domain.SessionStartCheckpoint,
		CheckpointID: checkpoint.ID,
	})
	if restored.Status != contracts.CommandCompleted || newSession.ID == oldSession.ID {
		t.Fatalf("restored session=%+v result=%+v", newSession, restored)
	}
	if newSession.RuntimeSnapshotID != h.snapshot.ID ||
		newSession.StateTruth.RestorationStatus != domain.SessionRestorationRestorable ||
		newSession.StateTruth.ManualConfirmationRequired ||
		newSession.StateTruth.DesiredStateRef == nil || newSession.StateTruth.VerifiedStateRef == nil ||
		*newSession.StateTruth.DesiredStateRef != checkpoint.ID || *newSession.StateTruth.VerifiedStateRef != checkpoint.ID {
		t.Fatalf("new recovery truth=%+v", newSession.StateTruth)
	}
	newExecutions, err := h.store.ListCueExecutions(ctx, newSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(newExecutions) != 0 {
		t.Fatalf("checkpoint restore replayed %d cue execution(s)", len(newExecutions))
	}
	restoredTwin := freshTwin.Snapshot(newSession.ID)
	if len(restoredTwin.Targets) != 1 || restoredTwin.Targets[0].Online || !restoredTwin.Targets[0].StateTruth.Restorable {
		t.Fatalf("restored twin=%+v", restoredTwin)
	}

	if err := freshControl.ConfigureFault(ctx, newSession.ID, simulator.FaultScenario{
		Capability: "osc.send", Behavior: "RECONNECT", Uses: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if reconnect := freshControl.Go(ctx, simulationcontrol.CueRequest{
		SessionID: newSession.ID, Issuer: "operator", RequestID: f020RequestID(205),
	}); reconnect.Status != contracts.CommandCompleted {
		t.Fatalf("reconnect GO=%+v", reconnect)
	}
	if afterReconnect := freshTwin.Snapshot(newSession.ID); len(afterReconnect.Targets) != 1 || !afterReconnect.Targets[0].Online {
		t.Fatalf("twin after reconnect=%+v", afterReconnect)
	}
	newExecutions, err = h.store.ListCueExecutions(ctx, newSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldExecutionsAfter, err := h.store.ListCueExecutions(ctx, oldSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(newExecutions) != 1 || len(oldExecutionsAfter) != 2 {
		t.Fatalf("execution counts new=%d old=%d", len(newExecutions), len(oldExecutionsAfter))
	}
	if h.physical.calls != 0 {
		t.Fatalf("fault recovery reached physical executor %d time(s)", h.physical.calls)
	}
}

func TestF020SimulationFaultsFailClosedWithoutAutomaticReplay(t *testing.T) {
	cases := []struct {
		name          string
		behavior      string
		timeoutPolicy json.RawMessage
		wantStatus    contracts.CommandStatus
	}{
		{name: "fail", behavior: "FAIL", wantStatus: contracts.CommandFailed},
		{name: "reject", behavior: "REJECT", wantStatus: contracts.CommandFailed},
		{name: "offline", behavior: "OFFLINE", wantStatus: contracts.CommandFailed},
		{name: "unavailable", behavior: "UNAVAILABLE", wantStatus: contracts.CommandFailed},
		{name: "timeout", behavior: "TIMEOUT", timeoutPolicy: json.RawMessage(`{"timeout_ms":5}`), wantStatus: contracts.CommandTimedOut},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := newF020FaultHarness(t, 1, tc.timeoutPolicy)
			session, start := h.control.Start(ctx, simulationcontrol.StartRequest{
				ProjectID: h.project.ID,
				Issuer:    "operator",
				RequestID: f020RequestID(300 + i*10),
				StartKind: domain.SessionStartBeginning,
			})
			if start.Status != contracts.CommandCompleted {
				t.Fatalf("start=%+v", start)
			}
			if err := h.control.ConfigureFault(ctx, session.ID, simulator.FaultScenario{
				Capability: "osc.send", Behavior: tc.behavior, Uses: 1,
			}); err != nil {
				t.Fatal(err)
			}
			result := h.control.Go(ctx, simulationcontrol.CueRequest{
				SessionID: session.ID, Issuer: "operator", RequestID: f020RequestID(301 + i*10),
			})
			if result.Status != tc.wantStatus {
				t.Fatalf("fault %s result=%+v want status=%s", tc.behavior, result, tc.wantStatus)
			}
			if h.physical.calls != 0 {
				t.Fatalf("fault %s reached physical executor %d time(s)", tc.behavior, h.physical.calls)
			}
			before, err := h.store.ListCueExecutions(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(before) != 1 || before[0].Result == domain.ExecutionRunning {
				t.Fatalf("fault %s executions=%+v", tc.behavior, before)
			}

			reconciled, err := h.store.ReconcileInterruptedRuntimeForHub(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if reconciled != 1 {
				t.Fatalf("fault %s reconciled=%d want 1", tc.behavior, reconciled)
			}
			loaded, err := h.store.GetSessionFoundation(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Status != domain.SessionAborted ||
				loaded.StateTruth.RestorationStatus != domain.SessionRestorationUnavailable ||
				loaded.StateTruth.ManualConfirmationRequired ||
				loaded.StateTruth.DesiredStateRef != nil || loaded.StateTruth.VerifiedStateRef != nil {
				t.Fatalf("fault %s restart truth=%+v session=%+v", tc.behavior, loaded.StateTruth, loaded)
			}
			after, err := h.store.ListCueExecutions(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != 1 || after[0].ID != before[0].ID {
				t.Fatalf("fault %s restart replayed work: before=%+v after=%+v", tc.behavior, before, after)
			}
			assertF020RecoveryDecision(t, h.store, session.ID, recovery.DispositionAbort, "", false)
		})
	}
}

func assertF020RecoveryDecision(t *testing.T, s *store.Store, sessionID string, wantDisposition recovery.Disposition, wantCheckpointID string, wantManual bool) {
	t.Helper()
	events, err := s.ListEvents(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.EventType != "runtime.recovery.decision" {
			continue
		}
		var payload struct {
			Disposition                      recovery.Disposition `json:"disposition"`
			ReplayAllowed                    bool                 `json:"replay_allowed"`
			ManualConfirmationRequired       bool                 `json:"manual_confirmation_required"`
			CheckpointID                     string               `json:"checkpoint_id"`
			ReconstructionStartKind          string               `json:"reconstruction_start_kind"`
			ReconstructionRequiresNewSession bool                 `json:"reconstruction_requires_new_session"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Disposition != wantDisposition || payload.ReplayAllowed || payload.ManualConfirmationRequired != wantManual || payload.CheckpointID != wantCheckpointID {
			t.Fatalf("recovery decision=%+v want disposition=%s checkpoint=%q manual=%v", payload, wantDisposition, wantCheckpointID, wantManual)
		}
		if wantCheckpointID != "" {
			if payload.ReconstructionStartKind != string(domain.SessionStartCheckpoint) || !payload.ReconstructionRequiresNewSession {
				t.Fatalf("checkpoint reconstruction evidence=%+v", payload)
			}
		} else if payload.ReconstructionStartKind != "" || payload.ReconstructionRequiresNewSession {
			t.Fatalf("unexpected reconstruction authority=%+v", payload)
		}
		return
	}
	t.Fatal("runtime.recovery.decision event not found")
}
