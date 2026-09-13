package simulator

import (
	"context"
	"errors"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// ExecutionSessionResolver resolves authoritative Session mode from either a
// persisted ActionExecution (Cue path) or the single ACTIVE Session that owns
// an immutable Runtime Snapshot (direct Route-output path).
type ExecutionSessionResolver interface {
	SessionTypeForActionExecution(context.Context, string) (domain.SessionType, error)
	SessionTypeForRuntimeSnapshotExecution(context.Context, string) (domain.SessionType, error)
}

// SessionExecutor is the F-024 safety boundary between runtime execution and
// real capability transports. SIMULATION work is always consumed by the
// deterministic simulator and never delegated to the physical executor.
type SessionExecutor struct {
	resolver   ExecutionSessionResolver
	physical   capability.Executor
	simulation capability.Executor
}

func NewSessionExecutor(resolver ExecutionSessionResolver, physical capability.Executor) *SessionExecutor {
	return &SessionExecutor{
		resolver:   resolver,
		physical:   physical,
		simulation: Adapter{},
	}
}

func (e *SessionExecutor) Execute(ctx context.Context, req capability.Request) capability.Result {
	if e == nil || e.resolver == nil || e.physical == nil || e.simulation == nil {
		return gateFailure("SIMULATION_GATE_UNAVAILABLE", "simulation execution gate is unavailable")
	}
	if strings.TrimSpace(req.ExecutionID) == "" && strings.TrimSpace(req.RuntimeSnapshotID) == "" {
		return gateFailure("ACTION_EXECUTION_ID_REQUIRED", "action execution identity or runtime snapshot is required")
	}

	sessionType, err := e.resolveSessionType(ctx, req)
	if err != nil {
		return gateFailure("SESSION_MODE_UNAVAILABLE", "authoritative session mode could not be resolved")
	}

	switch sessionType {
	case domain.SessionSimulation:
		return e.simulation.Execute(ctx, req)
	case domain.SessionRehearsal, domain.SessionShow:
		return e.physical.Execute(ctx, req)
	default:
		return gateFailure("SESSION_MODE_UNSUPPORTED", "unsupported session mode at execution boundary")
	}
}

func (e *SessionExecutor) resolveSessionType(ctx context.Context, req capability.Request) (domain.SessionType, error) {
	executionID := strings.TrimSpace(req.ExecutionID)
	if executionID != "" {
		sessionType, err := e.resolver.SessionTypeForActionExecution(ctx, executionID)
		if err == nil {
			return sessionType, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return "", err
		}
	}

	snapshotID := strings.TrimSpace(req.RuntimeSnapshotID)
	if snapshotID != "" {
		return e.resolver.SessionTypeForRuntimeSnapshotExecution(ctx, snapshotID)
	}
	return "", domain.ErrNotFound
}

func gateFailure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
