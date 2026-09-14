package simulationinput

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type Service struct {
	store   *store.Store
	routing *routing.Engine
	now     func() time.Time
}

type Option func(*Service)

func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func New(s *store.Store, engine *routing.Engine, options ...Option) *Service {
	if s == nil || engine == nil {
		return nil
	}
	service := &Service{store: s, routing: engine, now: time.Now}
	for _, option := range options {
		option(service)
	}
	return service
}

type InjectRequest struct {
	ProjectID       string
	Issuer          string
	RequestID       string
	InputID         string
	Value           json.RawMessage
	ConfirmCritical bool
}

type Inputs struct {
	SessionID         string           `json:"session_id"`
	RuntimeSnapshotID string           `json:"runtime_snapshot_id"`
	Items             []snapshot.Input `json:"inputs"`
}

func (s *Service) List(ctx context.Context, projectID string) (Inputs, error) {
	session, err := s.activeSimulation(ctx, projectID)
	if err != nil {
		return Inputs{}, err
	}
	runtimeSnapshot, err := s.store.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
	if err != nil {
		return Inputs{}, err
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return Inputs{}, err
	}
	items := make([]snapshot.Input, 0, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		if input.Enabled {
			items = append(items, input)
		}
	}
	return Inputs{SessionID: session.ID, RuntimeSnapshotID: session.RuntimeSnapshotID, Items: items}, nil
}

func (s *Service) Inject(ctx context.Context, req InjectRequest) contracts.CommandResult {
	if s == nil || s.store == nil || s.routing == nil {
		return failure(req.RequestID, "SIMULATION_INPUT_UNAVAILABLE", "simulation input service is unavailable", "")
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.Issuer = strings.TrimSpace(req.Issuer)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.InputID = strings.TrimSpace(req.InputID)
	if req.ProjectID == "" || req.Issuer == "" || req.RequestID == "" || req.InputID == "" || len(req.Value) == 0 {
		return failure(req.RequestID, "SIMULATION_INPUT_CONTEXT_REQUIRED", "project, issuer, request_id, input_id and value are required", req.ProjectID)
	}
	session, err := s.activeSimulation(ctx, req.ProjectID)
	if err != nil {
		return failure(req.RequestID, "SIMULATION_NOT_ACTIVE", err.Error(), req.ProjectID)
	}
	payload, err := json.Marshal(routing.InjectTestPayload{InputID: req.InputID, Value: req.Value, ConfirmCritical: req.ConfirmCritical})
	if err != nil {
		return failure(req.RequestID, "SIMULATION_INPUT_ENCODE_FAILED", err.Error(), req.InputID)
	}
	command := contracts.CommandEnvelope{
		CommandID:         req.RequestID,
		CommandType:       routing.InputInjectTestCommandType,
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          s.now().UTC(),
		ProjectID:         session.ProjectID,
		RuntimeSnapshotID: session.RuntimeSnapshotID,
		Issuer:            req.Issuer,
		CorrelationID:     req.RequestID,
		Priority:          "P1",
		IdempotencyKey:    req.RequestID,
		Payload:           payload,
	}
	return s.routing.InjectTest(ctx, session.ID, command)
}

func (s *Service) activeSimulation(ctx context.Context, projectID string) (domain.Session, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return domain.Session{}, fmt.Errorf("%w: project is required", domain.ErrInvalidInput)
	}
	active, err := s.store.ActiveSessionForProject(ctx, projectID)
	if err != nil {
		return domain.Session{}, err
	}
	if active == nil {
		return domain.Session{}, fmt.Errorf("%w: ACTIVE SIMULATION session is required", domain.ErrConflict)
	}

	// ActiveSessionForProject exposes the coarse Session view. Resolve the
	// F-027 foundation before checking lifecycle truth so an active Simulation
	// cannot be rejected merely because the coarse view omits LifecycleState.
	foundation, err := s.store.GetSessionFoundation(ctx, active.ID)
	if err != nil {
		return domain.Session{}, err
	}
	if foundation.ProjectID != projectID ||
		foundation.Type != domain.SessionSimulation ||
		foundation.Status != domain.SessionActive ||
		foundation.LifecycleState != domain.SessionLifecycleActive {
		return domain.Session{}, fmt.Errorf("%w: ACTIVE SIMULATION session is required", domain.ErrConflict)
	}
	return foundation, nil
}

func failure(commandID, code, message, affected string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID,
		Status:    contracts.CommandRejected,
		Error: &contracts.ContractError{
			ErrorCode:        code,
			Category:         "VALIDATION",
			Message:          message,
			Retryable:        false,
			AffectedEntityID: affected,
		},
	}
}
