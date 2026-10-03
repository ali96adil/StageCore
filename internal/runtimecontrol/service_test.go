package runtimecontrol

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestEmergencyBlackoutLatchesBlocksGoAndRequiresExplicitClear(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()
	session, startResult := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000701",
	})
	if startResult.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", startResult)
	}

	var calls []bool
	h.service.emergencySafety = func(_ context.Context, got domain.Session, command contracts.CommandEnvelope, enabled bool) (json.RawMessage, error) {
		if got.ID != session.ID || command.Priority != "P0" {
			t.Fatalf("emergency authority session=%s priority=%s", got.ID, command.Priority)
		}
		calls = append(calls, enabled)
		return json.RawMessage(`{"lighting":{"status":"COMPLETED"},"audio":{"status":"UNCHANGED_BY_DESIGN"}}`), nil
	}

	activated := h.service.EmergencyBlackout(ctx, EmergencyRequest{
		SessionID: session.ID, Issuer: "owner", Enabled: true,
		RequestID: "00000000-0000-7000-8000-000000000702",
	})
	if activated.Status != contracts.CommandCompleted {
		t.Fatalf("activate blackout=%+v", activated)
	}
	latched, err := h.store.SessionManagedOutputBlackout(ctx, session.ID)
	if err != nil || !latched {
		t.Fatalf("blackout latch=%v err=%v", latched, err)
	}

	blocked := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000703",
	})
	if blocked.Status != contracts.CommandRejected || blocked.Error == nil || blocked.Error.ErrorCode != "EMERGENCY_BLACKOUT_ACTIVE" {
		t.Fatalf("GO during blackout=%+v", blocked)
	}

	cleared := h.service.EmergencyBlackout(ctx, EmergencyRequest{
		SessionID: session.ID, Issuer: "owner", Enabled: false,
		RequestID: "00000000-0000-7000-8000-000000000704",
	})
	if cleared.Status != contracts.CommandCompleted {
		t.Fatalf("clear blackout=%+v", cleared)
	}
	latched, err = h.store.SessionManagedOutputBlackout(ctx, session.ID)
	if err != nil || latched {
		t.Fatalf("cleared blackout latch=%v err=%v", latched, err)
	}
	if len(calls) != 2 || !calls[0] || calls[1] {
		t.Fatalf("emergency callbacks=%v", calls)
	}

	goResult := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000705",
	})
	if goResult.Status != contracts.CommandCompleted {
		t.Fatalf("GO after explicit clear=%+v", goResult)
	}
}

func TestEmergencyBlackoutPartialFailureKeepsPersistentGoBlock(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()
	session, startResult := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000711",
	})
	if startResult.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", startResult)
	}
	h.service.emergencySafety = func(context.Context, domain.Session, contracts.CommandEnvelope, bool) (json.RawMessage, error) {
		return json.RawMessage(`{"lighting":{"status":"FAILED"}}`), fmt.Errorf("lighting blackout unconfirmed")
	}

	result := h.service.EmergencyBlackout(ctx, EmergencyRequest{
		SessionID: session.ID, Issuer: "owner", Enabled: true,
		RequestID: "00000000-0000-7000-8000-000000000712",
	})
	if result.Status != contracts.CommandFailed || result.Error == nil || result.Error.ErrorCode != "EMERGENCY_BLACKOUT_PARTIAL_FAILURE" {
		t.Fatalf("partial emergency result=%+v", result)
	}
	latched, err := h.store.SessionManagedOutputBlackout(ctx, session.ID)
	if err != nil || !latched {
		t.Fatalf("partial failure released latch=%v err=%v", latched, err)
	}
	blocked := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000713",
	})
	if blocked.Status != contracts.CommandRejected || blocked.Error == nil || blocked.Error.ErrorCode != "EMERGENCY_BLACKOUT_ACTIVE" {
		t.Fatalf("GO after partial emergency=%+v", blocked)
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

func TestStopSessionSafetyFailureKeepsSessionActiveAndAllowsRetry(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()
	session, startResult := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000401",
	})
	if startResult.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", startResult)
	}

	h.service.stopSafety = func(_ context.Context, got domain.Session, _ contracts.CommandEnvelope) error {
		if got.ID != session.ID {
			t.Fatalf("safety Session=%s want %s", got.ID, session.ID)
		}
		state, err := h.store.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Status != domain.SessionActive {
			t.Fatalf("safety ran after Session completion: %+v", state)
		}
		return fmt.Errorf("lighting blackout was not confirmed")
	}
	failedStop := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000402",
	})
	if failedStop.Status != contracts.CommandFailed || failedStop.Error == nil || failedStop.Error.ErrorCode != "SESSION_STOP_SAFETY_FAILED" || !failedStop.Error.Retryable {
		t.Fatalf("failed stop=%+v", failedStop)
	}
	state, err := h.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != domain.SessionActive || state.EndedAt != nil {
		t.Fatalf("failed safety incorrectly completed Session: %+v", state)
	}

	h.service.stopSafety = func(context.Context, domain.Session, contracts.CommandEnvelope) error { return nil }
	retry := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000403",
	})
	if retry.Status != contracts.CommandCompleted {
		t.Fatalf("retry stop=%+v", retry)
	}
	state, err = h.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != domain.SessionCompleted || state.EndedAt == nil {
		t.Fatalf("retry did not complete Session: %+v", state)
	}
}

