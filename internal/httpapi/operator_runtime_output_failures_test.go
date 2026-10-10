package httpapi

import (
    "testing"

    "github.com/ali96adil/StageCore/internal/domain"
)

// In fail-soft mode a Cue can be COMPLETED while an output Action failed.
// A later successful Cue must not erase that failure from the operator view.
func TestRecentOutputFailuresSurviveLaterSuccessfulCue(t *testing.T) {
    older := domain.CueExecution{ID: "exec-older", CueID: "cue-older", Result: domain.ExecutionCompleted}
    later := domain.CueExecution{ID: "exec-later", CueID: "cue-later", Result: domain.ExecutionCompleted}
    code := "OSC_OUTPUT_UNAVAILABLE"
    got := appendRecentOutputFailures(nil, later, []domain.ActionExecution{
        {ActionID: "good", Result: domain.ExecutionCompleted},
    }, 8)
    got = appendRecentOutputFailures(got, older, []domain.ActionExecution{
        {ActionID: "bad", Result: domain.ExecutionFailed, ErrorCode: &code, ResponseSummary: "VDMX offline"},
        {ActionID: "good-earlier", Result: domain.ExecutionCompleted},
    }, 8)
    if len(got) != 1 {
        t.Fatalf("failure count=%d, want 1", len(got))
    }
    if got[0].CueExecutionID != older.ID || got[0].CueID != older.CueID || got[0].ActionID != "bad" || got[0].ErrorCode != code {
        t.Fatalf("failed output details lost: %+v", got[0])
    }
}

func TestRecentOutputFailuresBoundsAndTimeouts(t *testing.T) {
    cue := domain.CueExecution{ID: "exec-1", CueID: "cue-1"}
    actions := []domain.ActionExecution{
        {ActionID: "failure", Result: domain.ExecutionFailed},
        {ActionID: "timeout", Result: domain.ExecutionTimedOut},
        {ActionID: "cancelled-by-stop", Result: domain.ExecutionCancelled},
        {ActionID: "running", Result: domain.ExecutionRunning},
    }
    got := appendRecentOutputFailures(nil, cue, actions, 1)
    if len(got) != 1 || got[0].ActionID != "timeout" {
        t.Fatalf("expected newest timeout only, got %+v", got)
    }
    got = appendRecentOutputFailures(nil, cue, actions, 8)
    if len(got) != 2 || got[0].ActionID != "timeout" || got[1].ActionID != "failure" {
        t.Fatalf("only failed/timed-out Actions belong in failure view, got %+v", got)
    }
}
