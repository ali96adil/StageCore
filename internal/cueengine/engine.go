package cueengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

const CueGoCommandType = "cue.go"

type CueGoPayload struct {
	ExpectedCurrentCueID *string `json:"expected_current_cue_id"`
	RequestedNextCueID   *string `json:"requested_next_cue_id"`
	OperatorNote         *string `json:"operator_note"`
}

type Engine struct {
	store    *store.Store
	executor capability.Executor
}

func New(s *store.Store) *Engine {
	registry := capability.NewRegistry()
	_ = registry.Register("sim.test", simulator.Adapter{})
	return &Engine{store: s, executor: registry}
}

func NewWithExecutor(s *store.Store, executor capability.Executor) *Engine {
	if executor == nil {
		return New(s)
	}
	return &Engine{store: s, executor: executor}
}

func (e *Engine) ExecuteCueGo(ctx context.Context, sessionID string, command contracts.CommandEnvelope) contracts.CommandResult {
	if result := validateEnvelope(command); result != nil {
		return *result
	}
	if existing, found, err := e.store.FindCommandRecord(ctx, command); err != nil {
		return internalFailure(command.CommandID, "COMMAND_LOOKUP_FAILED", err)
	} else if found {
		return e.duplicateResult(existing)
	}

	record, reserved, err := e.store.ReserveCommand(ctx, command)
	if err != nil {
		return internalFailure(command.CommandID, "COMMAND_RESERVE_FAILED", err)
	}
	if !reserved {
		return e.duplicateResult(record)
	}
	if command.CorrelationID == "" {
		command.CorrelationID = record.CorrelationID
	}
	result := e.executeReserved(ctx, sessionID, command)
	if err := e.store.FinishCommand(ctx, command.CommandID, result); err != nil {
		return internalFailure(command.CommandID, "COMMAND_FINISH_FAILED", err)
	}
	return result
}