func TestStopSessionCancelsActiveCueBeforeCompleting(t *testing.T) {
	parameters := json.RawMessage(`{"simulation":{"behavior":"COMPLETE","delay_ms":5000}}`)
	h := newRuntimeHarness(t, parameters)
	ctx := context.Background()
	session, startResult := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000501",
	})
	if startResult.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", startResult)
	}

	goResultCh := make(chan contracts.CommandResult, 1)
	go func() {
		goResultCh <- h.service.Go(context.Background(), CueRequest{
			SessionID: session.ID, Issuer: "owner",
			RequestID: "00000000-0000-7000-8000-000000000502",
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
			t.Fatal("Cue did not enter RUNNING state before session stop")
		}
		time.Sleep(10 * time.Millisecond)
	}

	stop := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000503",
	})
	if stop.Status != contracts.CommandCompleted {
		t.Fatalf("session stop=%+v", stop)
	}
	select {
	case goResult := <-goResultCh:
		if goResult.Status != contracts.CommandCancelled {
			t.Fatalf("GO after session stop status=%s want CANCELLED: %+v", goResult.Status, goResult)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active Cue did not terminate after session stop")
	}
	state, err := h.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != domain.SessionCompleted || state.EndedAt == nil {
		t.Fatalf("session not completed after active Cue cancellation: %+v", state)
	}
}


func TestForceStopSessionBypassesFailedSafetyAndRecordsAbortedExit(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()
	session, start := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000801",
	})
	if start.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", start)
	}
	safetyCalls := 0
	h.service.stopSafety = func(context.Context, domain.Session, contracts.CommandEnvelope) error {
		safetyCalls++
		return fmt.Errorf("blackout unconfirmed")
	}
	normal := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000802",
	})
	if normal.Status != contracts.CommandFailed || normal.Error == nil || normal.Error.ErrorCode != "SESSION_STOP_SAFETY_FAILED" {
		t.Fatalf("normal stop=%+v", normal)
	}
	forced := h.service.ForceStopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000803",
	})
	if forced.Status != contracts.CommandCompleted {
		t.Fatalf("force stop=%+v", forced)
	}
	if safetyCalls != 1 {
		t.Fatalf("force stop must bypass another safety attempt, calls=%d", safetyCalls)
	}
	state, err := h.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != domain.SessionAborted || state.EndedAt == nil {
		t.Fatalf("forced exit must be ABORTED and terminal: %+v", state)
	}
	events, err := h.store.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.EventType == "rehearsal.force_stopped" {
			found = true
			var payload map[string]any
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["forced"] != true || payload["safety_bypassed"] != true {
				t.Fatalf("force event payload=%v", payload)
			}
		}
	}
	if !found {
		t.Fatalf("force-exit audit event missing: %+v", events)
	}
}

