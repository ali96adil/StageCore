package cueengine_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
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
