package simulator

import (
	"context"
	"errors"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// ExecutionSessionResolver resolves authoritative Session ownership from either
// a persisted ActionExecution (Cue path) or the single ACTIVE Session that owns
// an immutable Runtime Snapshot (direct Route-output path).
type ExecutionSessionResolver interface {
	SessionForActionExecution(context.Context, string) (domain.Session, error)
	SessionForRuntimeSnapshotExecution(context.Context, string) (domain.Session, error)
}

// SessionExecutor is the F-024 safety boundary between runtime execution and
// real capability transports. SIMULATION work is always consumed by the
// session-scoped Digital Twin and never delegated to the physical executor.
type SessionExecutor struct {
	resolver    ExecutionSessionResolver
	physical    capability.Executor
	digitalTwin *DigitalTwin
}

func NewSessionExecutor(resolver ExecutionSessionResolver, physical capability.Executor) *SessionExecutor {
	return NewSessionExecutorWithDigitalTwin(resolver, physical, NewDigitalTwin())
}

func NewSessionExecutorWithDigitalTwin(resolver ExecutionSessionResolver, physical capability.Executor, twin *DigitalTwin) *SessionExecutor {
	return &SessionExecutor{
		resolver:    resolver,
		physical:    physical,
		digitalTwin: twin,
	}
}

func (e *SessionExecutor) Execute(ctx context.Context, req capability.Request) capability.Result {
	if e == nil || e.resolver == nil || e.physical == nil || e.digitalTwin == nil {
		return gateFailure("SIMULATION_GATE_UNAVAILABLE", "simulation execution gate is unavailable")
	}
	if strings.TrimSpace(req.ExecutionID) == "" && strings.TrimSpace(req.RuntimeSnapshotID) == "" {
		return gateFailure("ACTION_EXECUTION_ID_REQUIRED", "action execution identity or runtime snapshot is required")
	}

	session, err := e.resolveSession(ctx, req)
	if err != nil {
		return gateFailure("SESSION_MODE_UNAVAILABLE", "authoritative session mode could not be resolved")
	}
	if session.Status != domain.SessionActive {
		return gateFailure("SESSION_NOT_ACTIVE", "authoritative execution session is not active")
	}

	// Canonicalize authority before either execution path. Caller-supplied
	// SessionID/SnapshotID can never redirect Digital Twin state or physical
	// execution to another Session.
	req.SessionID = session.ID
	req.ProjectID = session.ProjectID
	req.RuntimeSnapshotID = session.RuntimeSnapshotID

	switch session.Type {
	case domain.SessionSimulation:
		return e.digitalTwin.Execute(ctx, req)
	case domain.SessionRehearsal, domain.SessionShow:
		return e.physical.Execute(ctx, req)
	default:
		return gateFailure("SESSION_MODE_UNSUPPORTED", "unsupported session mode at execution boundary")
	}
}

func (e *SessionExecutor) resolveSession(ctx context.Context, req capability.Request) (domain.Session, error) {
	executionID := strings.TrimSpace(req.ExecutionID)
	if executionID != "" {
		session, err := e.resolver.SessionForActionExecution(ctx, executionID)
		if err == nil {
			return session, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return domain.Session{}, err
		}
	}

	snapshotID := strings.TrimSpace(req.RuntimeSnapshotID)
	if snapshotID != "" {
		return e.resolver.SessionForRuntimeSnapshotExecution(ctx, snapshotID)
	}
	return domain.Session{}, domain.ErrNotFound
}

func gateFailure(code, summary string) capability.Result {
	return capability.Result{
		Result:          domain.ExecutionFailed,
		AckLevel:        contracts.AckNone,
		ErrorCode:       code,
		ResponseSummary: summary,
	}
}
