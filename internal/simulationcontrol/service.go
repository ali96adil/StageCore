package simulationcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	CommandStart = "simulation.start"
	CommandStop  = "simulation.stop"
)

type Service struct {
	store       *store.Store
	engine      *cueengine.Engine
	twin        *simulator.DigitalTwin
	checkpoints *simulator.CheckpointManager
}

type StartRequest struct {
	ProjectID     string
	Name          string
	Issuer        string
	RequestID     string
	StartKind     domain.SessionStartPositionKind
	StartCueID    string
	EndCueID      string
	CheckpointID  string
}

type CueRequest struct {
	SessionID            string
	Issuer               string
	RequestID             string
	ExpectedCurrentCueID *string
	RequestedCueID       *string
	OperatorNote         *string
}

type StopRequest struct {
	SessionID string
	Issuer    string
	RequestID string
}

type Status struct {
	ProjectID         string                         `json:"project_id"`
	RuntimeSnapshotID string                         `json:"runtime_snapshot_id,omitempty"`
	Session           *domain.Session                `json:"session,omitempty"`
	Twin              simulator.SessionSnapshot      `json:"digital_twin"`
	Checkpoints       []domain.SimulationCheckpoint  `json:"checkpoints"`
	CueExecutions     []domain.CueExecution          `json:"cue_executions"`
	Events            []contracts.EventEnvelope      `json:"events"`
}

func New(s *store.Store, engine *cueengine.Engine, twin *simulator.DigitalTwin) *Service {
	if s == nil || engine == nil || twin == nil {
		return nil
	}
	return &Service{
		store: s,
		engine: engine,
		twin: twin,
		checkpoints: simulator.NewCheckpointManager(s, twin),
	}
}

func (s *Service) Start(ctx context.Context, req StartRequest) (domain.Session, contracts.CommandResult) {
	if s == nil || s.store == nil || s.engine == nil || s.twin == nil {
		return domain.Session{}, failed(req.RequestID, "SIMULATION_CONTROL_UNAVAILABLE", "simulation control is unavailable", "")
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.Issuer = strings.TrimSpace(req.Issuer)
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.ProjectID == "" || req.Issuer == "" || req.RequestID == "" {
		return domain.Session{}, rejected(req.RequestID, "SIMULATION_CONTEXT_REQUIRED", "project, issuer and request_id are required", req.ProjectID)
	}

	project, err := s.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return domain.Session{}, fromStoreError(req.RequestID, "PROJECT_LOOKUP_FAILED", err, req.ProjectID)
	}
	snapshot, err := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil {
		return domain.Session{}, failed(req.RequestID, "SNAPSHOT_LOOKUP_FAILED", "failed to resolve published Runtime Snapshot", project.ID)
	}
	if snapshot == nil {
		return domain.Session{}, rejected(req.RequestID, "SNAPSHOT_REQUIRED", "a published Runtime Snapshot is required", project.ID)
	}

	command := commandEnvelope(req.RequestID, CommandStart, project.ID, snapshot.ID, req.Issuer, json.RawMessage(`{}`))
	if existing, terminal, ok := s.reserve(ctx, command); !ok {
		if terminal {
			return s.sessionFromResult(ctx, existing), existing
		}
		return domain.Session{}, existing
	}
	finish := func(session domain.Session, result contracts.CommandResult) (domain.Session, contracts.CommandResult) {
		if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
			return domain.Session{}, failed(command.CommandID, "COMMAND_FINISH_FAILED", "failed to persist command result", command.CommandID)
		}
		return session, result
	}

	active, err := s.store.ActiveSessionForProject(ctx, project.ID)
	if err != nil {
		return finish(domain.Session{}, failed(command.CommandID, "SESSION_LOOKUP_FAILED", "failed to inspect active Session", project.ID))
	}
	if active != nil {
		return finish(domain.Session{}, rejected(command.CommandID, "SESSION_ALREADY_ACTIVE", "the Project already has an active runtime Session", active.ID))
	}

	position, err := buildStartPosition(req)
	if err != nil {
		return finish(domain.Session{}, rejected(command.CommandID, "INVALID_SIMULATION_START", err.Error(), project.ID))
	}
	session, err := s.store.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: snapshot.ID,
		SessionType: domain.SessionSimulation,
		Name: strings.TrimSpace(req.Name),
		StartPosition: position,
	})
	if err != nil {
		return finish(domain.Session{}, fromStoreError(command.CommandID, "SIMULATION_START_FAILED", err, snapshot.ID))
	}
	abort := func(reason string) {
		_ = s.store.EndSessionLifecycle(context.WithoutCancel(ctx), session.ID, domain.SessionLifecycleAborted, reason)
	}
	if err := s.twin.BindSession(session); err != nil {
		abort("DIGITAL_TWIN_BIND_FAILED")
		return finish(domain.Session{}, failed(command.CommandID, "DIGITAL_TWIN_BIND_FAILED", err.Error(), session.ID))
	}
	if position.Kind == domain.SessionStartCheckpoint {
		if _, err := s.checkpoints.Restore(ctx, session.ID, strings.TrimSpace(req.CheckpointID)); err != nil {
			abort("CHECKPOINT_RESTORE_FAILED")
			return finish(domain.Session{}, fromStoreError(command.CommandID, "CHECKPOINT_RESTORE_FAILED", err, req.CheckpointID))
		}
		session, err = s.store.GetSessionFoundation(ctx, session.ID)
		if err != nil {
			abort("CHECKPOINT_SESSION_REFRESH_FAILED")
			return finish(domain.Session{}, failed(command.CommandID, "CHECKPOINT_SESSION_REFRESH_FAILED", err.Error(), session.ID))
		}
	}
	if err := s.appendEvent(ctx, session, "simulation.started", command, map[string]any{
		"session_id": session.ID,
		"start_position": session.StartPosition,
		"physical_dispatch": false,
	}); err != nil {
		abort("SIMULATION_STARTED_EVENT_FAILED")
		return finish(domain.Session{}, failed(command.CommandID, "SIMULATION_STARTED_EVENT_FAILED", err.Error(), session.ID))
	}
	payload, _ := json.Marshal(map[string]any{
		"session_id": session.ID,
		"session_type": session.Type,
		"runtime_snapshot_id": session.RuntimeSnapshotID,
		"start_position": session.StartPosition,
	})
	return finish(session, contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: payload})
}

