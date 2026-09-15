package dispatchauthority

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestNilSourceFuncFailsClosed(t *testing.T) {
	var calls atomic.Int32
	backend := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		calls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted}
	})
	var source SourceFunc
	gate := New(backend, source)

	result := gate.Execute(context.Background(), capability.Request{ExecutionID: "exec-nil-source"})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "PHYSICAL_DISPATCH_AUTHORITY_UNAVAILABLE" {
		t.Fatalf("result=%#v", result)
	}
	if calls.Load() != 0 {
		t.Fatalf("backend calls=%d, want 0", calls.Load())
	}
}
