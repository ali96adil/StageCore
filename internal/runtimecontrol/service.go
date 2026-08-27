package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	CommandCueStop        = "cue.stop"
	CommandRehearsalStart = "rehearsal.start"
	CommandRehearsalStop  = "rehearsal.stop"
	CommandShowEnter      = "show.enter"
	CommandShowExit       = "show.exit"

	defaultStopWait = 2 * time.Second
)

type ShowGate func(context.Context, string, string) (bool, string, error)

type Option func(*Service)

func WithShowGate(gate ShowGate) Option {
	return func(s *Service) { s.showGate = gate }
}

type Service struct {
	store    *store.Store
	engine   *cueengine.Engine
	executor *stoppableExecutor
	showGate ShowGate

	mu     sync.Mutex
	active map[string]activeRun
}

type activeRun struct {
	requestID     string
	correlationID string
	done          chan struct{}
}

type StartRequest struct {
	ProjectID string
	Mode      domain.SessionType
	Name      string
	Issuer    string
	RequestID string
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

func New(s *store.Store, executor capability.Executor, options ...Option) *Service {
	stoppable := newStoppableExecutor(executor)
	service := &Service{
		store: s, executor: stoppable,
		engine: cueengine.NewWithExecutor(s, stoppable),
		active: make(map[string]activeRun),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *Service) StartSession(ctx context.Context, req StartRequest) (domain.Session, contracts.CommandResult) {
	if s == nil || s.store == nil || strings.TrimSpace(req.ProjectID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return domain.Session{}, rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "project, issuer and request_id are required", "")
	}
	commandType := CommandRehearsalStart
	if req.Mode == domain.SessionShow {
		commandType = CommandShowEnter
	} else if req.Mode != domain.SessionRehearsal {
		return domain.Session{}, rejected(req.RequestID, "MODE_UNSUPPORTED", "only REHEARSAL and SHOW can be started", string(req.Mode))
	}

	project, err := s.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return domain.Session{}, resultFromStoreError(req.RequestID, "PROJECT_LOOKUP_FAILED", err, req.ProjectID)
	}
	snapshot, err := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil {
		return domain.Session{}, failed(req.RequestID, "SNAPSHOT_LOOKUP_FAILED")
	}
	snapshotID := ""
	if snapshot != nil {
		snapshotID = snapshot.ID
	}
	command := commandEnvelope(req.RequestID, commandType, project.ID, snapshotID, req.Issuer, json.RawMessage(`{}`))
	if existing, terminal, ok := s.reserve(ctx, command); !ok {
		return domain.Session{}, existing
	} else if terminal {
		return sessionFromStoredResult(existing), existing
	}

	finish := func(result contracts.CommandResult) (domain.Session, contracts.CommandResult) {
		if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
			return domain.Session{}, failed(command.CommandID, "COMMAND_FINISH_FAILED")
		}
		return sessionFromStoredResult(result), result
	}
	if snapshot == nil {
		return finish(rejected(command.CommandID, "SNAPSHOT_REQUIRED", "a published Runtime Snapshot is required", project.ID))
	}
	active, err := s.store.ActiveSessionForProject(ctx, project.ID)
	if err != nil {
		return finish(failed(command.CommandID, "SESSION_LOOKUP_FAILED"))
	}
	if active != nil {
		return finish(rejected(command.CommandID, "SESSION_ALREADY_ACTIVE", "the Project already has an active runtime Session", active.ID))
	}
	if req.Mode == domain.SessionShow {
		if s.showGate == nil {
			return finish(rejected(command.CommandID, "SHOW_PREFLIGHT_REQUIRED", "SHOW entry remains blocked until the S3 Preflight gate is configured", snapshot.ID))
		}
		allowed, reason, err := s.showGate(ctx, project.ID, snapshot.ID)
		if err != nil {
			return finish(failed(command.CommandID, "SHOW_PREFLIGHT_FAILED"))
		}
		if !allowed {
			if strings.TrimSpace(reason) == "" {
				reason = "SHOW Preflight contains a blocking condition"
			}
			return finish(rejected(command.CommandID, "SHOW_PREFLIGHT_BLOCKED", reason, snapshot.ID))
		}
	}

	session, err := s.store.CreateSession(ctx, snapshot.ID, req.Mode, strings.TrimSpace(req.Name))
	if err != nil {
		return finish(resultFromStoreError(command.CommandID, "SESSION_START_FAILED", err, snapshot.ID))
	}
	eventType := "rehearsal.started"
	if req.Mode == domain.SessionShow {
		eventType = "show.entered"
	}
	payload, _ := json.Marshal(map[string]any{"session_id": session.ID, "session_type": session.Type})
	if _, err := s.store.AppendEvent(ctx, &session.ID, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1, Source: "hub.runtime_control",
		ProjectID: project.ID, RuntimeSnapshotID: snapshot.ID, CorrelationID: command.CorrelationID,
		CausationID: command.CommandID, Priority: "P1", TraceContext: json.RawMessage(`{}`), Payload: payload,
	}); err != nil {
		_ = s.store.EndSession(context.WithoutCancel(ctx), session.ID, domain.SessionAborted)
		return finish(failed(command.CommandID, "SESSION_EVENT_FAILED"))
	}
	resultPayload, _ := json.Marshal(map[string]any{
		"session_id": session.ID, "session_type": session.Type,
		"runtime_snapshot_id": session.RuntimeSnapshotID,
	})
	result := contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: resultPayload}
	if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
		return domain.Session{}, failed(command.CommandID, "COMMAND_FINISH_FAILED")
	}
	return session, result
}