func buildStartPosition(req StartRequest) (domain.SessionStartPosition, error) {
	kind := req.StartKind
	if kind == "" {
		kind = domain.SessionStartBeginning
	}
	position := domain.SessionStartPosition{Version: domain.SessionContractVersion1, Kind: kind, Metadata: json.RawMessage(`{}`)}
	switch kind {
	case domain.SessionStartBeginning:
		return position, nil
	case domain.SessionStartRange:
		startCueID := strings.TrimSpace(req.StartCueID)
		endCueID := strings.TrimSpace(req.EndCueID)
		if startCueID == "" || endCueID == "" {
			return domain.SessionStartPosition{}, fmt.Errorf("RANGE requires start_cue_id and end_cue_id")
		}
		position.CueID = &startCueID
		metadata, _ := json.Marshal(domain.SimulationRangeMetadata{EndCueID: endCueID})
		position.Metadata = metadata
		return position, nil
	case domain.SessionStartCheckpoint:
		checkpointID := strings.TrimSpace(req.CheckpointID)
		if checkpointID == "" {
			return domain.SessionStartPosition{}, fmt.Errorf("CHECKPOINT requires checkpoint_id")
		}
		metadata, _ := json.Marshal(domain.SimulationCheckpointStartMetadata{CheckpointID: checkpointID})
		position.Metadata = metadata
		return position, nil
	default:
		return domain.SessionStartPosition{}, fmt.Errorf("unsupported simulation start kind %q", kind)
	}
}

func (s *Service) Go(ctx context.Context, req CueRequest) contracts.CommandResult {
	if s == nil || s.engine == nil || strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "SIMULATION_CONTEXT_REQUIRED", "session, issuer and request_id are required", req.SessionID)
	}
	session, err := s.store.GetSessionFoundation(ctx, strings.TrimSpace(req.SessionID))
	if err != nil {
		return fromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	if session.Type != domain.SessionSimulation || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return rejected(req.RequestID, "SIMULATION_NOT_ACTIVE", "an ACTIVE SIMULATION Session is required", session.ID)
	}
	if session.StateTruth.ManualConfirmationRequired {
		return rejected(req.RequestID, "SIMULATION_START_CONFIRMATION_REQUIRED", "the selected simulation start state requires explicit operator confirmation", session.ID)
	}
	if session.StartPosition.Kind == domain.SessionStartCheckpoint {
		if session.StateTruth.DesiredStateRef == nil || session.StateTruth.VerifiedStateRef == nil || *session.StateTruth.DesiredStateRef != *session.StateTruth.VerifiedStateRef {
			return rejected(req.RequestID, "SIMULATION_CHECKPOINT_RESTORE_REQUIRED", "checkpoint state has not been restored and verified in the virtual runtime", session.ID)
		}
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{
		ExpectedCurrentCueID: req.ExpectedCurrentCueID,
		RequestedNextCueID: req.RequestedCueID,
		OperatorNote: req.OperatorNote,
	})
	command := commandEnvelope(req.RequestID, cueengine.CueGoCommandType, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, payload)
	return s.engine.ExecuteCueGo(ctx, session.ID, command)
}

