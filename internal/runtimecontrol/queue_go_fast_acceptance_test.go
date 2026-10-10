package runtimecontrol

import (
    "context"
    "encoding/json"
    "testing"
    "time"

    "github.com/ali96adil/StageCore/internal/contracts"
    "github.com/ali96adil/StageCore/internal/domain"
)

// The Companion /stagecore/go handler must be able to accept Cue 2 before
// Cue 1's intentional 5-second delay ends. Acceptance means persisted
// selection, not successful physical Action completion.
func TestQueueGoAcceptsBeforePreviousCueCompletes(t *testing.T) {
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
        RequestID: "00000000-0000-7000-8000-000000009201",
    })
    if started.Status != contracts.CommandCompleted { t.Fatalf("start: %+v", started) }
    start := time.Now()
    first := h.service.QueueGo(ctx, CueRequest{
        SessionID: session.ID, Issuer: "companion.local_osc:test",
        RequestID: "00000000-0000-7000-8000-000000009202",
    })
    if first.Status != contracts.CommandAccepted { t.Fatalf("first fast accept: %+v", first) }
    if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
        t.Fatalf("QueueGo held the local OSC handler until Cue completion: %s", elapsed)
    }
    firstCue := h.cues[0].ID
    second := h.service.QueueGo(ctx, CueRequest{
        SessionID: session.ID, Issuer: "companion.local_osc:test",
        RequestID: "00000000-0000-7000-8000-000000009203",
        ExpectedCurrentCueID: &firstCue,
    })
    if second.Status != contracts.CommandAccepted && second.Status != contracts.CommandCompleted {
        t.Fatalf("second Cue not accepted while first runs: %+v", second)
    }
    waitForCueAdvancement(t, h, session.ID, h.cues[1].ID)
    stopped := h.service.StopSession(ctx, StopRequest{
        SessionID: session.ID, Issuer: "owner",
        RequestID: "00000000-0000-7000-8000-000000009204",
    })
    if stopped.Status != contracts.CommandCompleted { t.Fatalf("STOP: %+v", stopped) }
}