func TestProjectBlackoutWorksWithoutActiveSessionAndUsesPublishedSnapshot(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()
	var calls []bool
	h.service.emergencySafety = func(_ context.Context, session domain.Session, command contracts.CommandEnvelope, enabled bool) (json.RawMessage, error) {
		if session.ID != "" || session.ProjectID != h.project.ID || session.RuntimeSnapshotID != h.snapshot.ID {
			t.Fatalf("sessionless scope=%+v", session)
		}
		if command.Priority != "P0" || command.RuntimeSnapshotID != h.snapshot.ID {
			t.Fatalf("project blackout command=%+v", command)
		}
		calls = append(calls, enabled)
		return json.RawMessage(`{"lighting":{"status":"COMPLETED"},"tablets":{"status":"COMPLETED"},"native_visual":{"status":"NOT_CONFIGURED"}}`), nil
	}
	applied := h.service.ProjectBlackout(ctx, ProjectEmergencyRequest{
		ProjectID: h.project.ID, Issuer: "owner", Enabled: true,
		RequestID: "00000000-0000-7000-8000-000000000811",
	})
	if applied.Status != contracts.CommandCompleted {
		t.Fatalf("EDIT blackout=%+v", applied)
	}
	cleared := h.service.ProjectBlackout(ctx, ProjectEmergencyRequest{
		ProjectID: h.project.ID, Issuer: "owner", Enabled: false,
		RequestID: "00000000-0000-7000-8000-000000000812",
	})
	if cleared.Status != contracts.CommandCompleted {
		t.Fatalf("EDIT clear=%+v", cleared)
	}
	if len(calls) != 2 || !calls[0] || calls[1] {
		t.Fatalf("project blackout callbacks=%v", calls)
	}

	session, start := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000000813",
	})
	if start.Status != contracts.CommandCompleted || session.ID == "" {
		t.Fatalf("start=%+v session=%+v", start, session)
	}
	blocked := h.service.ProjectBlackout(ctx, ProjectEmergencyRequest{
		ProjectID: h.project.ID, Issuer: "owner", Enabled: true,
		RequestID: "00000000-0000-7000-8000-000000000814",
	})
	if blocked.Status != contracts.CommandRejected || blocked.Error == nil || blocked.Error.ErrorCode != "SESSION_ALREADY_ACTIVE" {
		t.Fatalf("sessionless blackout during active session=%+v", blocked)
	}
}


func TestSessionStartGateBlocksRehearsalAndShowOnStaleManagedDeviceScope(t *testing.T) {
	h := newRuntimeHarness(t)
	h.service.startGate = func(_ context.Context, projectID, snapshotID string) (bool, string, error) {
		if projectID != h.project.ID || snapshotID != h.snapshot.ID {
			t.Fatalf("start gate scope project=%s snapshot=%s", projectID, snapshotID)
		}
		return false, "Lighting Node is not synchronized to the latest Published Runtime Snapshot.", nil
	}

	for _, tc := range []struct {
		name      string
		mode      domain.SessionType
		requestID string
	}{
		{"REHEARSAL", domain.SessionRehearsal, "00000000-0000-7000-8000-000000000901"},
		{"SHOW", domain.SessionShow, "00000000-0000-7000-8000-000000000902"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, result := h.service.StartSession(context.Background(), StartRequest{
				ProjectID: h.project.ID,
				Mode: tc.mode,
				Issuer: "owner",
				RequestID: tc.requestID,
			})
			if session.ID != "" || result.Status != contracts.CommandRejected ||
				result.Error == nil || result.Error.ErrorCode != "SESSION_DEVICE_SCOPE_BLOCKED" {
				t.Fatalf("stale managed-device scope start=%+v session=%+v", result, session)
			}
			active, err := h.store.ActiveSessionForProject(context.Background(), h.project.ID)
			if err != nil {
				t.Fatal(err)
			}
			if active != nil {
				t.Fatalf("device-scope rejection created active Session: %+v", active)
			}
		})
	}
}
