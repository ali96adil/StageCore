package runtimecontrol

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func waitForCueAdvancement(t *testing.T, h *runtimeHarness, sessionID, wantCueID string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(5 * time.Second)
	for {
		session, err := h.store.GetSession(ctx, sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if session.CurrentCueID != nil && *session.CurrentCueID == wantCueID {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Cue %s was not selected/advanced", wantCueID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNextGoOverlapsPendingDelayWithoutCancellingFirstCue(t *testing.T) {
	h := newRuntimeHarnessWithPolicies(t,
		[]json.RawMessage{
			json.RawMessage(`{"start_delay_ms":5000}`),
			json.RawMessage(`{}`),
		},
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
	)
	ctx := context.Background()
	session, started := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009001",
	})
	if started.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", started)
	}
	firstDone := make(chan contracts.CommandResult, 1)
	go func() {
		firstDone <- h.service.Go(ctx, CueRequest{
			SessionID: session.ID, Issuer: "owner",
			RequestID: "00000000-0000-7000-8000-000000009002",
		})
	}()
	waitForCueAdvancement(t, h, session.ID, h.cues[0].ID)

	begin := time.Now()
	expected := h.cues[0].ID
	second := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009003",
		ExpectedCurrentCueID: &expected,
	})
	if second.Status != contracts.CommandCompleted {
		t.Fatalf("second GO while first is delayed=%+v", second)
	}
	if elapsed := time.Since(begin); elapsed > 2500*time.Millisecond {
		t.Fatalf("second GO waited on first 5s delay: %s", elapsed)
	}
	waitForCueAdvancement(t, h, session.ID, h.cues[1].ID)

	select {
	case r := <-firstDone:
		t.Fatalf("first 5s-delay Cue terminated after second GO: %+v", r)
	default:
	}
	stop := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009004",
	})
	if stop.Status != contracts.CommandCompleted {
		t.Fatalf("STOP overlapping Cue=%+v", stop)
	}
	select {
	case r := <-firstDone:
		if r.Status != contracts.CommandCancelled {
			t.Fatalf("stopped delayed Cue=%+v, want CANCELLED", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first delayed Cue did not cancel on STOP")
	}
	rows, err := h.store.ListCueExecutions(ctx, session.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("expected two persisted Cue executions: %+v err=%v", rows, err)
	}
	statusByCue := map[string]domain.ExecutionResult{}
	for _, row := range rows {
		statusByCue[row.CueID] = row.Result
	}
	if statusByCue[h.cues[0].ID] != domain.ExecutionCancelled ||
		statusByCue[h.cues[1].ID] != domain.ExecutionCompleted {
		t.Fatalf("second Cue should complete independent of cancelled first: %+v", statusByCue)
	}
}

func TestStopSessionCancelsEveryOverlappingDelayedCue(t *testing.T) {
	h := newRuntimeHarnessWithPolicies(t,
		[]json.RawMessage{
			json.RawMessage(`{"start_delay_ms":5000}`),
			json.RawMessage(`{"start_delay_ms":5000}`),
		},
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
	)
	ctx := context.Background()
	session, started := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009011",
	})
	if started.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", started)
	}
	done := make(chan contracts.CommandResult, 2)
	go func() {
		done <- h.service.Go(ctx, CueRequest{
			SessionID: session.ID, Issuer: "owner",
			RequestID: "00000000-0000-7000-8000-000000009012",
		})
	}()
	waitForCueAdvancement(t, h, session.ID, h.cues[0].ID)
	go func() {
		done <- h.service.Go(ctx, CueRequest{
			SessionID: session.ID, Issuer: "owner",
			RequestID: "00000000-0000-7000-8000-000000009013",
		})
	}()
	waitForCueAdvancement(t, h, session.ID, h.cues[1].ID)
	stop := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009014",
	})
	if stop.Status != contracts.CommandCompleted {
		t.Fatalf("STOP SESSION all overlapping Cues=%+v", stop)
	}
	for range 2 {
		select {
		case r := <-done:
			if r.Status != contracts.CommandCancelled {
				t.Fatalf("GO after STOP=%+v, want CANCELLED", r)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("not all delayed Cues terminated after STOP")
		}
	}
	rows, err := h.store.ListCueExecutions(ctx, session.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("expected both stopped Cue executions: %+v err=%v", rows, err)
	}
	for _, row := range rows {
		if row.Result != domain.ExecutionCancelled || row.CompletedAt == nil {
			t.Fatalf("Cue not cancelled and persisted: %+v", row)
		}
	}
}