func (s *Service) StopSession(ctx context.Context, req StopRequest) contracts.CommandResult {
	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	commandType := CommandRehearsalStop
	if session.Type == domain.SessionShow {
		commandType = CommandShowExit
	}
	command := commandEnvelope(req.RequestID, commandType, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, json.RawMessage(`{}`))
	if existing, terminal, ok := s.reserve(ctx, command); !ok {
		return existing
	} else if terminal {
		return existing
	}
	finish := func(result contracts.CommandResult) contracts.CommandResult {
		if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
			return failed(command.CommandID, "COMMAND_FINISH_FAILED")
		}
		return result
	}
	if session.Status != domain.SessionActive {
		return finish(rejected(command.CommandID, "SESSION_NOT_ACTIVE", "runtime Session is not active", session.ID))
	}
	if err := s.store.EndSession(ctx, session.ID, domain.SessionCompleted); err != nil {
		return finish(resultFromStoreError(command.CommandID, "SESSION_STOP_FAILED", err, session.ID))
	}
	eventType := "rehearsal.stopped"
	if session.Type == domain.SessionShow {
		eventType = "show.exited"
	}
	payload, _ := json.Marshal(map[string]any{"session_id": session.ID, "status": domain.SessionCompleted})
	if _, err := s.store.AppendEvent(ctx, &session.ID, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1, Source: "hub.runtime_control",
		ProjectID: session.ProjectID, RuntimeSnapshotID: session.RuntimeSnapshotID,
		CorrelationID: command.CorrelationID, CausationID: command.CommandID,
		Priority: "P1", TraceContext: json.RawMessage(`{}`), Payload: payload,
	}); err != nil {
		return finish(failed(command.CommandID, "SESSION_EVENT_FAILED"))
	}
	resultPayload, _ := json.Marshal(map[string]any{"session_id": session.ID, "status": domain.SessionCompleted})
	return finish(contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: resultPayload})
}

