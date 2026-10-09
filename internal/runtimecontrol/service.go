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
	CommandCueStop              = "cue.stop"
	CommandEmergencyBlackout    = "runtime.emergency_blackout.set"
	CommandProjectBlackout      = "runtime.project_blackout.set"
	CommandRehearsalStart       = "rehearsal.start"
	CommandRehearsalStop        = "rehearsal.stop"
	CommandRehearsalForceStop   = "rehearsal.force_stop"
	CommandShowEnter            = "show.enter"
	CommandShowExit             = "show.exit"
	CommandShowForceExit        = "show.force_exit"

	defaultStopWait = 2 * time.Second
)

type SessionStartGate func(context.Context, string, string) (bool, string, error)
type ShowGate func(context.Context, string, string) (bool, string, error)
type SessionStopSafety func(context.Context, domain.Session, contracts.CommandEnvelope) error
type EmergencySafety func(context.Context, domain.Session, contracts.CommandEnvelope, bool) (json.RawMessage, error)

type Option func(*Service)

func WithSessionStartGate(gate SessionStartGate) Option {
	return func(s *Service) { s.startGate = gate }
}

func WithShowGate(gate ShowGate) Option {
	return func(s *Service) { s.showGate = gate }
}

func WithSessionStopSafety(safety SessionStopSafety) Option {
	return func(s *Service) { s.stopSafety = safety }
}

func WithEmergencySafety(safety EmergencySafety) Option {
	return func(s *Service) { s.emergencySafety = safety }
}

type Service struct {
	store    *store.Store
	engine   *cueengine.Engine
	executor *stoppableExecutor
	startGate       SessionStartGate
	showGate        ShowGate
	stopSafety      SessionStopSafety
	emergencySafety EmergencySafety

	mu     sync.Mutex
	active map[string]map[string]activeRun
	// Block GO while a STOP/Session exit is in flight so no new Cue is
	// admitted after the STOP snapshot has been taken.
	stopping map[string]bool
}

