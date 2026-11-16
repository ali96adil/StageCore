package runtimecontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/httpaction"
)

type dispatchProbe struct {
	calls   atomic.Int32
	entered chan struct{}
}

func (p *dispatchProbe) Execute(ctx context.Context, _ capability.Request) capability.Result {
	p.calls.Add(1)
	if p.entered != nil {
		select {
		case p.entered <- struct{}{}:
		default:
		}
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

func TestDispatchAuthorizationIsRecheckedForEveryAction(t *testing.T) {
	probe := &dispatchProbe{}
	executor := newStoppableExecutor(probe)
	var revoked atomic.Bool
	ctx := WithDispatchAuthorization(context.Background(), func(_ context.Context, req capability.Request) error {
		if revoked.Load() || req.SessionID != "rehearsal" {
			return context.Canceled
		}
		return nil
	})
	request := capability.Request{CorrelationID: "cue", ExecutionID: "first", SessionID: "rehearsal"}
	if got := executor.Execute(ctx, request); got.Result != domain.ExecutionCompleted {
		t.Fatalf("first: %+v", got)
	}
	revoked.Store(true)
	request.ExecutionID = "delayed"
	if got := executor.Execute(ctx, request); got.ErrorCode != "PROJECT_DISPATCH_AUTHORITY_REVOKED" || got.AckLevel != contracts.AckNone {
		t.Fatalf("revoked: %+v", got)
	}
	if probe.calls.Load() != 1 {
		t.Fatalf("revoked action reached adapter: %d", probe.calls.Load())
	}
}

func TestHTTPActionRechecksDispatchAuthorityBeforeNetworkRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	target, err := json.Marshal(map[string]string{"url": server.URL})
	if err != nil {
		t.Fatal(err)
	}
	var checks atomic.Int32
	ctx := WithDispatchAuthorization(context.Background(), func(context.Context, capability.Request) error {
		if checks.Add(1) == 1 {
			return nil // Runtime Control admits the Action.
		}
		return capability.ErrDispatchAuthorityRevoked // authority ends during adapter preparation.
	})
	executor := newStoppableExecutor(httpaction.NewWithClient(server.Client()))
	result := executor.Execute(ctx, capability.Request{
		ExecutionID: "http-action-revoked-before-write", ProjectID: "project-a",
		SessionID: "show-a", RuntimeSnapshotID: "snapshot-a",
		Capability: httpaction.CapabilityKey,
		Target: &capability.Target{
			Ref: "HTTP-OUT", LogicalType: "http", Configuration: target,
		},
		Parameters: json.RawMessage(`{"method":"POST"}`), TimeoutMS: 1000,
	})
	if checks.Load() != 2 {
		t.Fatalf("dispatch authorization checks=%d, want admission plus transport-boundary recheck", checks.Load())
	}
	if result.ErrorCode != capability.DispatchAuthorityRevokedCode ||
		result.Result != domain.ExecutionFailed || result.AckLevel != contracts.AckNone ||
		!result.DispatchNotAttempted || requests.Load() != 0 {
		t.Fatalf("revoked HTTP Action reached network or returned an unsafe result: result=%+v requests=%d", result, requests.Load())
	}
}

func TestHTTPActionRechecksDispatchAuthorityBeforeRedirectFollowup(t *testing.T) {
	var redirectedRequests atomic.Int32
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer redirected.Close()
	var initialRequests atomic.Int32
	initial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initialRequests.Add(1)
		http.Redirect(w, r, redirected.URL+"/followup", http.StatusTemporaryRedirect)
	}))
	defer initial.Close()

	target, err := json.Marshal(map[string]string{"url": initial.URL})
	if err != nil {
		t.Fatal(err)
	}
	var checks atomic.Int32
	ctx := WithDispatchAuthorization(context.Background(), func(context.Context, capability.Request) error {
		if checks.Add(1) <= 2 {
			return nil // Runtime Control and the initial HTTP request are authorized.
		}
		return capability.ErrDispatchAuthorityRevoked
	})
	executor := newStoppableExecutor(httpaction.NewWithClient(initial.Client()))
	result := executor.Execute(ctx, capability.Request{
		ExecutionID: "http-action-revoked-before-redirect", ProjectID: "project-a",
		SessionID: "show-a", RuntimeSnapshotID: "snapshot-a",
		Capability: httpaction.CapabilityKey,
		Target: &capability.Target{
			Ref: "HTTP-OUT", LogicalType: "http", Configuration: target,
		},
		Parameters: json.RawMessage(`{"method":"POST"}`), TimeoutMS: 1000,
	})
	if checks.Load() != 3 || initialRequests.Load() != 1 || redirectedRequests.Load() != 0 {
		t.Fatalf("redirect dispatch crossed revoked authority: checks=%d initial=%d redirected=%d", checks.Load(), initialRequests.Load(), redirectedRequests.Load())
	}
	if result.ErrorCode != capability.DispatchAuthorityRevokedCode || result.Result != domain.ExecutionFailed ||
		result.AckLevel != contracts.AckNone || result.DispatchNotAttempted {
		t.Fatalf("redirect revocation did not preserve the already-attempted first request: %+v", result)
	}
}