func (s *Service) Stop(ctx context.Context, req StopRequest) contracts.CommandResult {
	if s == nil || strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "SIMULATION_CONTEXT_REQUIRED", "session, issuer and request_id are required", req.SessionID)
	}
	session, err := s.store.GetSessionFoundation(ctx, strings.TrimSpace(req.SessionID))
	if err != nil {
		return fromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	command := commandEnvelope(req.RequestID, CommandStop, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, json.RawMessage(`{}`))
	if existing, terminal, ok := s.reserve(ctx, command); !ok {
		if terminal {
			return existing
		}
		return existing
	}
	finish := func(result contracts.CommandResult) contracts.CommandResult {
		if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
			return failed(command.CommandID, "COMMAND_FINISH_FAILED", "failed to persist command result", command.CommandID)
		}
		return result
	}
	if session.Type != domain.SessionSimulation || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return finish(rejected(command.CommandID, "SIMULATION_NOT_ACTIVE", "an ACTIVE SIMULATION Session is required", session.ID))
	}
	if err := s.appendEvent(ctx, session, "simulation.stopped", command, map[string]any{"session_id": session.ID}); err != nil {
		return finish(failed(command.CommandID, "SIMULATION_STOP_EVENT_FAILED", err.Error(), session.ID))
	}
	if err := s.store.EndSessionLifecycle(ctx, session.ID, domain.SessionLifecycleStopped, "operator stopped simulation"); err != nil {
		return finish(fromStoreError(command.CommandID, "SIMULATION_STOP_FAILED", err, session.ID))
	}
	payload, _ := json.Marshal(map[string]any{"session_id": session.ID, "status": domain.SessionCompleted})
	return finish(contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: payload})
}

func (s *Service) ConfirmStartState(ctx context.Context, sessionID, issuer string) (domain.Session, error) {
	if s == nil {
		return domain.Session{}, fmt.Errorf("simulation control is unavailable")
	}
	if err := s.store.ConfirmSimulationStartState(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(issuer)); err != nil {
		return domain.Session{}, err
	}
	return s.store.GetSessionFoundation(ctx, strings.TrimSpace(sessionID))
}

func (s *Service) CaptureCheckpoint(ctx context.Context, sessionID string) (domain.SimulationCheckpoint, error) {
	if s == nil || s.checkpoints == nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("simulation checkpoint control is unavailable")
	}
	return s.checkpoints.Capture(ctx, strings.TrimSpace(sessionID))
}

func (s *Service) RestoreCheckpoint(ctx context.Context, sessionID, checkpointID string) (domain.SimulationCheckpoint, error) {
	if s == nil || s.checkpoints == nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("simulation checkpoint control is unavailable")
	}
	return s.checkpoints.Restore(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(checkpointID))
}

func (s *Service) ConfigureFault(ctx context.Context, sessionID string, fault simulator.FaultScenario) error {
	if s == nil || s.twin == nil {
		return fmt.Errorf("Digital Twin is unavailable")
	}
	return s.twin.ConfigureFaultContext(ctx, strings.TrimSpace(sessionID), fault)
}

func (s *Service) ClearFault(ctx context.Context, sessionID, targetRef, capability string) error {
	if s == nil || s.twin == nil {
		return fmt.Errorf("Digital Twin is unavailable")
	}
	return s.twin.ClearFaultContext(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(targetRef), strings.TrimSpace(capability))
}

func (s *Service) SetTargetOnline(ctx context.Context, sessionID, targetRef string, online bool) error {
	if s == nil || s.twin == nil {
		return fmt.Errorf("Digital Twin is unavailable")
	}
	return s.twin.SetTargetOnlineContext(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(targetRef), online)
}

func (s *Service) ResetTwin(ctx context.Context, sessionID string) error {
	if s == nil || s.twin == nil {
		return fmt.Errorf("Digital Twin is unavailable")
	}
	return s.twin.ResetSessionContext(ctx, strings.TrimSpace(sessionID))
}

