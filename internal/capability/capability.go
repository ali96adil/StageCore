package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type Target struct {
	AliasID       string
	Ref           string
	LogicalType   string
	Configuration json.RawMessage
}

type Request struct {
	ExecutionID       string
	RuntimeSnapshotID string
	Capability        string
	Target            *Target
	Parameters        json.RawMessage
	Priority          string
	TimeoutMS         int64
	CorrelationID     string
}

type Result struct {
	Result          domain.ExecutionResult
	AckLevel        contracts.AckLevel
	ResponseSummary string
	ErrorCode       string
}

type Executor interface {
	Execute(context.Context, Request) Result
}

type ExecutorFunc func(context.Context, Request) Result

func (f ExecutorFunc) Execute(ctx context.Context, req Request) Result {
	return f(ctx, req)
}

type Registry struct {
	mu              sync.RWMutex
	executors       map[string]Executor
	targetExecutors map[string]Executor
}

func NewRegistry() *Registry {
	return &Registry{
		executors:       make(map[string]Executor),
		targetExecutors: make(map[string]Executor),
	}
}

func (r *Registry) Register(capabilityKey string, executor Executor) error {
	key := strings.TrimSpace(capabilityKey)
	if key == "" || executor == nil {
		return fmt.Errorf("capability key and executor are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.executors[key]; exists {
		return fmt.Errorf("capability %q already registered", key)
	}
	r.executors[key] = executor
	return nil
}

// RegisterTargetType installs an execution boundary for one logical target type.
// Target-type dispatch takes precedence over capability-key dispatch so a real
// capability such as osc.send can execute locally for ordinary targets or be
// forwarded to an assigned Companion when the immutable Snapshot target is a
// machine_role. The capability identity itself never changes.
func (r *Registry) RegisterTargetType(logicalType string, executor Executor) error {
	key := normalizeLogicalType(logicalType)
	if key == "" || executor == nil {
		return fmt.Errorf("logical target type and executor are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.targetExecutors[key]; exists {
		return fmt.Errorf("target type %q already registered", logicalType)
	}
	r.targetExecutors[key] = executor
	return nil
}

// HasTargetTypeExecutor reports whether execution for this logical target type
// is handled by a target dispatcher instead of the capability-key executor.
func (r *Registry) HasTargetTypeExecutor(logicalType string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.targetExecutors[normalizeLogicalType(logicalType)]
	return ok
}

// Supports reports whether the runtime has an execution boundary for the
// capability/target combination. A logical target dispatcher (for example a
// Machine Role forwarded to a Companion) takes precedence in the same way as
// Execute. This is intentionally a structural availability check; live
// endpoint readiness remains a Preflight concern.
func (r *Registry) Supports(capabilityKey, logicalType string) bool {
	if r == nil {
		return false
	}
	capabilityKey = strings.TrimSpace(capabilityKey)
	if capabilityKey == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.targetExecutors[normalizeLogicalType(logicalType)]; ok {
		return true
	}
	_, ok := r.executors[capabilityKey]
	return ok
}

func (r *Registry) Execute(ctx context.Context, req Request) Result {
	r.mu.RLock()
	var executor Executor
	if req.Target != nil {
		executor = r.targetExecutors[normalizeLogicalType(req.Target.LogicalType)]
	}
	if executor == nil {
		executor = r.executors[req.Capability]
	}
	r.mu.RUnlock()
	if executor == nil {
		return Result{
			Result:          domain.ExecutionFailed,
			AckLevel:        contracts.AckNone,
			ErrorCode:       "CAPABILITY_UNAVAILABLE",
			ResponseSummary: "no executor registered for capability or logical target type",
		}
	}
	return executor.Execute(ctx, req)
}

func normalizeLogicalType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
