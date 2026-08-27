package runtimecontrol

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type runtimeHarness struct {
	store    *store.Store
	service  *Service
	project  domain.Project
	snapshot domain.RuntimeSnapshot
	cues     []domain.Cue
}

func newRuntimeHarness(t *testing.T, cueParameters ...json.RawMessage) *runtimeHarness {
	t.Helper()
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	s := store.New(h.DB, clock.Real{})
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Runtime Test", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	if len(cueParameters) == 0 {
		cueParameters = []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`{}`)}
	}
	cues := make([]domain.Cue, 0, len(cueParameters))
	for i, parameters := range cueParameters {
		cue, err := s.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: string(rune('1' + i)), Name: "Cue", OrderIndex: i + 1,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		}, []domain.Action{{
			OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
			Parameters: parameters, TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		}})
		if err != nil {
			t.Fatal(err)
		}
		cues = append(cues, cue)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return &runtimeHarness{store: s, service: New(s, registry), project: project, snapshot: published, cues: cues}
}

func TestRehearsalGoJumpNoReplayAndStopSession(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()

	session, startResult := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Name: "Rehearsal 1", Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000101",
	})
	if startResult.Status != contracts.CommandCompleted || session.ID == "" || session.RuntimeSnapshotID != h.snapshot.ID {
		t.Fatalf("start result=%+v session=%+v", startResult, session)
	}

	firstResult := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000000102",
	})
	if firstResult.Status != contracts.CommandCompleted {
		t.Fatalf("first GO=%+v", firstResult)
	}
	state, err := h.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentCueID == nil || *state.CurrentCueID != h.cues[0].ID {
		t.Fatalf("current cue=%v, want %s", state.CurrentCueID, h.cues[0].ID)
	}

	duplicate := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000000102",
	})
	if duplicate.Status != contracts.CommandCompleted {
		t.Fatalf("duplicate GO did not return stored terminal result: %+v", duplicate)
	}
	executions, err := h.store.ListCueExecutions(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 {
		t.Fatalf("duplicate GO replayed Cue: executions=%d", len(executions))
	}

	expected := h.cues[0].ID
	requested := h.cues[1].ID
	jumpResult := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000000103",
		ExpectedCurrentCueID: &expected, RequestedCueID: &requested,
	})
	if jumpResult.Status != contracts.CommandCompleted {
		t.Fatalf("Jump through cue.go requested_next failed: %+v", jumpResult)
	}
	state, err = h.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentCueID == nil || *state.CurrentCueID != h.cues[1].ID {
		t.Fatalf("current cue after Jump=%v, want %s", state.CurrentCueID, h.cues[1].ID)
	}

	stopResult := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000000104",
	})
	if stopResult.Status != contracts.CommandCompleted {
		t.Fatalf("stop Rehearsal=%+v", stopResult)
	}
	state, err = h.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != domain.SessionCompleted || state.EndedAt == nil {
		t.Fatalf("session not explicitly completed: %+v", state)
	}
}

func TestCueStopCancelsInterruptibleActionAndPersistsTruthfulResult(t *testing.T) {
	parameters := json.RawMessage(`{"simulation":{"behavior":"COMPLETE","delay_ms":5000}}`)
	h := newRuntimeHarness(t, parameters)
	ctx := context.Background()
	session, startResult := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000201",
	})
	if startResult.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", startResult)
	}

	goResultCh := make(chan contracts.CommandResult, 1)
	go func() {
		goResultCh <- h.service.Go(context.Background(), CueRequest{
			SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000000202",
		})
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		running, err := h.store.HasRunningCueExecution(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Cue did not enter RUNNING state before stop test deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}

	stop := h.service.StopCue(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000000203",
	})
	if stop.Status != contracts.CommandCompleted {
		t.Fatalf("cue.stop=%+v", stop)
	}
	select {
	case goResult := <-goResultCh:
		if goResult.Status != contracts.CommandCancelled {
			t.Fatalf("GO after STOP status=%s, want CANCELLED: %+v", goResult.Status, goResult)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GO did not terminate after cue.stop")
	}

	executions, err := h.store.ListCueExecutions(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 || executions[0].Result != domain.ExecutionCancelled || executions[0].CompletedAt == nil {
		t.Fatalf("Cue execution did not persist CANCELLED truthfully: %+v", executions)
	}
	actions, err := h.store.ListActionExecutions(ctx, executions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Result != domain.ExecutionCancelled || actions[0].ErrorCode == nil || *actions[0].ErrorCode != "CANCELLED" {
		t.Fatalf("Action execution did not persist cancellation: %+v", actions)
	}
}

func TestShowRemainsBlockedUntilPreflightGateIsInstalled(t *testing.T) {
	h := newRuntimeHarness(t)
	_, result := h.service.StartSession(context.Background(), StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionShow, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000301",
	})
	if result.Status != contracts.CommandRejected || result.Error == nil || result.Error.ErrorCode != "SHOW_PREFLIGHT_REQUIRED" {
		t.Fatalf("SHOW without S3 Preflight gate=%+v", result)
	}
	active, err := h.store.ActiveSessionForProject(context.Background(), h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("SHOW rejection created active Session: %+v", active)
	}
}