func (e *Engine) executeReserved(ctx context.Context, sessionID string, command contracts.CommandEnvelope) contracts.CommandResult {
	session, err := e.store.GetSession(ctx, sessionID)
	if err != nil {
		return rejection(command.CommandID, "SESSION_NOT_FOUND", "VALIDATION", "session not found", false, sessionID)
	}
	if session.Status != domain.SessionActive {
		return rejection(command.CommandID, "SESSION_NOT_ACTIVE", "VALIDATION", "session is not active", false, sessionID)
	}
	if session.ProjectID != command.ProjectID {
		return rejection(command.CommandID, "PROJECT_MISMATCH", "VALIDATION", "command project does not match session", false, command.ProjectID)
	}
	if session.RuntimeSnapshotID != command.RuntimeSnapshotID {
		return rejection(command.CommandID, "SNAPSHOT_MISMATCH", "SNAPSHOT_MISMATCH", "command snapshot does not match session", false, command.RuntimeSnapshotID)
	}
	if running, err := e.store.HasRunningCueExecution(ctx, sessionID); err != nil {
		return internalFailure(command.CommandID, "RUNNING_EXECUTION_CHECK_FAILED", err)
	} else if running {
		return rejection(command.CommandID, "UNRESOLVED_EXECUTION", "VALIDATION", "session has a running cue execution", false, sessionID)
	}

	runtimeSnapshot, err := e.store.GetRuntimeSnapshot(ctx, command.RuntimeSnapshotID)
	if err != nil {
		return rejection(command.CommandID, "SNAPSHOT_NOT_FOUND", "SNAPSHOT_MISMATCH", "runtime snapshot not found", false, command.RuntimeSnapshotID)
	}
	if runtimeSnapshot.Status != domain.SnapshotPublished {
		return rejection(command.CommandID, "SNAPSHOT_NOT_PUBLISHED", "SNAPSHOT_MISMATCH", "runtime snapshot is not published", false, runtimeSnapshot.ID)
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return internalFailure(command.CommandID, "SNAPSHOT_DECODE_FAILED", err)
	}

	var payload CueGoPayload
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return rejection(command.CommandID, "INVALID_CUE_GO_PAYLOAD", "VALIDATION", "cue.go payload is invalid", false, "")
	}
	selected, errResult := selectCue(manifest, session.CurrentCueID, payload)
	if errResult != nil {
		errResult.CommandID = command.CommandID
		return *errResult
	}

	cueExecution, err := e.store.CreateCueExecution(ctx, sessionID, selected.ID, command.CorrelationID, command.Issuer)
	if err != nil {
		return internalFailure(command.CommandID, "CUE_EXECUTION_CREATE_FAILED", err)
	}
	cueStarted, err := e.emit(ctx, &sessionID, command, "cue.started", command.CommandID, map[string]any{
		"cue_execution_id": cueExecution.ID,
		"cue_id":           selected.ID,
		"cue_name":         selected.Name,
	})
	if err != nil {
		_ = e.store.FinishCueExecution(ctx, cueExecution.ID, domain.ExecutionFailed)
		return internalFailure(command.CommandID, "CUE_STARTED_EVENT_FAILED", err)
	}
	if err := e.store.SetSessionCurrentCue(ctx, sessionID, selected.ID); err != nil {
		_ = e.store.FinishCueExecution(ctx, cueExecution.ID, domain.ExecutionFailed)
		return internalFailure(command.CommandID, "CURRENT_CUE_UPDATE_FAILED", err)
	}

	cueResult, lastEventID, executionErr := e.executeActions(ctx, sessionID, command, manifest, cueExecution, cueStarted.EventID, selected.Actions)
	if executionErr != nil {
		_ = e.store.FinishCueExecution(ctx, cueExecution.ID, domain.ExecutionFailed)
		return internalFailure(command.CommandID, "ACTION_EXECUTION_PERSISTENCE_FAILED", executionErr)
	}
	if err := e.store.FinishCueExecution(ctx, cueExecution.ID, cueResult); err != nil {
		return internalFailure(command.CommandID, "CUE_EXECUTION_FINISH_FAILED", err)
	}

	cueEventType := "cue.completed"
	commandStatus := contracts.CommandCompleted
	if cueResult != domain.ExecutionCompleted {
		cueEventType = "cue.failed"
		commandStatus = contracts.CommandFailed
		if cueResult == domain.ExecutionTimedOut {
			commandStatus = contracts.CommandTimedOut
		} else if cueResult == domain.ExecutionCancelled {
			commandStatus = contracts.CommandCancelled
		}
	}
	if lastEventID == "" {
		lastEventID = cueStarted.EventID
	}
	if _, err := e.emit(ctx, &sessionID, command, cueEventType, lastEventID, map[string]any{
		"cue_execution_id": cueExecution.ID,
		"cue_id":           selected.ID,
		"result":           cueResult,
	}); err != nil {
		return internalFailure(command.CommandID, "CUE_RESULT_EVENT_FAILED", err)
	}
	resultPayload, _ := json.Marshal(map[string]any{
		"cue_execution_id": cueExecution.ID,
		"cue_id":           selected.ID,
		"result":           cueResult,
	})
	return contracts.CommandResult{CommandID: command.CommandID, Status: commandStatus, Payload: resultPayload}
}

func (e *Engine) executeActions(
	ctx context.Context,
	sessionID string,
	command contracts.CommandEnvelope,
	manifest snapshot.Manifest,
	cueExecution domain.CueExecution,
	cueStartedEventID string,
	actions []snapshot.Action,
) (domain.ExecutionResult, string, error) {
	pending := make([]pendingAction, 0)
	lastEventID := cueStartedEventID

	waitPending := func() (domain.ExecutionResult, string, error) {
		final := domain.ExecutionCompleted
		last := lastEventID
		for _, item := range pending {
			outcome := <-item.result
			if outcome.err != nil {
				return domain.ExecutionFailed, last, outcome.err
			}
			last = outcome.eventID
			if outcome.fatal && final == domain.ExecutionCompleted {
				final = outcome.result
			}
		}
		pending = pending[:0]
		return final, last, nil
	}

	for _, action := range actions {
		if !action.Enabled {
			continue
		}
		switch strings.ToUpper(action.ExecutionMode) {
		case "PARALLEL":
			started, err := e.startAction(ctx, sessionID, command, manifest, cueExecution, cueStartedEventID, action, true)
			if err != nil {
				return domain.ExecutionFailed, lastEventID, err
			}
			pending = append(pending, started)
			lastEventID = started.startedEventID
		case "PARALLEL_BARRIER":
			started, err := e.startAction(ctx, sessionID, command, manifest, cueExecution, cueStartedEventID, action, true)
			if err != nil {
				return domain.ExecutionFailed, lastEventID, err
			}
			pending = append(pending, started)
			lastEventID = started.startedEventID
			pendingResult, last, err := waitPending()
			if err != nil {
				return domain.ExecutionFailed, last, err
			}
			lastEventID = last
			if pendingResult != domain.ExecutionCompleted {
				return pendingResult, lastEventID, nil
			}
		case "SEQUENTIAL", "":
			started, err := e.startAction(ctx, sessionID, command, manifest, cueExecution, cueStartedEventID, action, false)
			if err != nil {
				return domain.ExecutionFailed, lastEventID, err
			}
			outcome := <-started.result
			if outcome.err != nil {
				return domain.ExecutionFailed, lastEventID, outcome.err
			}
			lastEventID = outcome.eventID
			if outcome.fatal {
				pendingResult, last, err := waitPending()
				if err != nil {
					return domain.ExecutionFailed, last, err
				}
				if last != "" {
					lastEventID = last
				}
				if pendingResult != domain.ExecutionCompleted && outcome.result == domain.ExecutionCompleted {
					return pendingResult, lastEventID, nil
				}
				return outcome.result, lastEventID, nil
			}
		default:
			return domain.ExecutionFailed, lastEventID, fmt.Errorf("unsupported execution mode %q", action.ExecutionMode)
		}
	}

	pendingResult, last, err := waitPending()
	if err != nil {
		return domain.ExecutionFailed, last, err
	}
	if last != "" {
		lastEventID = last
	}
	return pendingResult, lastEventID, nil
}