type activeRun struct {
	requestID     string
	correlationID string
	done          chan struct{}
	cancel        context.CancelFunc
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

type EmergencyRequest struct {
	SessionID string
	Issuer    string
	RequestID string
	Enabled   bool
}

type ProjectEmergencyRequest struct {
	ProjectID string
	Issuer    string
	RequestID string
	Enabled   bool
}

func New(s *store.Store, executor capability.Executor, options ...Option) *Service {
	stoppable := newStoppableExecutor(executor)
	service := &Service{
		store: s, executor: stoppable,
		engine: cueengine.NewWithExecutor(s, stoppable),
		active: make(map[string]map[string]activeRun),
		stopping: make(map[string]bool),
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
	startWarnings := make([]string, 0, 2)
	deviceScopeWarning := ""
	preflightWarning := ""
	if s.startGate != nil {
		allowed, reason, err := s.startGate(ctx, project.ID, snapshot.ID)
		if err != nil {
			return finish(failed(command.CommandID, "SESSION_DEVICE_SCOPE_GATE_FAILED"))
		}
		if !allowed {
			if strings.TrimSpace(reason) == "" {
				reason = "managed Stage Device Session-start invariant failed"
			}
			return finish(rejected(command.CommandID, "SESSION_DEVICE_SCOPE_BLOCKED", reason, snapshot.ID))
		}
		deviceScopeWarning = strings.TrimSpace(reason)
		if deviceScopeWarning != "" {
			startWarnings = append(startWarnings, deviceScopeWarning)
		}
	}
	if req.Mode == domain.SessionShow {
		if s.showGate == nil {
			return finish(rejected(command.CommandID, "SHOW_PREFLIGHT_REQUIRED", "SHOW entry requires the Preflight evaluator to be configured", snapshot.ID))
		}
		allowed, reason, err := s.showGate(ctx, project.ID, snapshot.ID)
		if err != nil {
			return finish(failed(command.CommandID, "SHOW_PREFLIGHT_FAILED"))
		}
		if !allowed {
			reason = strings.TrimSpace(reason)
			if reason == "" {
				reason = "SHOW Preflight contains a structural blocking condition"
			}
			return finish(rejected(command.CommandID, "SHOW_PREFLIGHT_BLOCKED", reason, snapshot.ID))
		}
		preflightWarning = strings.TrimSpace(reason)
		if preflightWarning != "" {
			startWarnings = append(startWarnings, preflightWarning)
		}
	}
	startWarning := strings.Join(startWarnings, " ")

	session, err := s.store.CreateSession(ctx, snapshot.ID, req.Mode, strings.TrimSpace(req.Name))
	if err != nil {
		return finish(resultFromStoreError(command.CommandID, "SESSION_START_FAILED", err, snapshot.ID))
	}
	eventType := "rehearsal.started"
	if req.Mode == domain.SessionShow {
		eventType = "show.entered"
	}
	eventPayload := map[string]any{"session_id": session.ID, "session_type": session.Type}
	if startWarning != "" {
		eventPayload["degraded_start"] = true
		eventPayload["degraded_reasons"] = append([]string(nil), startWarnings...)
	}
	if deviceScopeWarning != "" {
		eventPayload["device_scope_warning"] = deviceScopeWarning
	}
	if preflightWarning != "" {
		eventPayload["preflight_warning"] = preflightWarning
	}
	payload, _ := json.Marshal(eventPayload)
	if _, err := s.store.AppendEvent(ctx, &session.ID, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1, Source: "hub.runtime_control",
		ProjectID: project.ID, RuntimeSnapshotID: snapshot.ID, CorrelationID: command.CorrelationID,
		CausationID: command.CommandID, Priority: "P1", TraceContext: json.RawMessage(`{}`), Payload: payload,
	}); err != nil {
		_ = s.store.EndSession(context.WithoutCancel(ctx), session.ID, domain.SessionAborted)
		return finish(failed(command.CommandID, "SESSION_EVENT_FAILED"))
	}
	resultData := map[string]any{
		"session_id": session.ID, "session_type": session.Type,
		"runtime_snapshot_id": session.RuntimeSnapshotID,
	}
	if startWarning != "" {
		resultData["degraded_start"] = true
		resultData["degraded_reasons"] = append([]string(nil), startWarnings...)
	}
	if deviceScopeWarning != "" {
		resultData["device_scope_warning"] = deviceScopeWarning
	}
	if preflightWarning != "" {
		resultData["preflight_warning"] = preflightWarning
	}
	resultPayload, _ := json.Marshal(resultData)
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
	if !s.beginStoppingCues(session.ID) {
		return finish(rejected(command.CommandID, "CUE_STOP_IN_PROGRESS", "another Cue STOP or Session exit is in progress", session.ID))
	}
	defer s.endStoppingCues(session.ID)
	if err := s.stopActiveCueForSession(ctx, session.ID); err != nil {
		return finish(sessionStopSafetyFailure(command.CommandID, "SESSION_STOP_CUE_UNCONFIRMED", err.Error(), session.ID))
	}
	if s.stopSafety != nil {
		if err := s.stopSafety(ctx, session, command); err != nil {
			return finish(sessionStopSafetyFailure(command.CommandID, "SESSION_STOP_SAFETY_FAILED", err.Error(), session.ID))
		}
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

func (s *Service) ForceStopSession(ctx context.Context, req StopRequest) contracts.CommandResult {
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "session, issuer and request_id are required", req.SessionID)
	}
	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	commandType := CommandRehearsalForceStop
	eventType := "rehearsal.force_stopped"
	if session.Type == domain.SessionShow {
		commandType = CommandShowForceExit
		eventType = "show.force_exited"
	}
	payload, _ := json.Marshal(map[string]any{"forced": true, "safety_bypassed": true})
	command := commandEnvelope(req.RequestID, commandType, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, payload)
	command.Priority = "P0"
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
	if !s.beginStoppingCues(session.ID) {
		return finish(rejected(command.CommandID, "CUE_STOP_IN_PROGRESS", "another Cue STOP or Session exit is in progress", session.ID))
	}
	defer s.endStoppingCues(session.ID)

	cueStopErr := s.stopActiveCueForSession(ctx, session.ID)
	reason := "FORCED_OPERATOR_EXIT_WITHOUT_CONFIRMED_SAFE_STATE"
	if err := s.store.EndSessionLifecycle(context.WithoutCancel(ctx), session.ID, domain.SessionLifecycleAborted, reason); err != nil {
		return finish(resultFromStoreError(command.CommandID, "SESSION_FORCE_STOP_FAILED", err, session.ID))
	}

	resultPayloadMap := map[string]any{
		"session_id": session.ID,
		"status": domain.SessionAborted,
		"forced": true,
		"safety_bypassed": true,
		"cue_stop_confirmed": cueStopErr == nil,
	}
	if cueStopErr != nil {
		resultPayloadMap["cue_stop_error"] = cueStopErr.Error()
	}
	resultPayload, _ := json.Marshal(resultPayloadMap)
	_, _ = s.store.AppendEvent(context.WithoutCancel(ctx), &session.ID, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1, Source: "hub.runtime_control",
		ProjectID: session.ProjectID, RuntimeSnapshotID: session.RuntimeSnapshotID,
		CorrelationID: command.CorrelationID, CausationID: command.CommandID,
		Priority: "P0", TraceContext: json.RawMessage(`{}`), Payload: resultPayload,
	})
	return finish(contracts.CommandResult{
		CommandID: command.CommandID,
		Status: contracts.CommandCompleted,
		Payload: resultPayload,
	})
}