func (s *Service) Go(ctx context.Context, req CueRequest) contracts.CommandResult {
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "session, issuer and request_id are required", req.SessionID)
	}
	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{
		ExpectedCurrentCueID: req.ExpectedCurrentCueID,
		RequestedNextCueID: req.RequestedCueID,
		OperatorNote: req.OperatorNote,
	})
	command := commandEnvelope(req.RequestID, cueengine.CueGoCommandType, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, payload)

	done := make(chan struct{})
	run := activeRun{requestID: req.RequestID, correlationID: command.CorrelationID, done: done}
	s.mu.Lock()
	if existing, found := s.active[session.ID]; found {
		s.mu.Unlock()
		return s.rejectWhileActive(ctx, command, existing)
	}
	s.active[session.ID] = run
	s.mu.Unlock()
	defer func() {
		s.executor.clear(command.CorrelationID)
		s.mu.Lock()
		if current, ok := s.active[session.ID]; ok && current.requestID == req.RequestID {
			delete(s.active, session.ID)
		}
		close(done)
		s.mu.Unlock()
	}()
	return s.engine.ExecuteCueGo(ctx, session.ID, command)
}

func (s *Service) StopCue(ctx context.Context, req StopRequest) contracts.CommandResult {
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "session, issuer and request_id are required", req.SessionID)
	}
	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	command := commandEnvelope(req.RequestID, CommandCueStop, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, json.RawMessage(`{}`))
	if existing, terminal, ok := s.reserve(ctx, command); !ok {
		return existing
	} else if terminal {
		return existing
	}
	finish := func(result contracts.CommandResult) contracts.CommandResult {
		if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
			return failed(command.CommandID, "COMMAND_FINISH_FAILED")
		}
		return result
	}
	if session.Status != domain.SessionActive {
		return finish(rejected(command.CommandID, "SESSION_NOT_ACTIVE", "runtime Session is not active", session.ID))
	}

	s.mu.Lock()
	run, ok := s.active[session.ID]
	s.mu.Unlock()
	if !ok {
		return finish(rejected(command.CommandID, "NO_RUNNING_CUE", "there is no running Cue to stop", session.ID))
	}
	s.executor.stop(run.correlationID)
	timer := time.NewTimer(defaultStopWait)
	defer timer.Stop()
	select {
	case <-run.done:
		payload, _ := json.Marshal(map[string]any{"session_id": session.ID, "stop_confirmed": true})
		return finish(contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: payload})
	case <-timer.C:
		return finish(contracts.CommandResult{
			CommandID: command.CommandID, Status: contracts.CommandTimedOut,
			Error: &contracts.ContractError{ErrorCode: "STOP_UNCONFIRMED", Category: "TIMEOUT", Message: "stop was requested but the active capability did not terminate within the bounded wait", Retryable: false, AffectedEntityID: session.ID},
		})
	case <-ctx.Done():
		return finish(contracts.CommandResult{
			CommandID: command.CommandID, Status: contracts.CommandCancelled,
			Error: &contracts.ContractError{ErrorCode: "STOP_REQUEST_CANCELLED", Category: "CANCELLED", Message: "stop request context was cancelled", Retryable: false, AffectedEntityID: session.ID},
		})
	}
}

func (s *Service) rejectWhileActive(ctx context.Context, command contracts.CommandEnvelope, active activeRun) contracts.CommandResult {
	if existing, terminal, ok := s.reserve(ctx, command); !ok {
		return existing
	} else if terminal {
		return existing
	}
	result := rejected(command.CommandID, "UNRESOLVED_EXECUTION", "another Cue execution is still active for this Session", active.requestID)
	if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
		return failed(command.CommandID, "COMMAND_FINISH_FAILED")
	}
	return result
}

func (s *Service) reserve(ctx context.Context, command contracts.CommandEnvelope) (contracts.CommandResult, bool, bool) {
	record, reserved, err := s.store.ReserveCommand(ctx, command)
	if err != nil {
		return failed(command.CommandID, "COMMAND_RESERVE_FAILED"), false, false
	}
	if reserved {
		return contracts.CommandResult{}, false, true
	}
	if stored, terminal, err := s.store.StoredCommandResult(record); err == nil && terminal {
		return stored, true, false
	}
	return rejected(record.CommandID, "DUPLICATE_UNRESOLVED", "matching command is already accepted and will not be replayed", record.CommandID), false, false
}

