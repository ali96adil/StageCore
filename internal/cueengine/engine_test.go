package cueengine_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type fixture struct { root string; handle *db.Handle; store *store.Store; snapshot domain.RuntimeSnapshot; session domain.Session; cue domain.Cue }

func newFixture(t *testing.T, actions []domain.Action) fixture {
	t.Helper(); ctx := context.Background(); root := t.TempDir(); h, err := db.Open(ctx, db.Config{DataRoot: root}); if err != nil { t.Fatal(err) }
	s := store.New(h.DB, clock.Real{}); _, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "M1 Show", CreatedBy: "test"}); if err != nil { t.Fatal(err) }
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: "1", Name: "Cue One", OrderIndex: 0, Enabled: true}, actions); if err != nil { t.Fatal(err) }
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test"); if err != nil { t.Fatal(err) }
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "M1 test"); if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = h.Close() }); return fixture{root: root, handle: h, store: s, snapshot: runtimeSnapshot, session: session, cue: cue}
}

func action(mode, behavior string, delayMS int64, onError string) domain.Action {
	params, _ := json.Marshal(map[string]any{"simulation": map[string]any{"behavior": behavior, "delay_ms": delayMS}}); errorPolicy := json.RawMessage(`{"on_error":"FAIL_CUE"}`); if onError != "" { errorPolicy, _ = json.Marshal(map[string]any{"on_error": onError}) }
	return domain.Action{ExecutionMode: mode, TargetRef: "SIM", CapabilityKey: "sim.test", Parameters: params, TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: errorPolicy, PriorityClass: domain.PriorityP1, Enabled: true}
}

func commandFor(t *testing.T, f fixture) contracts.CommandEnvelope {
	t.Helper(); id, err := stageid.New(); if err != nil { t.Fatal(err) }; payload, _ := json.Marshal(cueengine.CueGoPayload{})
	return contracts.CommandEnvelope{CommandID: id, CommandType: cueengine.CueGoCommandType, SchemaVersion: contracts.SchemaVersion1, IssuedAt: time.Now().UTC(), ProjectID: f.snapshot.ProjectID, RuntimeSnapshotID: f.snapshot.ID, Issuer: "test.operator", Priority: "P1", Payload: payload}
}

func TestCueGoSequentialSuccessPersistsTrace(t *testing.T) {
	first := action("SEQUENTIAL", "COMPLETE", 0, ""); first.OrderIndex = 0; second := action("SEQUENTIAL", "COMPLETE", 0, ""); second.OrderIndex = 1; f := newFixture(t, []domain.Action{first, second}); ctx := context.Background()
	result := cueengine.New(f.store).ExecuteCueGo(ctx, f.session.ID, commandFor(t, f)); if result.Status != contracts.CommandCompleted { t.Fatalf("result=%#v", result) }
	events, err := f.store.ListEvents(ctx, f.session.ID); if err != nil { t.Fatal(err) }; assertEventTypes(t, events, []string{"cue.started", "action.started", "action.completed", "action.started", "action.completed", "cue.completed"})
	for i, event := range events { if event.Sequence != int64(i+1) { t.Fatalf("event sequence[%d]=%d", i, event.Sequence) }; if string(event.TraceContext) != "{}" { t.Fatalf("trace_context=%s", event.TraceContext) } }
	cues, err := f.store.ListCueExecutions(ctx, f.session.ID); if err != nil || len(cues) != 1 || cues[0].Result != domain.ExecutionCompleted { t.Fatalf("cue executions=%#v err=%v", cues, err) }
	actions, err := f.store.ListActionExecutions(ctx, cues[0].ID); if err != nil || len(actions) != 2 { t.Fatalf("action executions=%#v err=%v", actions, err) }
}

func TestCueGoFailureStopsFollowingSequentialAction(t *testing.T) {
	first := action("SEQUENTIAL", "FAIL", 0, "FAIL_CUE"); first.OrderIndex = 0; second := action("SEQUENTIAL", "COMPLETE", 0, ""); second.OrderIndex = 1; f := newFixture(t, []domain.Action{first, second})
	result := cueengine.New(f.store).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f)); if result.Status != contracts.CommandFailed { t.Fatalf("result=%#v", result) }
	cueExecutions, _ := f.store.ListCueExecutions(context.Background(), f.session.ID); actions, _ := f.store.ListActionExecutions(context.Background(), cueExecutions[0].ID); if len(actions) != 1 || actions[0].Result != domain.ExecutionFailed { t.Fatalf("actions=%#v", actions) }
	events, _ := f.store.ListEvents(context.Background(), f.session.ID); assertEventTypes(t, events, []string{"cue.started", "action.started", "action.failed", "cue.failed"})
}

func TestCueGoContinuePolicyAllowsCueCompletion(t *testing.T) {
	first := action("SEQUENTIAL", "FAIL", 0, "CONTINUE"); first.OrderIndex = 0; second := action("SEQUENTIAL", "COMPLETE", 0, ""); second.OrderIndex = 1; f := newFixture(t, []domain.Action{first, second})
	result := cueengine.New(f.store).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f)); if result.Status != contracts.CommandCompleted { t.Fatalf("result=%#v", result) }
	events, _ := f.store.ListEvents(context.Background(), f.session.ID); assertEventTypes(t, events, []string{"cue.started", "action.started", "action.failed", "action.started", "action.completed", "cue.completed"})
}

