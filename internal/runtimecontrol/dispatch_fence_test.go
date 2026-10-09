package runtimecontrol

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type dispatchProbe struct {
	calls atomic.Int32
	entered chan struct{}
}

func (p *dispatchProbe) Execute(ctx context.Context, _ capability.Request) capability.Result {
	p.calls.Add(1)
	if p.entered != nil {
		select { case p.entered <- struct{}{}: default: }
		<-ctx.Done()
		return capability.Result{
			Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone,
			ErrorCode: "CANCELLED",
		}
	}
	return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckNone}
}

func TestStoppableExecutorNeverDispatchesActionAfterStop(t *testing.T) {
	probe := &dispatchProbe{}
	executor := newStoppableExecutor(probe)
	executor.stop("cue-1")
	for i := 0; i < 12; i++ {
		result := executor.Execute(context.Background(), capability.Request{
			CorrelationID: "cue-1", ExecutionID: "action-late",
		})
		if result.Result != domain.ExecutionCancelled || result.ErrorCode != "CANCELLED" {
			t.Fatalf("late action must cancel without dispatch: %+v", result)
		}
	}
	if calls := probe.calls.Load(); calls != 0 {
		t.Fatalf("device adapter was invoked %d times after STOP", calls)
	}
	// An independent Cue must not inherit the stopped correlation.
	other := executor.Execute(context.Background(), capability.Request{
		CorrelationID: "cue-2", ExecutionID: "action-independent",
	})
	if other.Result != domain.ExecutionCompleted || probe.calls.Load() != 1 {
		t.Fatalf("independent Cue was incorrectly stopped: %+v calls=%d", other, probe.calls.Load())
	}
}

func TestStoppableExecutorCancelsAlreadyRunningAction(t *testing.T) {
	probe := &dispatchProbe{entered: make(chan struct{}, 1)}
	executor := newStoppableExecutor(probe)
	results := make(chan capability.Result, 1)
	go func() {
		results <- executor.Execute(context.Background(), capability.Request{
			CorrelationID: "cue-3", ExecutionID: "action-running",
		})
	}()
	select {
	case <-probe.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("adapter never entered")
	}
	executor.stop("cue-3")
	select {
	case result := <-results:
		if result.Result != domain.ExecutionCancelled {
			t.Fatalf("already-running adapter must observe cancellation: %+v", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("STOP did not cancel in-flight adapter")
	}
	late := executor.Execute(context.Background(), capability.Request{
		CorrelationID: "cue-3", ExecutionID: "action-late",
	})
	if late.Result != domain.ExecutionCancelled || probe.calls.Load() != 1 {
		t.Fatalf("late action dispatched despite stop: %+v calls=%d", late, probe.calls.Load())
	}
}