func commandEnvelope(requestID, commandType, projectID, snapshotID, issuer string, payload json.RawMessage) contracts.CommandEnvelope {
	return contracts.CommandEnvelope{
		CommandID: requestID, CommandType: commandType, SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(), ProjectID: projectID, RuntimeSnapshotID: snapshotID,
		Issuer: issuer, CorrelationID: requestID, Priority: "P1", IdempotencyKey: requestID,
		Payload: payload,
	}
}

func sessionFromStoredResult(result contracts.CommandResult) domain.Session {
	if result.Status != contracts.CommandCompleted || len(result.Payload) == 0 {
		return domain.Session{}
	}
	var payload struct {
		SessionID         string             `json:"session_id"`
		SessionType       domain.SessionType `json:"session_type"`
		RuntimeSnapshotID string             `json:"runtime_snapshot_id"`
	}
	if json.Unmarshal(result.Payload, &payload) != nil {
		return domain.Session{}
	}
	return domain.Session{ID: payload.SessionID, Type: payload.SessionType, RuntimeSnapshotID: payload.RuntimeSnapshotID}
}

func rejected(commandID, code, message, entityID string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID, Status: contracts.CommandRejected,
		Error: &contracts.ContractError{ErrorCode: code, Category: "VALIDATION", Message: message, Retryable: false, AffectedEntityID: entityID},
	}
}

func failed(commandID, code string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID, Status: contracts.CommandFailed,
		Error: &contracts.ContractError{ErrorCode: code, Category: "INTERNAL", Message: "internal runtime control failure", Retryable: false},
	}
}

func resultFromStoreError(commandID, code string, err error, entityID string) contracts.CommandResult {
	if errors.Is(err, domain.ErrNotFound) {
		return rejected(commandID, "NOT_FOUND", "requested runtime entity was not found", entityID)
	}
	if errors.Is(err, domain.ErrConflict) {
		return rejected(commandID, "CONFLICT", "runtime state conflicts with the requested operation", entityID)
	}
	return failed(commandID, code)
}

type stoppableExecutor struct {
	inner capability.Executor
	mu sync.Mutex
	active map[string]map[string]context.CancelFunc
	stopped map[string]bool
}

func newStoppableExecutor(inner capability.Executor) *stoppableExecutor {
	return &stoppableExecutor{inner: inner, active: make(map[string]map[string]context.CancelFunc), stopped: make(map[string]bool)}
}

func (e *stoppableExecutor) Execute(ctx context.Context, req capability.Request) capability.Result {
	if e == nil || e.inner == nil {
		return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: "CAPABILITY_UNAVAILABLE", ResponseSummary: "runtime executor is unavailable"}
	}
	executionCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	if e.active[req.CorrelationID] == nil {
		e.active[req.CorrelationID] = make(map[string]context.CancelFunc)
	}
	e.active[req.CorrelationID][req.ExecutionID] = cancel
	if e.stopped[req.CorrelationID] {
		cancel()
	}
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		if executions := e.active[req.CorrelationID]; executions != nil {
			delete(executions, req.ExecutionID)
			if len(executions) == 0 {
				delete(e.active, req.CorrelationID)
			}
		}
		e.mu.Unlock()
	}()
	return e.inner.Execute(executionCtx, req)
}

func (e *stoppableExecutor) stop(correlationID string) {
	e.mu.Lock()
	e.stopped[correlationID] = true
	cancels := make([]context.CancelFunc, 0, len(e.active[correlationID]))
	for _, cancel := range e.active[correlationID] {
		cancels = append(cancels, cancel)
	}
	e.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (e *stoppableExecutor) clear(correlationID string) {
	e.mu.Lock()
	delete(e.stopped, correlationID)
	delete(e.active, correlationID)
	e.mu.Unlock()
}

var _ capability.Executor = (*stoppableExecutor)(nil)
var _ = fmt.Sprintf
