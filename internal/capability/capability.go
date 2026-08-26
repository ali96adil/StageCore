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
	ExecutionID   string
	Capability    string
	Target        *Target
	Parameters    json.RawMessage
	Priority      string
	TimeoutMS     int64
	CorrelationID string
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
	mu        sync.RWMutex
	executors map[string]Executor
}

func NewRegistry() *Registry {
	return &Registry{executors: make(map[string]Executor)}
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

func (r *Registry) Execute(ctx context.Context, req Request) Result {
	r.mu.RLock()
	executor := r.executors[req.Capability]
	r.mu.RUnlock()
	if executor == nil {
		return Result{
			Result:          domain.ExecutionFailed,
			AckLevel:        contracts.AckNone,
			ErrorCode:       "CAPABILITY_UNAVAILABLE",
			ResponseSummary: "no executor registered for capability",
		}
	}
	return executor.Execute(ctx, req)
}