func (s *Service) ProjectBlackout(ctx context.Context, req ProjectEmergencyRequest) contracts.CommandResult {
	if strings.TrimSpace(req.ProjectID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "project, issuer and request_id are required", req.ProjectID)
	}
	project, err := s.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "PROJECT_LOOKUP_FAILED", err, req.ProjectID)
	}
	if active, err := s.store.ActiveSessionForProject(ctx, project.ID); err != nil {
		return failed(req.RequestID, "SESSION_LOOKUP_FAILED")
	} else if active != nil {
		return rejected(req.RequestID, "SESSION_ALREADY_ACTIVE", "use the Session Emergency Blackout while REHEARSAL or SHOW is active", active.ID)
	}
	snapshot, err := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil {
		return failed(req.RequestID, "SNAPSHOT_LOOKUP_FAILED")
	}
	if snapshot == nil {
		return rejected(req.RequestID, "SNAPSHOT_REQUIRED", "a published Runtime Snapshot is required", project.ID)
	}

	payload, _ := json.Marshal(map[string]any{"enabled": req.Enabled, "sessionless": true})
	command := commandEnvelope(req.RequestID, CommandProjectBlackout, project.ID, snapshot.ID, req.Issuer, payload)
	command.Priority = "P0"
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
	if s.emergencySafety == nil {
		return finish(contracts.CommandResult{
			CommandID: command.CommandID,
			Status: contracts.CommandFailed,
			Error: &contracts.ContractError{
				ErrorCode: "PROJECT_BLACKOUT_UNAVAILABLE", Category: "SAFETY",
				Message: "managed-output project blackout safety is not configured",
				Retryable: false, AffectedEntityID: project.ID,
			},
		})
	}

	synthetic := domain.Session{
		ProjectID: project.ID,
		RuntimeSnapshotID: snapshot.ID,
		Type: domain.SessionRehearsal,
		Status: domain.SessionActive,
	}
	report, safetyErr := s.emergencySafety(ctx, synthetic, command, req.Enabled)
	eventType := "runtime.edit_blackout.cleared"
	if req.Enabled {
		eventType = "runtime.edit_blackout.applied"
	}
	eventPayload := report
	if len(eventPayload) == 0 {
		eventPayload = json.RawMessage(`{}`)
	}
	_, _ = s.store.AppendEvent(context.WithoutCancel(ctx), nil, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1, Source: "hub.runtime_control",
		ProjectID: project.ID, RuntimeSnapshotID: snapshot.ID,
		CorrelationID: command.CorrelationID, CausationID: command.CommandID,
		Priority: "P0", TraceContext: json.RawMessage(`{}`), Payload: eventPayload,
	})
	if safetyErr != nil {
		return finish(contracts.CommandResult{
			CommandID: command.CommandID,
			Status: contracts.CommandFailed,
			Payload: eventPayload,
			Error: &contracts.ContractError{
				ErrorCode: "PROJECT_BLACKOUT_PARTIAL_FAILURE", Category: "SAFETY",
				Message: "sessionless managed-output blackout completed with a partial failure: " + safetyErr.Error(),
				Retryable: true, AffectedEntityID: project.ID,
			},
		})
	}
	return finish(contracts.CommandResult{
		CommandID: command.CommandID,
		Status: contracts.CommandCompleted,
		Payload: eventPayload,
	})
}