type pendingAction struct {
	startedEventID string
	result         <-chan actionOutcome
}

type actionOutcome struct {
	result  domain.ExecutionResult
	fatal   bool
	eventID string
	err     error
}

func (e *Engine) startAction(
	ctx context.Context,
	sessionID string,
	command contracts.CommandEnvelope,
	manifest snapshot.Manifest,
	cueExecution domain.CueExecution,
	cueStartedEventID string,
	action snapshot.Action,
	async bool,
) (pendingAction, error) {
	actionExecution, err := e.store.CreateActionExecution(ctx, cueExecution.ID, action.ID)
	if err != nil {
		return pendingAction{}, err
	}
	startedEvent, err := e.emit(ctx, &sessionID, command, "action.started", cueStartedEventID, map[string]any{
		"cue_execution_id":    cueExecution.ID,
		"action_execution_id": actionExecution.ID,
		"action_id":           action.ID,
		"capability_key":      action.CapabilityKey,
		"target_ref":          action.TargetRef,
	})
	if err != nil {
		return pendingAction{}, err
	}
	resultCh := make(chan actionOutcome, 1)
	run := func() {
		outcome := e.runAction(ctx, sessionID, command, manifest, actionExecution, startedEvent.EventID, action)
		resultCh <- outcome
		close(resultCh)
	}
	if async {
		go run()
	} else {
		run()
	}
	return pendingAction{startedEventID: startedEvent.EventID, result: resultCh}, nil
}

func (e *Engine) runAction(
	parent context.Context,
	sessionID string,
	command contracts.CommandEnvelope,
	manifest snapshot.Manifest,
	actionExecution domain.ActionExecution,
	startedEventID string,
	action snapshot.Action,
) actionOutcome {
	actionCtx, cancel, timeoutMS, policyErr := actionContext(parent, action.TimeoutPolicy)
	if cancel != nil {
		defer cancel()
	}

	started := time.Now()
	var executionResult capability.Result
	if policyErr != nil {
		executionResult = capability.Result{
			Result:          domain.ExecutionFailed,
			AckLevel:        contracts.AckNone,
			ErrorCode:       "INVALID_TIMEOUT_POLICY",
			ResponseSummary: policyErr.Error(),
		}
	} else {
		executionResult = e.executor.Execute(actionCtx, capability.Request{
			ExecutionID:   actionExecution.ID,
			Capability:    action.CapabilityKey,
			Target:        resolvedTarget(manifest, action.TargetRef),
			Parameters:    action.Parameters,
			Priority:      action.PriorityClass,
			TimeoutMS:     timeoutMS,
			CorrelationID: command.CorrelationID,
		})
	}

	latencyMS := time.Since(started).Milliseconds()
	errorCode := nullableErrorCode(executionResult.ErrorCode)
	if err := e.store.FinishActionExecution(
		parent,
		actionExecution.ID,
		executionResult.Result,
		latencyMS,
		executionResult.ResponseSummary,
		errorCode,
	); err != nil {
		return actionOutcome{result: domain.ExecutionFailed, fatal: true, err: err}
	}

	eventType := "action.completed"
	if executionResult.Result == domain.ExecutionTimedOut {
		eventType = "action.timed_out"
	} else if executionResult.Result != domain.ExecutionCompleted {
		eventType = "action.failed"
	}
	event, err := e.emit(parent, &sessionID, command, eventType, startedEventID, map[string]any{
		"action_execution_id": actionExecution.ID,
		"action_id":           action.ID,
		"result":              executionResult.Result,
		"ack_level":           executionResult.AckLevel,
		"latency_ms":          latencyMS,
		"response_summary":    executionResult.ResponseSummary,
		"error_code":          executionResult.ErrorCode,
		"simulation":          action.CapabilityKey == "sim.test",
	})
	if err != nil {
		return actionOutcome{result: domain.ExecutionFailed, fatal: true, err: err}
	}
	fatal, err := actionFailureIsFatal(action.ErrorPolicy, executionResult.Result)
	if err != nil {
		return actionOutcome{result: domain.ExecutionFailed, fatal: true, eventID: event.EventID, err: err}
	}
	return actionOutcome{result: executionResult.Result, fatal: fatal, eventID: event.EventID}
}

