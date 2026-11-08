package cueengine_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestActionDispatchRetryRequiresProofAndKeepsExecutionID(t *testing.T) {
	for _, tc := range []struct {
		name     string
		first    capability.Result
		attempts int
		status   contracts.CommandStatus
	}{
		{"offline before send", capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, DispatchNotAttempted: true}, 2, contracts.CommandCompleted},
		{"ambiguous disconnect", capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone}, 1, contracts.CommandFailed},
		{"transport receipt only", capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckTransportOnly}, 1, contracts.CommandFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := action("SEQUENTIAL", "COMPLETE", 0, "")
			a.ErrorPolicy = json.RawMessage(`{"on_error":"FAIL_CUE","retry_before_dispatch":true,"require_application_ack":true}`)
			f := newFixture(t, []domain.Action{a})
			var ids []string
			executor := capability.ExecutorFunc(func(_ context.Context, req capability.Request) capability.Result {
				ids = append(ids, req.ExecutionID)
				if len(ids) == 1 {
					return tc.first
				}
				return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckApplication}
			})
			engine := cueengine.NewWithExecutor(f.store, executor)
			command := commandFor(t, f)
			result := engine.ExecuteCueGo(context.Background(), f.session.ID, command)
			if result.Status != tc.status || len(ids) != tc.attempts {
				t.Fatalf("result=%+v attempts=%v", result, ids)
			}
			if len(ids) == 2 && (ids[0] == "" || ids[0] != ids[1]) {
				t.Fatalf("retry changed execution ID: %v", ids)
			}
			engine.ExecuteCueGo(context.Background(), f.session.ID, command)
			if len(ids) != tc.attempts {
				t.Fatalf("duplicate GO dispatched again: %v", ids)
			}
		})
	}
}

func TestActionDelayOrdersSequentialDispatchAndLeavesTimeoutBudget(t *testing.T) {
	first := action("SEQUENTIAL", "COMPLETE", 0, "")
	first.TargetRef = "first"
	first.TimeoutPolicy = json.RawMessage(`{"start_delay_ms":80,"timeout_ms":50}`)
	second := action("SEQUENTIAL", "COMPLETE", 0, "")
	second.TargetRef = "second"
	second.OrderIndex = 1
	f := newFixture(t, []domain.Action{first, second})
	started := time.Now()
	var attempts int
	executor := capability.ExecutorFunc(func(ctx context.Context, req capability.Request) capability.Result {
		attempts++
		if attempts == 1 && (time.Since(started) < 80*time.Millisecond || ctx.Err() != nil || req.TimeoutMS != 50) {
			t.Errorf("first dispatch did not wait or timeout was consumed: elapsed=%s err=%v timeout=%d", time.Since(started), ctx.Err(), req.TimeoutMS)
		}
		if attempts == 2 && req.TimeoutMS != 0 {
			t.Errorf("second Action dispatched out of order")
		}
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
	})
	result := cueengine.NewWithExecutor(f.store, executor).ExecuteCueGo(context.Background(), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandCompleted || attempts != 2 {
		t.Fatalf("result=%+v attempts=%d", result, attempts)
	}
}

func TestStopCancelsDelayedParallelActionWithoutBlockingSequential(t *testing.T) {
	first := action("PARALLEL", "COMPLETE", 0, "")
	first.TimeoutPolicy = json.RawMessage(`{"start_delay_ms":600000}`)
	second := action("SEQUENTIAL", "COMPLETE", 0, "")
	second.OrderIndex = 1
	f := newFixture(t, []domain.Action{first, second})
	stop := make(chan struct{})
	dispatched := make(chan int64, 2)
	var stopOnce sync.Once
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	executor := capability.ExecutorFunc(func(_ context.Context, req capability.Request) capability.Result {
		dispatched <- req.TimeoutMS
		stopOnce.Do(func() { close(stop) })
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
	})
	result := cueengine.NewWithExecutor(f.store, executor).ExecuteCueGo(cueengine.WithDelayStop(ctx, stop), f.session.ID, commandFor(t, f))
	if result.Status != contracts.CommandCancelled || len(dispatched) != 1 {
		t.Fatalf("result=%+v dispatches=%d", result, len(dispatched))
	}
}