func (s *Service) Go(ctx context.Context, req CueRequest) contracts.CommandResult {
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "session, issuer and request_id are required", req.SessionID)
	}
	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	blackout, err := s.store.SessionManagedOutputBlackout(ctx, session.ID)
	if err != nil {
		return failed(req.RequestID, "EMERGENCY_BLACKOUT_STATE_FAILED")
	}
	if blackout {
		return rejected(req.RequestID, "EMERGENCY_BLACKOUT_ACTIVE", "managed-output Emergency Blackout is active; clear it explicitly before GO", session.ID)
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{
		ExpectedCurrentCueID: req.ExpectedCurrentCueID,
		RequestedNextCueID: req.RequestedCueID,
		OperatorNote: req.OperatorNote,
	})
	command := commandEnvelope(req.RequestID, cueengine.CueGoCommandType, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, payload)

	done := make(chan struct{})
	// STOP cancels only Cue delays. Command persistence must retain the live
	// request context so terminal results can still be durably recorded.
	delayStop, cancel := context.WithCancel(context.Background())
	runCtx := cueengine.WithDelayStop(ctx, delayStop.Done())
	run := activeRun{requestID: req.RequestID, correlationID: command.CorrelationID, done: done, cancel: cancel}
	s.mu.Lock()
	if s.stopping[session.ID] {
		s.mu.Unlock()
		cancel()
		return rejected(req.RequestID, "CUE_STOP_IN_PROGRESS", "a Cue STOP or Session exit is in progress", session.ID)
	}
	runs := s.active[session.ID]
	// Validate orphan RUNNING rows while holding the admission lock. The
	// first admitted run may reach the engine after another concurrent GO;
	// checking for orphans inside that first engine goroutine would race.
	if len(runs) == 0 {
		running, err := s.store.HasRunningCueExecution(ctx, session.ID)
		if err != nil {
			s.mu.Unlock()
			cancel()
			return failed(req.RequestID, "RUNNING_EXECUTION_CHECK_FAILED")
		}
		if running {
			s.mu.Unlock()
			cancel()
			return rejected(req.RequestID, "UNRESOLVED_EXECUTION", "a Cue execution from an earlier runtime is still unresolved", session.ID)
		}
	}
	// Every request admitted through this service uses its serialized,
	// persisted admission decision. Direct CueEngine requests remain strict.
	runCtx = cueengine.WithConcurrentCueGo(runCtx)
	if runs == nil {
		runs = make(map[string]activeRun)
		s.active[session.ID] = runs
	}
	if _, duplicate := runs[req.RequestID]; duplicate {
		s.mu.Unlock()
		cancel()
		return rejected(req.RequestID, "DUPLICATE_UNRESOLVED", "this Cue GO request is already active", req.RequestID)
	}
	runs[req.RequestID] = run
	s.mu.Unlock()
	defer func() {
		cancel()
		s.executor.clear(command.CorrelationID)
		s.mu.Lock()
		if current := s.active[session.ID]; current != nil {
			delete(current, req.RequestID)
			if len(current) == 0 {
				delete(s.active, session.ID)
			}
		}
		close(done)
		s.mu.Unlock()
	}()
	return s.engine.ExecuteCueGo(runCtx, session.ID, command)
}

