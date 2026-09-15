package dispatchauthority

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type sourceFunc func(context.Context) (Grant, error)

func (f sourceFunc) Current(ctx context.Context) (Grant, error) {
	return f(ctx)
}

func TestGateStandaloneDelegatesExactRequest(t *testing.T) {
	var calls atomic.Int32
	backend := capability.ExecutorFunc(func(_ context.Context, req capability.Request) capability.Result {
		calls.Add(1)
		if req.ExecutionID != "execution-1" {
			t.Fatalf("execution id=%q", req.ExecutionID)
		}
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckVerifiedState, ResponseSummary: "ok"}
	})

	result := NewStandalone(backend).Execute(context.Background(), capability.Request{ExecutionID: "execution-1"})
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckVerifiedState || result.ResponseSummary != "ok" {
		t.Fatalf("result=%#v", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("backend calls=%d, want 1", calls.Load())
	}
}

func TestGateStandbyFailsClosedBeforeBackend(t *testing.T) {
	var calls atomic.Int32
	backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		calls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted}
	})
	gate := New(backend, sourceFunc(func(context.Context) (Grant, error) {
		return Grant{Mode: ModeStandby}, nil
	}), "hub-a")

	result := gate.Execute(context.Background(), capability.Request{ExecutionID: "execution-standby"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "DISPATCH_AUTHORITY_STANDBY" || result.AckLevel != contracts.AckNone {
		t.Fatalf("result=%#v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("backend calls=%d, want 0", calls.Load())
	}
}

func TestGateLeaderRequiresMatchingGrantedEpochAndToken(t *testing.T) {
	tests := []struct {
		name     string
		grant    Grant
		holderID string
		allowed  bool
	}{
		{name: "valid", grant: Grant{Mode: ModeLeader, HolderID: "hub-a", Epoch: 7, Token: "opaque-fence-7", Granted: true}, holderID: "hub-a", allowed: true},
		{name: "not granted", grant: Grant{Mode: ModeLeader, HolderID: "hub-a", Epoch: 7, Token: "opaque-fence-7", Granted: false}, holderID: "hub-a"},
		{name: "wrong holder", grant: Grant{Mode: ModeLeader, HolderID: "hub-b", Epoch: 7, Token: "opaque-fence-7", Granted: true}, holderID: "hub-a"},
		{name: "missing local holder", grant: Grant{Mode: ModeLeader, HolderID: "hub-a", Epoch: 7, Token: "opaque-fence-7", Granted: true}},
		{name: "zero epoch", grant: Grant{Mode: ModeLeader, HolderID: "hub-a", Epoch: 0, Token: "opaque-fence-7", Granted: true}, holderID: "hub-a"},
		{name: "missing token", grant: Grant{Mode: ModeLeader, HolderID: "hub-a", Epoch: 7, Granted: true}, holderID: "hub-a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
				calls.Add(1)
				return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckAccepted}
			})
			gate := New(backend, sourceFunc(func(context.Context) (Grant, error) { return tt.grant, nil }), tt.holderID)

			result := gate.Execute(context.Background(), capability.Request{ExecutionID: "execution-leader"})
			if tt.allowed {
				if result.Result != domain.ExecutionCompleted || calls.Load() != 1 {
					t.Fatalf("result=%#v calls=%d", result, calls.Load())
				}
				return
			}
			if result.Result != domain.ExecutionFailed || result.ErrorCode != "DISPATCH_AUTHORITY_FENCED" || calls.Load() != 0 {
				t.Fatalf("result=%#v calls=%d", result, calls.Load())
			}
		})
	}
}

func TestGateSourceErrorAndUnknownModeFailClosed(t *testing.T) {
	backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		t.Fatal("backend must not be called")
		return capability.Result{}
	})

	t.Run("source error", func(t *testing.T) {
		gate := New(backend, sourceFunc(func(context.Context) (Grant, error) {
			return Grant{}, errors.New("lease store unavailable")
		}), "hub-a")
		result := gate.Execute(context.Background(), capability.Request{})
		if result.ErrorCode != "DISPATCH_AUTHORITY_UNAVAILABLE" || result.Result != domain.ExecutionFailed {
			t.Fatalf("result=%#v", result)
		}
	})

	t.Run("unknown mode", func(t *testing.T) {
		gate := New(backend, sourceFunc(func(context.Context) (Grant, error) {
			return Grant{Mode: Mode("UNKNOWN"), Granted: true}, nil
		}), "hub-a")
		result := gate.Execute(context.Background(), capability.Request{})
		if result.ErrorCode != "DISPATCH_AUTHORITY_FENCED" || result.Result != domain.ExecutionFailed {
			t.Fatalf("result=%#v", result)
		}
	})
}