func TestSimultaneousGoRequestsSelectDistinctCues(t *testing.T) {
	h := newRuntimeHarnessWithPolicies(t,
		[]json.RawMessage{
			json.RawMessage(`{"start_delay_ms":5000}`),
			json.RawMessage(`{"start_delay_ms":5000}`),
			json.RawMessage(`{"start_delay_ms":5000}`),
		},
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
	)
	ctx := context.Background()
	session, started := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009021",
	})
	if started.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", started)
	}
	results := make(chan contracts.CommandResult, 3)
	start := make(chan struct{})
	for _, id := range []string{
		"00000000-0000-7000-8000-000000009022",
		"00000000-0000-7000-8000-000000009023",
		"00000000-0000-7000-8000-000000009024",
	} {
		requestID := id
		go func() {
			<-start
			results <- h.service.Go(ctx, CueRequest{SessionID: session.ID, Issuer: "owner", RequestID: requestID})
		}()
	}
	close(start)
	waitForCueAdvancement(t, h, session.ID, h.cues[2].ID)
	if stop := h.service.StopSession(ctx, StopRequest{
		SessionID: session.ID, Issuer: "owner", RequestID: "00000000-0000-7000-8000-000000009025",
	}); stop.Status != contracts.CommandCompleted {
		t.Fatalf("STOP SESSION simultaneous Cues=%+v", stop)
	}
	for range 3 {
		select {
		case result := <-results:
			if result.Status != contracts.CommandCancelled {
				t.Fatalf("simultaneous GO returned %+v; expected cancelled delay", result)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent GO did not terminate")
		}
	}
	executions, err := h.store.ListCueExecutions(ctx, session.ID)
	if err != nil || len(executions) != 3 {
		t.Fatalf("simultaneous GO must persist exactly three executions: %+v err=%v", executions, err)
	}
	seen := make(map[string]bool)
	for _, execution := range executions {
		if seen[execution.CueID] || execution.Result != domain.ExecutionCancelled {
			t.Fatalf("Cue selected twice or failed to cancel: %+v", executions)
		}
		seen[execution.CueID] = true
	}
	for _, cue := range h.cues {
		if !seen[cue.ID] {
			t.Fatalf("missing distinct Cue: %s", cue.ID)
		}
	}
}

func TestLateOlderCueCompletionNeverRewindsSessionCursor(t *testing.T) {
	h := newRuntimeHarnessWithPolicies(t,
		[]json.RawMessage{
			json.RawMessage(`{"start_delay_ms":2500}`),
			json.RawMessage(`{}`),
			json.RawMessage(`{}`),
		},
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
		json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
	)
	ctx := context.Background()
	session, started := h.service.StartSession(ctx, StartRequest{
		ProjectID: h.project.ID, Mode: domain.SessionRehearsal, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009031",
	})
	if started.Status != contracts.CommandCompleted {
		t.Fatalf("start=%+v", started)
	}
	firstDone := make(chan contracts.CommandResult, 1)
	go func() {
		firstDone <- h.service.Go(ctx, CueRequest{
			SessionID: session.ID, Issuer: "owner",
			RequestID: "00000000-0000-7000-8000-000000009032",
		})
	}()
	waitForCueAdvancement(t, h, session.ID, h.cues[0].ID)

	second := h.service.Go(ctx, CueRequest{
		SessionID: session.ID, Issuer: "owner",
		RequestID: "00000000-0000-7000-8000-000000009033",
	})
	if second.Status != contracts.CommandCompleted {
		t.Fatalf("second Cue did not finish before delayed first Cue: %+v", second)
	}
	select {
	case premature := <-firstDone:
		t.Fatalf("first Cue should still be pending when second finishes: %+v", premature)
	default:
	}
	waitForCueAdvancement(t, h, session.ID, h.cues[1].ID)

	select {
	case result := <-firstDone:
		if result.Status != contracts.CommandCompleted {
			t.Fatalf("late first Cue=%+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first delayed Cue did not complete")
	}
	state, err := h.store.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentCueID == nil || *state.CurrentCueID != h.cues[1].ID {
		t.Fatalf("late Cue 1 completion rewound session current position: %+v", state.CurrentCueID)
	}
	if state.NextCueID == nil || *state.NextCueID != h.cues[2].ID {
		t.Fatalf("late Cue 1 completion rewound session next Cue: %+v", state.NextCueID)
	}
}