func (s *Service) EmergencyBlackout(ctx context.Context, req EmergencyRequest) contracts.CommandResult {
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Issuer) == "" || strings.TrimSpace(req.RequestID) == "" {
		return rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "session, issuer and request_id are required", req.SessionID)
	}
	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	payload, _ := json.Marshal(map[string]any{"enabled": req.Enabled})
	command := commandEnvelope(req.RequestID, CommandEmergencyBlackout, session.ProjectID, session.RuntimeSnapshotID, req.Issuer, payload)
	command.Priority = "P0"
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
	if s.emergencySafety == nil {
		return finish(contracts.CommandResult{
			CommandID: command.CommandID,
			Status: contracts.CommandFailed,
			Error: &contracts.ContractError{
				ErrorCode: "EMERGENCY_BLACKOUT_UNAVAILABLE", Category: "SAFETY",
				Message: "managed-output Emergency Blackout safety is not configured",
				Retryable: false, AffectedEntityID: session.ID,
			},
		})
	}

	current, stateErr := s.store.SessionManagedOutputBlackout(ctx, session.ID)
	if stateErr != nil {
		return finish(failed(command.CommandID, "EMERGENCY_BLACKOUT_STATE_FAILED"))
	}
	if !req.Enabled && !current {
		return finish(rejected(command.CommandID, "EMERGENCY_BLACKOUT_NOT_ACTIVE", "managed-output Emergency Blackout is not active", session.ID))
	}

	// Activation is latched durably BEFORE cancelling the current Cue or touching
	// outputs. Any partial failure therefore keeps GO blocked across Hub restart.
	if req.Enabled {
		if err := s.store.SetSessionManagedOutputBlackout(ctx, session.ID, true, req.Issuer); err != nil {
			return finish(resultFromStoreError(command.CommandID, "EMERGENCY_BLACKOUT_LATCH_FAILED", err, session.ID))
		}
	}

	var cueStopErr error
	if req.Enabled {
		cueStopErr = s.stopActiveCueForSession(ctx, session.ID)
	}

	report, safetyErr := s.emergencySafety(ctx, session, command, req.Enabled)
	if !req.Enabled && safetyErr == nil {
		if err := s.store.SetSessionManagedOutputBlackout(ctx, session.ID, false, req.Issuer); err != nil {
			safetyErr = fmt.Errorf("clear emergency blackout latch: %w", err)
		}
	}

	eventType := "runtime.emergency_blackout.cleared"
	if req.Enabled {
		eventType = "runtime.emergency_blackout.applied"
	}
	eventPayload := report
	if len(eventPayload) == 0 {
		eventPayload = json.RawMessage(`{}`)
	}
	_, _ = s.store.AppendEvent(context.WithoutCancel(ctx), &session.ID, contracts.EventEnvelope{
		EventType: eventType, SchemaVersion: contracts.SchemaVersion1, Source: "hub.runtime_control",
		ProjectID: session.ProjectID, RuntimeSnapshotID: session.RuntimeSnapshotID,
		CorrelationID: command.CorrelationID, CausationID: command.CommandID,
		Priority: "P0", TraceContext: json.RawMessage(`{}`), Payload: eventPayload,
	})

	if cueStopErr != nil || safetyErr != nil {
		message := "managed-output Emergency Blackout completed with a partial failure"
		if !req.Enabled {
			message = "managed-output Emergency Blackout clear failed; safety latch remains active"
		}
		details := make([]string, 0, 2)
		if cueStopErr != nil {
			details = append(details, "Cue stop unconfirmed: "+cueStopErr.Error())
		}
		if safetyErr != nil {
			details = append(details, safetyErr.Error())
		}
		return finish(contracts.CommandResult{
			CommandID: command.CommandID, Status: contracts.CommandFailed, Payload: eventPayload,
			Error: &contracts.ContractError{
				ErrorCode: "EMERGENCY_BLACKOUT_PARTIAL_FAILURE", Category: "SAFETY",
				Message: message + ": " + strings.Join(details, "; "),
				Retryable: true, AffectedEntityID: session.ID,
			},
		})
	}

	return finish(contracts.CommandResult{
		CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: eventPayload,
	})
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

	if !s.beginStoppingCues(session.ID) {
		return finish(rejected(command.CommandID, "CUE_STOP_IN_PROGRESS", "another Cue STOP or Session exit is in progress", session.ID))
	}
	defer s.endStoppingCues(session.ID)
	runs := s.activeRunsForSession(session.ID)
	if len(runs) == 0 {
		return finish(rejected(command.CommandID, "NO_RUNNING_CUE", "there is no running Cue to stop", session.ID))
	}
	s.interruptCueRuns(runs)
	timer := time.NewTimer(defaultStopWait)
	defer timer.Stop()
	for _, run := range runs {
		select {
		case <-run.done:
		case <-timer.C:
			return finish(contracts.CommandResult{
				CommandID: command.CommandID, Status: contracts.CommandTimedOut,
				Error: &contracts.ContractError{ErrorCode: "STOP_UNCONFIRMED", Category: "TIMEOUT", Message: "STOP requested but not all active Cue executions terminated within the bounded wait", Retryable: false, AffectedEntityID: session.ID},
			})
		case <-ctx.Done():
			return finish(contracts.CommandResult{
				CommandID: command.CommandID, Status: contracts.CommandCancelled,
				Error: &contracts.ContractError{ErrorCode: "STOP_REQUEST_CANCELLED", Category: "CANCELLED", Message: "stop request context was cancelled", Retryable: false, AffectedEntityID: session.ID},
			})
		}
	}
	payload, _ := json.Marshal(map[string]any{"session_id": session.ID, "stop_confirmed": true, "stopped_cue_count": len(runs)})
	return finish(contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: payload})
}