func (s *Service) Status(ctx context.Context, projectID string) (Status, error) {
	if s == nil || s.store == nil || s.twin == nil {
		return Status{}, fmt.Errorf("simulation control is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Status{}, fmt.Errorf("%w: project is required", domain.ErrInvalidInput)
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return Status{}, err
	}
	status := Status{ProjectID: project.ID, Checkpoints: []domain.SimulationCheckpoint{}, CueExecutions: []domain.CueExecution{}, Events: []contracts.EventEnvelope{}}
	latest, err := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil {
		return Status{}, err
	}
	if latest != nil {
		status.RuntimeSnapshotID = latest.ID
		status.Checkpoints, err = s.store.ListSimulationCheckpoints(ctx, project.ID, latest.ID, 50)
		if err != nil {
			return Status{}, err
		}
	}
	active, err := s.store.ActiveSessionForProject(ctx, project.ID)
	if err != nil {
		return Status{}, err
	}
	if active == nil || active.Type != domain.SessionSimulation {
		return status, nil
	}
	foundation, err := s.store.GetSessionFoundation(ctx, active.ID)
	if err != nil {
		return Status{}, err
	}
	status.Session = &foundation
	status.RuntimeSnapshotID = foundation.RuntimeSnapshotID
	status.Twin = s.twin.Snapshot(foundation.ID)
	status.Checkpoints, err = s.store.ListSimulationCheckpoints(ctx, project.ID, foundation.RuntimeSnapshotID, 50)
	if err != nil {
		return Status{}, err
	}
	status.CueExecutions, err = s.store.ListCueExecutions(ctx, foundation.ID)
	if err != nil {
		return Status{}, err
	}
	status.Events, err = s.store.ListEvents(ctx, foundation.ID)
	if err != nil {
		return Status{}, err
	}
	return status, nil
}

func (s *Service) reserve(ctx context.Context, command contracts.CommandEnvelope) (contracts.CommandResult, bool, bool) {
	record, reserved, err := s.store.ReserveCommand(ctx, command)
	if err != nil {
		return failed(command.CommandID, "COMMAND_RESERVE_FAILED", err.Error(), command.CommandID), false, false
	}
	if reserved {
		return contracts.CommandResult{}, false, true
	}
	if stored, terminal, err := s.store.StoredCommandResult(record); err == nil && terminal {
		return stored, true, false
	}
	return rejected(record.CommandID, "DUPLICATE_UNRESOLVED", "matching command is already accepted and will not be replayed", record.CommandID), false, false
}

func (s *Service) sessionFromResult(ctx context.Context, result contracts.CommandResult) domain.Session {
	if result.Status != contracts.CommandCompleted || len(result.Payload) == 0 {
		return domain.Session{}
	}
	var payload struct { SessionID string `json:"session_id"` }
	if json.Unmarshal(result.Payload, &payload) != nil || strings.TrimSpace(payload.SessionID) == "" {
		return domain.Session{}
	}
	session, err := s.store.GetSessionFoundation(ctx, payload.SessionID)
	if err != nil {
		return domain.Session{}
	}
	return session
}

func (s *Service) appendEvent(ctx context.Context, session domain.Session, eventType string, command contracts.CommandEnvelope, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.store.AppendEvent(ctx, &session.ID, contracts.EventEnvelope{
		EventType: eventType,
		SchemaVersion: contracts.SchemaVersion1,
		Source: "hub.simulation_control",
		ProjectID: session.ProjectID,
		RuntimeSnapshotID: session.RuntimeSnapshotID,
		CorrelationID: command.CorrelationID,
		CausationID: command.CommandID,
		Priority: "P1",
		TraceContext: json.RawMessage(`{}`),
		Payload: body,
	})
	return err
}

func commandEnvelope(requestID, commandType, projectID, snapshotID, issuer string, payload json.RawMessage) contracts.CommandEnvelope {
	return contracts.CommandEnvelope{
		CommandID: requestID,
		CommandType: commandType,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(),
		ProjectID: projectID,
		RuntimeSnapshotID: snapshotID,
		Issuer: issuer,
		CorrelationID: requestID,
		Priority: "P1",
		IdempotencyKey: requestID,
		Payload: payload,
	}
}

func rejected(commandID, code, message, affected string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID,
		Status: contracts.CommandRejected,
		Error: &contracts.ContractError{ErrorCode: code, Category: "VALIDATION", Message: message, Retryable: false, AffectedEntityID: affected},
	}
}

func failed(commandID, code, message, affected string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID,
		Status: contracts.CommandFailed,
		Error: &contracts.ContractError{ErrorCode: code, Category: "INTERNAL", Message: message, Retryable: false, AffectedEntityID: affected},
	}
}

func fromStoreError(commandID, code string, err error, affected string) contracts.CommandResult {
	if errors.Is(err, domain.ErrNotFound) {
		return rejected(commandID, "NOT_FOUND", "requested StageCore entity was not found", affected)
	}
	if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrConflict) {
		return rejected(commandID, code, err.Error(), affected)
	}
	return failed(commandID, code, err.Error(), affected)
}
