package simulationcontrol_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
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
		TargetRef: "SIM-TARGET",
		CapabilityKey: "osc.send",
		Parameters: json.RawMessage(`{}`),
		TimeoutPolicy: json.RawMessage(`{}`),
		ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1,
		Enabled: true,
	}
	for i, name := range []string{"First", "Second"} {
		copy := action
		copy.OrderIndex = 0
		if _, err := s.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID,
			DisplayLabel: string(rune('1' + i)),
			Name: name,
			OrderIndex: i,
			CueType: "STANDARD",
			Criticality: "NORMAL",
			Enabled: true,
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
		Name: "full show",
		Issuer: "operator",
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
		Issuer: "operator",
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
		Behavior: "FAIL",
		ErrorCode: "VIRTUAL_DEVICE_FAILURE",
		Uses: 1,
	}); err != nil {
		t.Fatal(err)
	}
	second := control.Go(ctx, simulationcontrol.CueRequest{
		SessionID: session.ID,
		Issuer: "operator",
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
		ProjectID: project.ID,
		Issuer: "operator",
		RequestID: "00000000-0000-7000-8000-000000000111",
		StartKind: domain.SessionStartRange,
		StartCueID: cues[1].ID,
		EndCueID: cues[2].ID,
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