// beginStoppingCues prevents newly admitted GO executions while STOP is
// canceling its snapshot. It also fences a normal or forced Session exit.
func (s *Service) beginStoppingCues(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping[sessionID] {
		return false
	}
	s.stopping[sessionID] = true
	return true
}

func (s *Service) endStoppingCues(sessionID string) {
	s.mu.Lock()
	delete(s.stopping, sessionID)
	s.mu.Unlock()
}

// activeRunsForSession snapshots every active Cue execution. STOP and session
// shutdown target all of them; a later GO never implicitly stops an earlier one.
func (s *Service) activeRunsForSession(sessionID string) []activeRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	byRequest := s.active[sessionID]
	runs := make([]activeRun, 0, len(byRequest))
	for _, run := range byRequest {
		runs = append(runs, run)
	}
	return runs
}

func (s *Service) interruptCueRuns(runs []activeRun) {
	for _, run := range runs {
		s.executor.stop(run.correlationID)
		if run.cancel != nil {
			run.cancel()
		}
	}
}

func (s *Service) stopActiveCueForSession(ctx context.Context, sessionID string) error {
	runs := s.activeRunsForSession(sessionID)
	if len(runs) == 0 {
		return nil
	}
	s.interruptCueRuns(runs)
	timer := time.NewTimer(defaultStopWait)
	defer timer.Stop()
	for _, run := range runs {
		select {
		case <-run.done:
		case <-timer.C:
			return fmt.Errorf("not all active Cues terminated within the bounded stop wait")
		case <-ctx.Done():
			return fmt.Errorf("session stop was cancelled while waiting for the active Cues: %w", ctx.Err())
		}
	}
	return nil
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

func sessionStopSafetyFailure(commandID, code, message, entityID string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID, Status: contracts.CommandFailed,
		Error: &contracts.ContractError{
			ErrorCode: code, Category: "SAFETY", Message: message,
			Retryable: true, AffectedEntityID: entityID,
		},
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
