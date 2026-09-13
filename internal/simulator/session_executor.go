package simulator

import (
	"context"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// ActionExecutionSessionResolver resolves the authoritative Session mode for a
// persisted ActionExecution. The Cue Engine creates the ActionExecution before
// invoking the capability executor, so the decision cannot be supplied by an
// untrusted action payload.
type ActionExecutionSessionResolver interface {
	SessionTypeForActionExecution(context.Context, string) (domain.SessionType, error)
}

// SessionExecutor is the F-024 safety boundary between Cue execution and real
// capability transports. SIMULATION actions are always consumed by the
// deterministic simulator and never delegated to the physical executor.
type SessionExecutor struct {
	resolver   ActionExecutionSessionResolver
	physical   capability.Executor
	simulation capability.Executor
}

func NewSessionExecutor(resolver ActionExecutionSessionResolver, physical capability.Executor) *SessionExecutor {
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
	if strings.TrimSpace(req.ExecutionID) == "" {
		return gateFailure("ACTION_EXECUTION_ID_REQUIRED", "action execution identity is required")
	}

	sessionType, err := e.resolver.SessionTypeForActionExecution(ctx, req.ExecutionID)
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

func gateFailure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
