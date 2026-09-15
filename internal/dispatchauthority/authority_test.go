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

func TestStandaloneAllowsPhysicalDispatch(t *testing.T) {
	var calls atomic.Int32
	request := capability.Request{ExecutionID: "exec-1", Capability: "osc.send"}
	backend := capability.ExecutorFunc(func(_ context.Context, got capability.Request) capability.Result {
		calls.Add(1)
		if got.ExecutionID != request.ExecutionID || got.Capability != request.Capability {
			t.Fatalf("request mutated: %#v", got)
		}
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
	})

	result := NewStandalone(backend).Execute(context.Background(), request)
	if result.Result != domain.ExecutionCompleted {
		t.Fatalf("result=%#v", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("backend calls=%d, want 1", calls.Load())
	}
}

func TestStandbyBlocksPhysicalDispatch(t *testing.T) {
	var calls atomic.Int32
	backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		calls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted}
	})
	gate := New(backend, SourceFunc(func(context.Context) (Snapshot, error) {
		return Snapshot{Mode: ModeStandby, HolderID: "hub-a", Epoch: 7}, nil
	}))

	result := gate.Execute(context.Background(), capability.Request{ExecutionID: "exec-standby"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "PHYSICAL_DISPATCH_NOT_LEADER" {
		t.Fatalf("result=%#v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("backend calls=%d, want 0", calls.Load())
	}
}

func TestLeaderRequiresFencingIdentity(t *testing.T) {
	tests := []struct {
		name      string
		snapshot  Snapshot
		wantCalls int32
		wantCode  string
	}{
		{name: "missing holder", snapshot: Snapshot{Mode: ModeLeader, Epoch: 4}, wantCode: "PHYSICAL_DISPATCH_AUTHORITY_INVALID"},
		{name: "missing epoch", snapshot: Snapshot{Mode: ModeLeader, HolderID: "hub-a"}, wantCode: "PHYSICAL_DISPATCH_AUTHORITY_INVALID"},
		{name: "valid leader", snapshot: Snapshot{Mode: ModeLeader, HolderID: "hub-a", Epoch: 4}, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
				calls.Add(1)
				return capability.Result{Result: domain.ExecutionCompleted}
			})
			gate := New(backend, SourceFunc(func(context.Context) (Snapshot, error) {
				return tt.snapshot, nil
			}))

			result := gate.Execute(context.Background(), capability.Request{ExecutionID: "exec-leader"})
			if calls.Load() != tt.wantCalls {
				t.Fatalf("backend calls=%d, want %d", calls.Load(), tt.wantCalls)
			}
			if tt.wantCode == "" {
				if result.Result != domain.ExecutionCompleted {
					t.Fatalf("result=%#v", result)
				}
			} else if result.Result != domain.ExecutionFailed || result.ErrorCode != tt.wantCode {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}

func TestAuthorityResolutionFailureFailsClosed(t *testing.T) {
	var calls atomic.Int32
	backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		calls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted}
	})
	gate := New(backend, SourceFunc(func(context.Context) (Snapshot, error) {
		return Snapshot{}, errors.New("lease unavailable")
	}))

	result := gate.Execute(context.Background(), capability.Request{ExecutionID: "exec-error"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "PHYSICAL_DISPATCH_AUTHORITY_UNAVAILABLE" {
		t.Fatalf("result=%#v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("backend calls=%d, want 0", calls.Load())
	}
}

func TestUnknownAuthorityModeFailsClosed(t *testing.T) {
	var calls atomic.Int32
	backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		calls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted}
	})
	gate := New(backend, SourceFunc(func(context.Context) (Snapshot, error) {
		return Snapshot{Mode: Mode("UNKNOWN")}, nil
	}))

	result := gate.Execute(context.Background(), capability.Request{ExecutionID: "exec-unknown"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "PHYSICAL_DISPATCH_AUTHORITY_INVALID" {
		t.Fatalf("result=%#v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("backend calls=%d, want 0", calls.Load())
	}
}
