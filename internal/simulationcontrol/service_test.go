package simulationcontrol_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/simulationcontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type physicalProbe struct{ calls int }

func (p *physicalProbe) Execute(context.Context, capability.Request) capability.Result {
	p.calls++
	return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice, ResponseSummary: "physical path reached"}
}

func TestOperatorSimulationStartGoFaultCheckpointNeverReachesPhysicalExecutor(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Operator Simulation"})
	if err != nil {
		t.Fatal(err)
	}

	action := domain.Action{
		ExecutionMode: "SEQUENTIAL",
		TargetRef:     "SIM-TARGET",
		CapabilityKey: "osc.send",
		Parameters:    json.RawMessage(`{}`),
		TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy:   json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1,
		Enabled:       true,
	}
	for i, name := range []string{"First", "Second"} {
		copy := action
		copy.OrderIndex = 0
		if _, err := s.CreateCueWithActions(ctx, domain.Cue{
			RevisionID:   revision.ID,
			DisplayLabel: string(rune('1' + i)),
			Name:         name,
			OrderIndex:   i,
			CueType:      "STANDARD",
			Criticality:  "NORMAL",
			Enabled:      true,
		}, []domain.Action{copy}); err != nil {
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
	twin := simulator.NewDigitalTwin()
	executor := simulator.NewSessionExecutorWithDigitalTwin(s, physical, twin)
	engine := cueengine.NewWithExecutor(s, executor)
	control := simulationcontrol.New(s, engine, twin)
	if control == nil {
		t.Fatal("simulation control is nil")
	}

	session, result := control.Start(ctx, simulationcontrol.StartRequest{
		ProjectID: project.ID,
		Name:      "full show",
		Issuer:    "operator",
		RequestID: "00000000-0000-7000-8000-000000000101",
		StartKind: domain.SessionStartBeginning,
	})
	if result.Status != contracts.CommandCompleted || session.Type != domain.SessionSimulation {
		t.Fatalf("start session=%+v result=%+v", session, result)
	}
	if session.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("snapshot=%s want=%s", session.RuntimeSnapshotID, runtimeSnapshot.ID)
	}

	first := control.Go(ctx, simulationcontrol.CueRequest{
		SessionID: session.ID,
		Issuer:    "operator",
		RequestID: "00000000-0000-7000-8000-000000000102",
	})
	if first.Status != contracts.CommandCompleted {
		t.Fatalf("first GO=%+v", first)
	}
	if physical.calls != 0 {
		t.Fatalf("SIMULATION reached physical executor %d time(s)", physical.calls)
	}

	checkpoint, err := control.CaptureCheckpoint(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.RuntimeSnapshotID != runtimeSnapshot.ID || checkpoint.ContentHash == "" {
		t.Fatalf("checkpoint=%+v", checkpoint)
	}

	// SIM-TARGET is intentionally not present in the Runtime Snapshot target
	// alias table. Faulting by capability is therefore the truthful selector for
	// this missing-mapping case; Slice E reports that authored target separately.
	if err := control.ConfigureFault(ctx, session.ID, simulator.FaultScenario{
		Capability: "osc.send",
		Behavior:   "FAIL",
		ErrorCode:  "VIRTUAL_DEVICE_FAILURE",
		Uses:       1,
	}); err != nil {
		t.Fatal(err)
	}
	second := control.Go(ctx, simulationcontrol.CueRequest{
		SessionID: session.ID,
		Issuer:    "operator",
		RequestID: "00000000-0000-7000-8000-000000000103",
	})
	if second.Status != contracts.CommandFailed {
		t.Fatalf("faulted GO=%+v", second)
	}
	if physical.calls != 0 {
		t.Fatalf("faulted SIMULATION reached physical executor %d time(s)", physical.calls)
	}

	status, err := control.Status(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Session == nil || status.Session.ID != session.ID || len(status.Checkpoints) != 1 || len(status.CueExecutions) != 2 {
		t.Fatalf("status=%+v", status)
	}
	if status.Twin.RuntimeSnapshotID != runtimeSnapshot.ID || len(status.Twin.Targets) != 1 {
		t.Fatalf("twin=%+v", status.Twin)
	}
	seenStart := false
	seenSimulatedExecution := false
	for _, event := range status.Events {
		switch event.EventType {
		case "simulation.started":
			seenStart = true
		case "simulation.execution.completed", "simulation.execution.failed":
			seenSimulatedExecution = true
		}
	}
	if !seenStart || !seenSimulatedExecution {
		t.Fatalf("simulation events missing start=%v execution=%v", seenStart, seenSimulatedExecution)
	}
}

func TestSimulationStatusSerializesCurrentAndNextCueForOperatorUI(t *testing.T) {
	current, next := "cue-current", "cue-next"
	payload, err := json.Marshal(simulationcontrol.Status{
		ProjectID: "project",
		Session: &domain.Session{
			ID: "session", ProjectID: "project", RuntimeSnapshotID: "snapshot",
			Type: domain.SessionSimulation, Status: domain.SessionActive,
			CurrentCueID: &current, NextCueID: &next,
			StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartBeginning},
			StateTruth:    domain.SessionStateTruth{RestorationStatus: domain.SessionRestorationNotRequired},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	session, ok := wire["session"].(map[string]any)
	if !ok || session["session_id"] != "session" || session["current_cue_id"] != current || session["next_cue_id"] != next {
		t.Fatalf("Simulation status omitted snake_case current/next Cue fields: %s", payload)
	}
}

func TestScheduledMixedSimulationGOIsOwnedAndCancelledByRuntimeSTOP(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Mixed Simulation STOP"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	makeAction := func(index int, key string, parameters string) domain.Action {
		return domain.Action{OrderIndex: index, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: key,
			Parameters: json.RawMessage(parameters), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: domain.PriorityP1, Enabled: true}
	}
	terminal, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1.1.1", Name: "Delayed Linked Cue", OrderIndex: 3,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{"start_delay_ms":8000}`),
	}, []domain.Action{makeAction(0, "sim.test", `{}`)})
	if err != nil {
		t.Fatal(err)
	}
	linkedPolicy, _ := json.Marshal(map[string]any{"linked_cue_ids": []string{terminal.ID}, "linked_cue_mode": "TOGETHER"})
	linked, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1.1", Name: "Mixed Linked Child", OrderIndex: 2,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: linkedPolicy,
	}, []domain.Action{makeAction(0, "sim.test", `{"branch":"lighting"}`), makeAction(1, "sim.secondary", `{"branch":"visual"}`)})
	if err != nil {
		t.Fatal(err)
	}
	rootPolicy, _ := json.Marshal(map[string]any{"linked_cue_ids": []string{linked.ID}, "linked_cue_mode": "TOGETHER"})
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Mixed Root", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: rootPolicy,
	}, []domain.Action{makeAction(0, "sim.test", `{"branch":"root"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "operator")
	if err != nil {
		t.Fatal(err)
	}
	physical := &physicalProbe{}
	twin := simulator.NewDigitalTwin()
	simulationExecutor := simulator.NewSessionExecutorWithDigitalTwin(s, physical, twin)
	runtime := runtimecontrol.New(s, physical, runtimecontrol.WithSimulationExecutor(simulationExecutor))
	control := simulationcontrol.New(s, cueengine.NewWithExecutor(s, simulationExecutor), twin, runtime)
	session, started := control.Start(ctx, simulationcontrol.StartRequest{
		ProjectID: project.ID, Name: "Temporary mixed cue test", Issuer: "operator",
		RequestID: "00000000-0000-7000-8000-000000000801", StartKind: domain.SessionStartBeginning,
	})
	if started.Status != contracts.CommandCompleted || session.RuntimeSnapshotID != published.ID {
		t.Fatalf("simulation start=%+v session=%+v", started, session)
	}

	goResult := make(chan contracts.CommandResult, 1)
	go func() {
		goResult <- control.Go(ctx, simulationcontrol.CueRequest{
			SessionID: session.ID, Issuer: "operator", RequestID: "00000000-0000-7000-8000-000000000802",
		})
	}()
	var execution domain.CueExecution
	var actions []domain.ActionExecution
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		executions, listErr := s.ListCueExecutions(ctx, session.ID)
		if listErr != nil {
			t.Fatal(listErr)
		}
		if len(executions) == 1 {
			execution = executions[0]
			actions, listErr = s.ListActionExecutions(ctx, execution.ID)
			if listErr != nil {
				t.Fatal(listErr)
			}
			if len(actions) == 3 {
				allDone := true
				for _, action := range actions {
					allDone = allDone && action.Result == domain.ExecutionCompleted
				}
				if allDone {
					break
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if execution.ID == "" || execution.Result != domain.ExecutionRunning || len(actions) != 3 {
		t.Fatalf("Mixed execution did not reach its terminal Cue delay: Cue=%+v Actions=%+v", execution, actions)
	}
	select {
	case result := <-goResult:
		if result.Status != contracts.CommandAccepted {
			t.Fatalf("Simulation GO acknowledgement=%+v, want durable ACCEPTED", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Simulation GO did not acknowledge the durable schedule")
	}
	for _, action := range actions {
		if action.Result != domain.ExecutionCompleted || action.ActionID == terminal.Actions[0].ID {
			t.Fatalf("unexpected Action before STOP while linked Cue is delayed: %+v", action)
		}
	}

	stop := runtime.StopCue(ctx, runtimecontrol.StopRequest{
		SessionID: session.ID, Issuer: "operator", RequestID: "00000000-0000-7000-8000-000000000803",
	})
	if stop.Status != contracts.CommandCompleted {
		t.Fatalf("Runtime STOP did not own scheduled Simulation run: %+v", stop)
	}
	finished, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil || len(finished) != 1 || finished[0].Result != domain.ExecutionCancelled || finished[0].CompletedAt == nil {
		t.Fatalf("STOP must leave a terminal CueExecution, not RUNNING: executions=%+v err=%v", finished, err)
	}
	finalActions, err := s.ListActionExecutions(ctx, execution.ID)
	if err != nil || len(finalActions) != 3 || physical.calls != 0 {
		t.Fatalf("delayed linked Action was dispatched or physical output reached: actions=%+v err=%v physical_calls=%d", finalActions, err, physical.calls)
	}
	command, found, err := s.FindCommandRecord(ctx, contracts.CommandEnvelope{
		CommandID: "00000000-0000-7000-8000-000000000802", CommandType: cueengine.CueGoCommandType, ProjectID: project.ID,
	})
	if err != nil || !found || command.Status != string(contracts.CommandCancelled) {
		t.Fatalf("accepted Cue command did not reach terminal cancellation: command=%+v found=%v err=%v", command, found, err)
	}
}

func TestOperatorSimulationRejectsRangeGoUntilExplicitStartConfirmation(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Range Simulation"})
	if err != nil {
		t.Fatal(err)
	}
	cues := make([]domain.Cue, 0, 3)
	for i, name := range []string{"One", "Two", "Three"} {
		cue, err := s.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: string(rune('1' + i)), Name: name, OrderIndex: i, Enabled: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		cues = append(cues, cue)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test"); err != nil {
		t.Fatal(err)
	}
	twin := simulator.NewDigitalTwin()
	engine := cueengine.NewWithExecutor(s, simulator.NewSessionExecutorWithDigitalTwin(s, simulator.Adapter{}, twin))
	control := simulationcontrol.New(s, engine, twin)
	session, result := control.Start(ctx, simulationcontrol.StartRequest{
		ProjectID:  project.ID,
		Issuer:     "operator",
		RequestID:  "00000000-0000-7000-8000-000000000111",
		StartKind:  domain.SessionStartRange,
		StartCueID: cues[1].ID,
		EndCueID:   cues[2].ID,
	})
	if result.Status != contracts.CommandCompleted || !session.StateTruth.ManualConfirmationRequired {
		t.Fatalf("range start session=%+v result=%+v", session, result)
	}
	blocked := control.Go(ctx, simulationcontrol.CueRequest{SessionID: session.ID, Issuer: "operator", RequestID: "00000000-0000-7000-8000-000000000112"})
	if blocked.Status != contracts.CommandRejected || blocked.Error == nil || blocked.Error.ErrorCode != "SIMULATION_START_CONFIRMATION_REQUIRED" {
		t.Fatalf("blocked GO=%+v", blocked)
	}
	confirmed, err := control.ConfirmStartState(ctx, session.ID, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.StateTruth.ManualConfirmationRequired || confirmed.StateTruth.VerifiedStateRef != nil || confirmed.StateTruth.RestorationStatus != domain.SessionRestorationUnavailable {
		t.Fatalf("confirmed truth=%+v", confirmed.StateTruth)
	}
	allowed := control.Go(ctx, simulationcontrol.CueRequest{SessionID: session.ID, Issuer: "operator", RequestID: "00000000-0000-7000-8000-000000000113"})
	if allowed.Status != contracts.CommandCompleted {
		t.Fatalf("allowed GO=%+v", allowed)
	}
}