func TestCueGoTimeoutIsTruthful(t *testing.T) {
	timed := action("SEQUENTIAL", "TIMEOUT", 0, "FAIL_CUE"); timed.OrderIndex = 0; timed.TimeoutPolicy = json.RawMessage(`{"timeout_ms":15}`); f := newFixture(t, []domain.Action{timed})
	result := cueengine.New(f.store).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f)); if result.Status != contracts.CommandTimedOut { t.Fatalf("result=%#v", result) }
	events, _ := f.store.ListEvents(context.Background(), f.session.ID); assertEventTypes(t, events, []string{"cue.started", "action.started", "action.timed_out", "cue.failed"}); cueExecutions, _ := f.store.ListCueExecutions(context.Background(), f.session.ID); if cueExecutions[0].Result != domain.ExecutionTimedOut { t.Fatalf("cue result=%s", cueExecutions[0].Result) }
}

func TestDuplicateCommandAfterRestartDoesNotReplay(t *testing.T) {
	one := action("SEQUENTIAL", "COMPLETE", 0, ""); one.OrderIndex = 0; f := newFixture(t, []domain.Action{one}); ctx := context.Background(); command := commandFor(t, f)
	first := cueengine.New(f.store).ExecuteCueGo(ctx, f.session.ID, command); if first.Status != contracts.CommandCompleted { t.Fatalf("first=%#v", first) }; eventsBefore, _ := f.store.ListEvents(ctx, f.session.ID); if err := f.handle.Close(); err != nil { t.Fatal(err) }
	reopened, err := db.Open(ctx, db.Config{DataRoot: f.root}); if err != nil { t.Fatal(err) }; defer reopened.Close(); reopenedStore := store.New(reopened.DB, clock.Real{}); second := cueengine.New(reopenedStore).ExecuteCueGo(ctx, f.session.ID, command); if second.Status != contracts.CommandCompleted { t.Fatalf("second=%#v", second) }
	eventsAfter, err := reopenedStore.ListEvents(ctx, f.session.ID); if err != nil { t.Fatal(err) }; if len(eventsAfter) != len(eventsBefore) { t.Fatalf("duplicate replayed events: before=%d after=%d", len(eventsBefore), len(eventsAfter)) }; cueExecutions, _ := reopenedStore.ListCueExecutions(ctx, f.session.ID); if len(cueExecutions) != 1 { t.Fatalf("duplicate replayed cue execution: %#v", cueExecutions) }
}

func TestAcceptedDuplicateIsRejectedWithoutReplay(t *testing.T) {
	one := action("SEQUENTIAL", "COMPLETE", 0, ""); one.OrderIndex = 0; f := newFixture(t, []domain.Action{one}); command := commandFor(t, f); if _, reserved, err := f.store.ReserveCommand(context.Background(), command); err != nil || !reserved { t.Fatalf("reserve=%v err=%v", reserved, err) }
	result := cueengine.New(f.store).ExecuteCueGo(context.Background(), f.session.ID, command); if result.Status != contracts.CommandRejected || result.Error == nil || result.Error.ErrorCode != "DUPLICATE_UNRESOLVED" { t.Fatalf("result=%#v", result) }; events, _ := f.store.ListEvents(context.Background(), f.session.ID); if len(events) != 0 { t.Fatalf("unresolved duplicate emitted events: %#v", events) }
}

func TestParallelDoesNotBlockFollowingSequentialUntilCueEnd(t *testing.T) {
	parallel := action("PARALLEL", "COMPLETE", 60, ""); parallel.OrderIndex = 0; sequential := action("SEQUENTIAL", "COMPLETE", 0, ""); sequential.OrderIndex = 1; f := newFixture(t, []domain.Action{parallel, sequential})
	result := cueengine.New(f.store).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f)); if result.Status != contracts.CommandCompleted { t.Fatalf("result=%#v", result) }; events, _ := f.store.ListEvents(context.Background(), f.session.ID)
	parallelCompleted := eventIndexForAction(events, "action.completed", f.cue.Actions[0].ID); secondStarted := eventIndexForAction(events, "action.started", f.cue.Actions[1].ID); if secondStarted < 0 || parallelCompleted < 0 || secondStarted > parallelCompleted { t.Fatalf("parallel action blocked sequential start: events=%v", eventTypes(events)) }
}

func TestParallelBarrierWaitsBeforeFollowingSequential(t *testing.T) {
	parallel := action("PARALLEL", "COMPLETE", 60, ""); parallel.OrderIndex = 0; barrier := action("PARALLEL_BARRIER", "COMPLETE", 20, ""); barrier.OrderIndex = 1; sequential := action("SEQUENTIAL", "COMPLETE", 0, ""); sequential.OrderIndex = 2; f := newFixture(t, []domain.Action{parallel, barrier, sequential})
	result := cueengine.New(f.store).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f)); if result.Status != contracts.CommandCompleted { t.Fatalf("result=%#v", result) }; events, _ := f.store.ListEvents(context.Background(), f.session.ID)
	thirdStarted := eventIndexForAction(events, "action.started", f.cue.Actions[2].ID); firstCompleted := eventIndexForAction(events, "action.completed", f.cue.Actions[0].ID); if thirdStarted < 0 || firstCompleted < 0 || thirdStarted < firstCompleted { t.Fatalf("barrier did not wait: events=%v", eventTypes(events)) }
}

func assertEventTypes(t *testing.T, events []contracts.EventEnvelope, want []string) { t.Helper(); got := eventTypes(events); if len(got) != len(want) { t.Fatalf("event types=%v want=%v", got, want) }; for i := range want { if got[i] != want[i] { t.Fatalf("event types=%v want=%v", got, want) } } }
func eventTypes(events []contracts.EventEnvelope) []string { out := make([]string, len(events)); for i := range events { out[i] = events[i].EventType }; return out }
func eventIndexForAction(events []contracts.EventEnvelope, eventType, actionID string) int { for i, event := range events { if event.EventType != eventType { continue }; var payload struct { ActionID string `json:"action_id"` }; if err := json.Unmarshal(event.Payload, &payload); err == nil && payload.ActionID == actionID { return i } }; return -1 }