func resolvedTarget(manifest snapshot.Manifest, targetRef string) *capability.Target {
	target := manifest.ResolveTarget(targetRef)
	if target == nil {
		return nil
	}
	return &capability.Target{
		AliasID:       target.AliasID,
		Ref:           target.TargetRef,
		LogicalType:   target.LogicalType,
		Configuration: append(json.RawMessage(nil), target.Configuration...),
	}
}

func actionContext(parent context.Context, raw json.RawMessage) (context.Context, context.CancelFunc, int64, error) {
	if len(raw) == 0 || string(raw) == "{}" {
		return parent, nil, 0, nil
	}
	var policy struct {
		TimeoutMS int64 `json:"timeout_ms"`
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return parent, nil, 0, fmt.Errorf("invalid timeout policy: %w", err)
	}
	if policy.TimeoutMS < 0 {
		return parent, nil, 0, fmt.Errorf("timeout_ms must be >= 0")
	}
	if policy.TimeoutMS == 0 {
		return parent, nil, 0, nil
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(policy.TimeoutMS)*time.Millisecond)
	return ctx, cancel, policy.TimeoutMS, nil
}

func actionFailureIsFatal(raw json.RawMessage, result domain.ExecutionResult) (bool, error) {
	if result == domain.ExecutionCompleted {
		return false, nil
	}
	var policy struct {
		OnError string `json:"on_error"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &policy); err != nil {
			return true, fmt.Errorf("invalid error policy: %w", err)
		}
	}
	onError := strings.ToUpper(strings.TrimSpace(policy.OnError))
	if onError == "" || onError == "FAIL_CUE" {
		return true, nil
	}
	if onError == "CONTINUE" {
		return false, nil
	}
	return true, fmt.Errorf("unsupported on_error policy %q", policy.OnError)
}

func (e *Engine) emit(
	ctx context.Context,
	sessionID *string,
	command contracts.CommandEnvelope,
	eventType, causationID string,
	payload any,
) (contracts.EventEnvelope, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return contracts.EventEnvelope{}, err
	}
	priority := command.Priority
	if priority == "" {
		priority = "P1"
	}
	return e.store.AppendEvent(ctx, sessionID, contracts.EventEnvelope{
		EventType:         eventType,
		SchemaVersion:     contracts.SchemaVersion1,
		Source:            "hub.cue_engine",
		ProjectID:         command.ProjectID,
		RuntimeSnapshotID: command.RuntimeSnapshotID,
		CorrelationID:     command.CorrelationID,
		CausationID:       causationID,
		Priority:          priority,
		TraceContext:      json.RawMessage(`{}`),
		Payload:           payloadJSON,
	})
}

func (e *Engine) duplicateResult(record domain.CommandRecord) contracts.CommandResult {
	if result, terminal, err := e.store.StoredCommandResult(record); err == nil && terminal {
		return result
	}
	return rejection(
		record.CommandID,
		"DUPLICATE_UNRESOLVED",
		"VALIDATION",
		"matching command is already accepted and will not be replayed",
		false,
		record.CommandID,
	)
}

func validateEnvelope(command contracts.CommandEnvelope) *contracts.CommandResult {
	if strings.TrimSpace(command.CommandID) == "" {
		result := rejection(command.CommandID, "COMMAND_ID_REQUIRED", "VALIDATION", "command_id is required", false, "")
		return &result
	}
	if command.CommandType != CueGoCommandType {
		result := rejection(command.CommandID, "UNSUPPORTED_COMMAND", "VALIDATION", "only cue.go is supported by the cue engine", false, command.CommandType)
		return &result
	}
	if command.SchemaVersion != contracts.SchemaVersion1 {
		result := rejection(command.CommandID, "UNSUPPORTED_SCHEMA_VERSION", "VALIDATION", "unsupported command schema version", false, "")
		return &result
	}
	if strings.TrimSpace(command.ProjectID) == "" ||
		strings.TrimSpace(command.RuntimeSnapshotID) == "" ||
		strings.TrimSpace(command.Issuer) == "" {
		result := rejection(command.CommandID, "COMMAND_CONTEXT_REQUIRED", "VALIDATION", "project_id, runtime_snapshot_id and issuer are required", false, "")
		return &result
	}
	if command.Priority != "P1" {
		result := rejection(command.CommandID, "INVALID_PRIORITY", "VALIDATION", "cue.go must use P1 priority", false, command.Priority)
		return &result
	}
	if !json.Valid(command.Payload) {
		result := rejection(command.CommandID, "INVALID_PAYLOAD_JSON", "VALIDATION", "payload must be valid JSON", false, "")
		return &result
	}
	return nil
}

func selectCue(manifest snapshot.Manifest, currentCueID *string, payload CueGoPayload) (snapshot.Cue, *contracts.CommandResult) {
	current := ""
	if currentCueID != nil {
		current = *currentCueID
	}
	if payload.ExpectedCurrentCueID != nil && *payload.ExpectedCurrentCueID != current {
		result := rejection("", "CURRENT_CUE_MISMATCH", "VALIDATION", "expected current cue does not match session state", false, current)
		return snapshot.Cue{}, &result
	}
	if payload.RequestedNextCueID != nil {
		for _, cue := range manifest.Cues {
			if cue.ID == *payload.RequestedNextCueID {
				if !cue.Enabled {
					result := rejection("", "CUE_DISABLED", "SAFETY_BLOCK", "requested cue is disabled", false, cue.ID)
					return snapshot.Cue{}, &result
				}
				return cue, nil
			}
		}
		result := rejection("", "CUE_NOT_IN_SNAPSHOT", "SNAPSHOT_MISMATCH", "requested cue is not present in runtime snapshot", false, *payload.RequestedNextCueID)
		return snapshot.Cue{}, &result
	}

	start := 0
	if current != "" {
		found := false
		for i, cue := range manifest.Cues {
			if cue.ID == current {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			result := rejection("", "CURRENT_CUE_NOT_IN_SNAPSHOT", "SNAPSHOT_MISMATCH", "session current cue is not present in runtime snapshot", false, current)
			return snapshot.Cue{}, &result
		}
	}
	for i := start; i < len(manifest.Cues); i++ {
		if manifest.Cues[i].Enabled {
			return manifest.Cues[i], nil
		}
	}
	result := rejection("", "NO_NEXT_CUE", "VALIDATION", "no enabled next cue is available", false, "")
	return snapshot.Cue{}, &result
}

func rejection(commandID, code, category, message string, retryable bool, entityID string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID,
		Status:    contracts.CommandRejected,
		Error: &contracts.ContractError{
			ErrorCode:        code,
			Category:         category,
			Message:          message,
			Retryable:        retryable,
			AffectedEntityID: entityID,
		},
	}
}

func internalFailure(commandID, code string, err error) contracts.CommandResult {
	message := "internal runtime failure"
	if err != nil && errors.Is(err, context.Canceled) {
		message = "runtime operation cancelled"
	}
	return contracts.CommandResult{
		CommandID: commandID,
		Status:    contracts.CommandFailed,
		Error: &contracts.ContractError{
			ErrorCode: code,
			Category:  "INTERNAL",
			Message:   message,
			Retryable: false,
			Details:   json.RawMessage(`{}`),
		},
	}
}

func nullableErrorCode(code string) *string {
	if code == "" {
		return nil
	}
	value := code
	return &value
}
