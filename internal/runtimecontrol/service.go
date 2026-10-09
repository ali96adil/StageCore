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

	mu           sync.Mutex
	active       map[string]map[string]activeRun
	nextSequence uint64
	// Keep Cue selection ordering brief; do not hold while Actions run.
	selectionMu  sync.Mutex
}

type activeRun struct {
	requestID     string
	correlationID string
	done          chan struct{}
	cancel        context.CancelFunc
	sequence      uint64
	resources     []string
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
	onSelected           func()
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
	// Serialize GO selection against session shutdown and safety latch.
	s.selectionMu.Lock()
	defer s.selectionMu.Unlock()

	if session.Status != domain.SessionActive {
		return finish(rejected(command.CommandID, "SESSION_NOT_ACTIVE", "runtime Session is not active", session.ID))
	}
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
	// Serialize GO selection against session shutdown and safety latch.
	s.selectionMu.Lock()
	defer s.selectionMu.Unlock()

	if session.Status != domain.SessionActive {
		return finish(rejected(command.CommandID, "SESSION_NOT_ACTIVE", "runtime Session is not active", session.ID))
	}

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
	// Multiple Cues may execute simultaneously, but selecting/advancing each
	// one remains serialized to prevent two rapid GOs from selecting Cue N+1.
	s.selectionMu.Lock()
	var release sync.Once
	unlock := func() { release.Do(s.selectionMu.Unlock) }
	defer unlock()

	session, err := s.store.GetSession(ctx, req.SessionID)
	if err != nil {
		return resultFromStoreError(req.RequestID, "SESSION_LOOKUP_FAILED", err, req.SessionID)
	}
	if session.Status != domain.SessionActive {
		return rejected(req.RequestID, "SESSION_NOT_ACTIVE", "runtime Session is not active", session.ID)
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
	// Idempotent GO replay must be resolved before selecting another Cue.
	// Otherwise retrying the final Cue can produce NO_NEXT_CUE rather than
	// returning the stored terminal command result.
	if record, found, err := s.store.FindCommandRecord(ctx, command); err != nil {
		return failed(req.RequestID, "COMMAND_LOOKUP_FAILED")
	} else if found {
		// A retry during an in-flight GO is not a new request and must not
		// advance the Cue or be reported as an error. Do not claim that
		// current_cue_id was already advanced: the original GO may still
		// be finalizing its durable selection.
		if result, terminal, resultErr := s.store.StoredCommandResult(record); resultErr != nil {
			return failed(req.RequestID, "COMMAND_RESULT_LOOKUP_FAILED")
		} else if terminal {
			return result
		}
		return contracts.CommandResult{
			CommandID: record.CommandID, Status: contracts.CommandAccepted,
			Payload: json.RawMessage(`{"already_accepted":true,"execution_continues":true}`),
		}
	}

	// Resolve all linked Cue outputs while GO selection remains fenced.
	// Same-target output overlap is rejected rather than silently racing
	// late old actions against a newer Cue's desired device state.
	resources, previewRejected, previewErr := s.engine.PreviewCueOutputTargets(
		ctx, session, cueengine.CueGoPayload{
			ExpectedCurrentCueID: req.ExpectedCurrentCueID,
			RequestedNextCueID: req.RequestedCueID,
			OperatorNote: req.OperatorNote,
		})
	if previewErr != nil {
		return failed(req.RequestID, "CUE_OUTPUT_PREVIEW_FAILED")
	}
	if previewRejected != nil {
		previewRejected.CommandID = req.RequestID
		return *previewRejected
	}

	done := make(chan struct{})
	delayStop, cancel := context.WithCancel(context.Background())
	runCtx := cueengine.WithSelectionHook(cueengine.WithDelayStop(ctx, delayStop.Done()), func() {
		unlock()
		if req.onSelected != nil { req.onSelected() }
	})

	s.mu.Lock()
	s.nextSequence++
	run := activeRun{
		requestID: req.RequestID, correlationID: command.CorrelationID,
		done: done, cancel: cancel, sequence: s.nextSequence,
		resources: resources,
	}
	// Resource ownership is global to this Hub, not only to a Session.
	// Two projects can refer to the same physical OSC endpoint or Companion.
	for _, sessionRuns := range s.active {
		for _, other := range sessionRuns {
			if conflict := overlappingTarget(resources, other.resources); conflict != "" {
				s.mu.Unlock()
				cancel()
				return s.rejectOutputConflict(ctx, command, other.requestID, conflict)
			}
		}
	}
	if s.active[session.ID] == nil {
		s.active[session.ID] = make(map[string]activeRun)
	}
	if _, alreadyActive := s.active[session.ID][req.RequestID]; alreadyActive {
		s.mu.Unlock()
		cancel()
		return rejected(req.RequestID, "DUPLICATE_UNRESOLVED", "Cue request is already in progress", req.RequestID)
	}
	s.active[session.ID][req.RequestID] = run
	s.mu.Unlock()

	defer func() {
		cancel()
		s.executor.clear(command.CorrelationID)
		s.mu.Lock()
		delete(s.active[session.ID], req.RequestID)
		if len(s.active[session.ID]) == 0 {
			delete(s.active, session.ID)
		}
		close(done)
		s.mu.Unlock()
	}()
	return s.engine.ExecuteCueGo(runCtx, session.ID, command)
}

// overlappingTarget is a conservative in-flight resource fence. Two Cues
// writing one physical target cannot overlap while an earlier one is running.
// Empty-resource Cues (e.g. sim.test-only) remain freely concurrent.
func overlappingTarget(a, b []string) string {
	for _, left := range a {
		for _, right := range b {
			if left == right || left == "*" || right == "*" {
				if left == "*" || right == "*" { return "UNRESOLVED_TARGET" }
				return left
			}
		}
	}
	return ""
}

func (s *Service) rejectOutputConflict(
	ctx context.Context, command contracts.CommandEnvelope, priorRequestID, target string,
) contracts.CommandResult {
	if existing, terminal, ok := s.reserve(ctx, command); !ok {
		return existing
	} else if terminal { return existing }
	result := rejected(command.CommandID, "OUTPUT_BUSY",
		"another Cue execution still controls this output target; wait or STOP the conflicting Cue",
		target)
	if err := s.store.FinishCommand(ctx, command.CommandID, result); err != nil {
		return failed(command.CommandID, "COMMAND_FINISH_FAILED")
	}
	return result
}

// QueueGo provides a fast, durable GO acceptance for the operator interface:
// return ACCEPTED only after current_cue_id was persisted. The run continues
// under Hub-owned context, independent from the original HTTP request.
func (s *Service) QueueGo(ctx context.Context, req CueRequest) contracts.CommandResult {
	if ctx == nil {
		return rejected(req.RequestID, "RUNTIME_CONTEXT_REQUIRED", "request context is missing", req.SessionID)
	}
	selected := make(chan struct{}, 1)
	terminal := make(chan contracts.CommandResult, 1)
	req.onSelected = func() {
		select { case selected <- struct{}{}: default: }
	}
	go func() {
		terminal <- s.Go(context.WithoutCancel(ctx), req)
	}()
	select {
	case <-selected:
		return contracts.CommandResult{
			CommandID: req.RequestID, Status: contracts.CommandAccepted,
			Payload: json.RawMessage(`{"cue_selected":true,"execution_continues":true}`),
		}
	case result := <-terminal:
		return result
	case <-ctx.Done():
		// The Hub-owned operation may still complete; idempotent request_id is
		// required for a retry. Never claim a cancelled HTTP request stopped GO.
		return failed(req.RequestID, "GO_ACCEPTANCE_UNCONFIRMED")
	}
}

// latestActiveRun is the last GO still running, NOT necessarily the latest
// completed Cue. STOP CUE only cancels this execution. STOP SESSION and
// Emergency Blackout cancel all outstanding executions.
func (s *Service) latestActiveRun(sessionID string) (activeRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest activeRun
	found := false
	for _, run := range s.active[sessionID] {
		if !found || run.sequence > latest.sequence {
			latest = run
			found = true
		}
	}
	return latest, found
}

func (s *Service) activeRunsForSession(sessionID string) []activeRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := make([]activeRun, 0, len(s.active[sessionID]))
	for _, run := range s.active[sessionID] {
		runs = append(runs, run)
	}
	return runs
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
	// Serialize GO selection against session shutdown and safety latch.
	s.selectionMu.Lock()
	defer s.selectionMu.Unlock()

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

	// Fence STOP selection against concurrent GO selection, but release
	// before waiting for a long-running adapter. STOP applies to the latest
	// *selected* Cue at this serialized point in time.
	s.selectionMu.Lock()
	run, ok := s.latestActiveRun(session.ID)
	if !ok {
		s.selectionMu.Unlock()
		return finish(rejected(command.CommandID, "NO_RUNNING_CUE", "there is no running Cue to stop", session.ID))
	}
	s.executor.stop(run.correlationID)
	if run.cancel != nil { run.cancel() }
	s.selectionMu.Unlock()
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

func (s *Service) stopActiveCueForSession(ctx context.Context, sessionID string) error {
	runs := s.activeRunsForSession(sessionID)
	if len(runs) == 0 {
		return nil
	}
	// Stop *every* execution before waiting. The same bound applies to the
	// whole batch; a slow older Cue cannot hide behind the current Cue.
	for _, run := range runs {
		s.executor.stop(run.correlationID)
		if run.cancel != nil { run.cancel() }
	}
	return waitForActiveRunStops(ctx, runs, defaultStopWait)
}

// waitForActiveRunStops uses a level-triggered deadline (context.Done) rather
// than reading time.Timer.C in a loop. A timer's single signal can otherwise
// be consumed by a select racing with a just-completed Cue, leaving a later
// unfinished Cue blocked forever after the timeout has already fired.
func waitForActiveRunStops(ctx context.Context, runs []activeRun, timeout time.Duration) error {
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for _, run := range runs {
		select {
		case <-run.done:
		case <-bounded.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("session stop was cancelled while waiting for Cue executions: %w", err)
			}
			return fmt.Errorf("%d active Cue execution(s) did not terminate within the bounded stop wait", len(runs))
		}
	}
	return nil
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
	// If STOP already won the registration race, never invoke the device
	// adapter at all. The adapter may not be cancellation-aware before its
	// first network write. Cancellation after this check still requires
	// adapter/device-level fencing and remains a release blocker.
	if executionCtx.Err() != nil {
		return capability.Result{
			Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone,
			ErrorCode: "CANCELLED", ResponseSummary: "Cue stopped before capability dispatch",
		}
	}
	return e.inner.Execute(executionCtx, req)
}

func (e *stoppableExecutor) stop(correlationID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped[correlationID] = true
	// Latch and cancel every registered dispatch before releasing the
	// registration lock. A concurrent Execute must observe STOP as soon as
	// it acquires this lock; no registered context remains uncancelled.
	// Device adapters still need their own pre-write cancellation fence.
	for _, cancel := range e.active[correlationID] {
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
