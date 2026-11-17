package cueengine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueGoRejectsSnapshotMismatchWithoutExecution(t *testing.T) {
	one := action("SEQUENTIAL", "COMPLETE", 0, "")
	one.OrderIndex = 0
	f := newFixture(t, []domain.Action{one})
	ctx := context.Background()

	otherSnapshot, _, err := snapshot.NewBuilder(f.store).Create(ctx, f.snapshot.RevisionID, "test")
	if err != nil {
		t.Fatal(err)
	}
	command := commandFor(t, f)
	command.RuntimeSnapshotID = otherSnapshot.ID

	result := cueengine.New(f.store).ExecuteCueGo(ctx, f.session.ID, command)
	if result.Status != contracts.CommandRejected || result.Error == nil || result.Error.ErrorCode != "SNAPSHOT_MISMATCH" {
		t.Fatalf("result=%#v", result)
	}
	events, err := f.store.ListEvents(ctx, f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("snapshot mismatch emitted runtime events: %#v", events)
	}
	cueExecutions, err := f.store.ListCueExecutions(ctx, f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cueExecutions) != 0 {
		t.Fatalf("snapshot mismatch created execution: %#v", cueExecutions)
	}
}

func TestCueGoRequiresStateConfirmationForSelectedRehearsalStart(t *testing.T) {
	one := action("SEQUENTIAL", "COMPLETE", 0, "")
	one.OrderIndex = 0
	f := newFixture(t, []domain.Action{one})
	ctx := context.Background()
	if err := f.store.EndSessionLifecycle(ctx, f.session.ID, domain.SessionLifecycleCompleted, "replace fixture session"); err != nil {
		t.Fatal(err)
	}
	cueID := f.cue.ID
	selected, err := f.store.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: f.snapshot.ID, SessionType: domain.SessionRehearsal, Name: "selected rehearsal cue",
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartCue, CueID: &cueID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !selected.StateTruth.ManualConfirmationRequired {
		t.Fatalf("selected Cue start lacks its state-confirmation gate: %+v", selected.StateTruth)
	}
	dispatches := 0
	executor := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		dispatches++
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
	})
	engine := cueengine.NewWithExecutor(f.store, executor)
	command := commandFor(t, f)
	result := engine.ExecuteCueGo(ctx, selected.ID, command)
	if result.Status != contracts.CommandRejected || result.Error == nil || result.Error.ErrorCode != "SESSION_START_CONFIRMATION_REQUIRED" {
		t.Fatalf("selected Cue GO did not require state confirmation: %+v", result)
	}
	if dispatches != 0 {
		t.Fatalf("unconfirmed selected Cue dispatched %d Action(s)", dispatches)
	}
	duplicate := engine.ExecuteCueGo(ctx, selected.ID, command)
	if duplicate.Status != result.Status || duplicate.Error == nil || duplicate.Error.ErrorCode != result.Error.ErrorCode || dispatches != 0 {
		t.Fatalf("duplicate unconfirmed GO changed outcome or dispatched: first=%+v duplicate=%+v dispatches=%d", result, duplicate, dispatches)
	}
	// Keep the restoration marker fail-closed even if the redundant boolean is
	// cleared by a corrupt or partially migrated row.
	if _, err := f.handle.DB.ExecContext(ctx, `UPDATE sessions SET manual_confirmation_required = 0 WHERE session_id = ?`, selected.ID); err != nil {
		t.Fatal(err)
	}
	markerOnly := engine.ExecuteCueGo(ctx, selected.ID, commandFor(t, f))
	if markerOnly.Status != contracts.CommandRejected || markerOnly.Error == nil || markerOnly.Error.ErrorCode != "SESSION_START_CONFIRMATION_REQUIRED" || dispatches != 0 {
		t.Fatalf("manual-confirmation restoration marker was not independently enforced: result=%+v dispatches=%d", markerOnly, dispatches)
	}
	events, err := f.store.ListEvents(ctx, selected.ID)
	if err != nil || len(events) != 0 {
		t.Fatalf("unconfirmed selected Cue emitted runtime events: %+v err=%v", events, err)
	}
	executions, err := f.store.ListCueExecutions(ctx, selected.ID)
	if err != nil || len(executions) != 0 {
		t.Fatalf("unconfirmed selected Cue created executions: %+v err=%v", executions, err)
	}
}

func TestCueGoRejectsExpectedCurrentCueMismatch(t *testing.T) {
	one := action("SEQUENTIAL", "COMPLETE", 0, "")
	one.OrderIndex = 0
	f := newFixture(t, []domain.Action{one})
	ctx := context.Background()

	wrongCurrent, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(cueengine.CueGoPayload{ExpectedCurrentCueID: &wrongCurrent})
	if err != nil {
		t.Fatal(err)
	}
	command := commandFor(t, f)
	command.Payload = payload

	result := cueengine.New(f.store).ExecuteCueGo(ctx, f.session.ID, command)
	if result.Status != contracts.CommandRejected || result.Error == nil || result.Error.ErrorCode != "CURRENT_CUE_MISMATCH" {
		t.Fatalf("result=%#v", result)
	}
	events, err := f.store.ListEvents(ctx, f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("current-cue mismatch emitted runtime events: %#v", events)
	}
}

func TestCueExecutionUsesSnapshotCapturedActionNotLiveDefinition(t *testing.T) {
	one := action("SEQUENTIAL", "COMPLETE", 0, "")
	one.OrderIndex = 0
	f := newFixture(t, []domain.Action{one})
	ctx := context.Background()

	// This direct SQL mutation deliberately bypasses the product Store API.
	// It proves the runtime engine consumes the immutable snapshot manifest rather
	// than rereading a live definition at GO time.
	liveFailure := `{"simulation":{"behavior":"FAIL","delay_ms":0}}`
	if _, err := f.handle.DB.ExecContext(ctx, `UPDATE actions SET parameters_json = ? WHERE action_id = ?`, liveFailure, f.cue.Actions[0].ID); err != nil {
		t.Fatal(err)
	}

	result := cueengine.New(f.store).ExecuteCueGo(ctx, f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("snapshot-captured action was not used: %#v", result)
	}
	events, err := f.store.ListEvents(ctx, f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEventTypes(t, events, []string{"cue.started", "action.started", "action.completed", "cue.completed"})
}
