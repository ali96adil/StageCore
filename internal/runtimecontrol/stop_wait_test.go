package runtimecontrol

import (
    "context"
    "errors"
    "strings"
    "testing"
    "time"
)

func TestWaitForActiveRunStopsKeepsDeadlineAcrossSeveralCues(t *testing.T) {
    finished := make(chan struct{})
    close(finished)
    unfinished := make(chan struct{})
    runs := []activeRun{
        {done: finished},
        {done: unfinished},
        {done: finished},
    }

    started := time.Now()
    err := waitForActiveRunStops(context.Background(), runs, 35*time.Millisecond)
    elapsed := time.Since(started)
    if err == nil || !strings.Contains(err.Error(), "3 active Cue execution(s)") {
        t.Fatalf("missing bounded shared stop failure: %v", err)
    }
    if elapsed > time.Second {
        t.Fatalf("stop wait ran beyond shared deadline: %s", elapsed)
    }
}

func TestWaitForActiveRunStopsHonorsParentCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    err := waitForActiveRunStops(ctx, []activeRun{{done: make(chan struct{})}}, time.Minute)
    if !errors.Is(err, context.Canceled) {
        t.Fatalf("expected cancellation rather than timeout or indefinite wait: %v", err)
    }
}

func TestWaitForActiveRunStopsAllFinished(t *testing.T) {
    done := make(chan struct{})
    close(done)
    if err := waitForActiveRunStops(context.Background(), []activeRun{
        {done: done}, {done: done}, {done: done},
    }, 25*time.Millisecond); err != nil {
        t.Fatalf("completed Cue batch should not report stop failure: %v", err)
    }
}
