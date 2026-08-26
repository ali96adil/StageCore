package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

const InputInjectTestCommandType = "input.inject_test"

type InjectTestPayload struct {
	InputID         string          `json:"input_id"`
	Value           json.RawMessage `json:"value"`
	ConfirmCritical bool            `json:"confirm_critical,omitempty"`
}

type Engine struct {
	store     *store.Store
	executor  capability.Executor
	cueEngine *cueengine.Engine
	debouncer *Debouncer
	now       func() time.Time
}

func New(s *store.Store, executor capability.Executor) *Engine {
	return NewWithNow(s, executor, time.Now)
}

func NewWithNow(s *store.Store, executor capability.Executor, now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{
		store:     s,
		executor:  executor,
		cueEngine: cueengine.NewWithExecutor(s, executor),
		debouncer: NewDebouncer(),
		now:       now,
	}
}

func (e *Engine) InjectTest(ctx context.Context, sessionID string, command contracts.CommandEnvelope) contracts.CommandResult {
	if result := validateInjectEnvelope(command); result != nil {
		return *result
	}

	record, reserved, err := e.store.ReserveCommand(ctx, command)
	if err != nil {
		return commandFailure(command.CommandID, "COMMAND_RESERVE_FAILED", "INTERNAL", err.Error(), "")
	}
	if !reserved {
		if stored, terminal, err := e.store.StoredCommandResult(record); err != nil {
			return commandFailure(command.CommandID, "COMMAND_RESULT_DECODE_FAILED", "INTERNAL", err.Error(), "")
		} else if terminal {
			return stored
		}
		return commandFailure(command.CommandID, "DUPLICATE_UNRESOLVED", "VALIDATION", "input command is already accepted but unresolved", command.CommandID)
	}
	if command.CorrelationID == "" {
		command.CorrelationID = record.CorrelationID
	}

	result := e.executeReserved(ctx, sessionID, command)
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := e.store.FinishCommand(finishCtx, command.CommandID, result); err != nil {
		return commandFailure(command.CommandID, "COMMAND_FINISH_FAILED", "INTERNAL", err.Error(), command.CommandID)
	}
	return result
}

func (e *Engine) executeReserved(ctx context.Context, sessionID string, command contracts.CommandEnvelope) contracts.CommandResult {
	if e == nil || e.store == nil || e.executor == nil {
		return commandFailure(command.CommandID, "ROUTING_UNAVAILABLE", "INTERNAL", "routing engine is unavailable", "")
	}
	session, err := e.store.GetSession(ctx, sessionID)
	if err != nil {
		return commandFailure(command.CommandID, "SESSION_NOT_FOUND", "VALIDATION", "session not found", sessionID)
	}
	if session.Status != domain.SessionActive {
		return commandFailure(command.CommandID, "SESSION_NOT_ACTIVE", "VALIDATION", "session is not active", sessionID)
	}
	if session.ProjectID != command.ProjectID {
		return commandFailure(command.CommandID, "PROJECT_MISMATCH", "VALIDATION", "command project does not match session", command.ProjectID)
	}
	if session.RuntimeSnapshotID != command.RuntimeSnapshotID {
		return commandFailure(command.CommandID, "SNAPSHOT_MISMATCH", "SNAPSHOT_MISMATCH", "command snapshot does not match session", command.RuntimeSnapshotID)
	}
	runtimeSnapshot, err := e.store.GetRuntimeSnapshot(ctx, command.RuntimeSnapshotID)
	if err != nil || runtimeSnapshot.Status != domain.SnapshotPublished {
		return commandFailure(command.CommandID, "SNAPSHOT_NOT_PUBLISHED", "SNAPSHOT_MISMATCH", "published runtime snapshot is required", command.RuntimeSnapshotID)
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return commandFailure(command.CommandID, "SNAPSHOT_DECODE_FAILED", "INTERNAL", err.Error(), runtimeSnapshot.ID)
	}

	var payload InjectTestPayload
	if err := json.Unmarshal(command.Payload, &payload); err != nil || strings.TrimSpace(payload.InputID) == "" || len(bytes.TrimSpace(payload.Value)) == 0 {
		return commandFailure(command.CommandID, "INVALID_INPUT_PAYLOAD", "VALIDATION", "input.inject_test requires input_id and JSON value", "")
	}
	input := manifest.ResolveInput(payload.InputID)
	if input == nil {
		return commandFailure(command.CommandID, "INPUT_NOT_FOUND", "VALIDATION", "input is not present in Runtime Snapshot", payload.InputID)
	}
	if !input.Enabled {
		return commandFailure(command.CommandID, "INPUT_DISABLED", "VALIDATION", "input is disabled in Runtime Snapshot", input.ID)
	}

	inputEvent, err := e.emit(ctx, session.ID, command, "input.received", command.CommandID, map[string]any{
		"input_id": input.ID,
		"name":     input.Name,
		"source":   inputSource(command),
		"value":    json.RawMessage(payload.Value),
	})
	if err != nil {
		return commandFailure(command.CommandID, "INPUT_EVENT_FAILED", "INTERNAL", err.Error(), input.ID)
	}

	allowCritical := inputSource(command) != "TEST" || payload.ConfirmCritical
	evaluated := 0
	triggered := 0
	for _, route := range manifest.Routes {
		if route.InputID != input.ID {
			continue
		}
		evaluated++
		wasTriggered, routeErr := e.evaluateRoute(ctx, session, command, manifest, inputEvent.EventID, route, payload.Value, allowCritical)
		if wasTriggered {
			triggered++
		}
		if routeErr != nil {
			return commandFailure(command.CommandID, routeErr.code, routeErr.category, routeErr.message, route.ID)
		}
	}

	resultPayload, _ := json.Marshal(map[string]any{
		"input_event_id":   inputEvent.EventID,
		"input_id":         input.ID,
		"routes_evaluated": evaluated,
		"routes_triggered": triggered,
	})
	return contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandCompleted, Payload: resultPayload}
}

type routeFailure struct {
	code     string
	category string
	message  string
}

type criticalRouteTarget struct {
	actionID   string
	kind       string
	targetID   string
	criticality string
}

func (e *Engine) evaluateRoute(
	ctx context.Context,
	session domain.Session,
	command contracts.CommandEnvelope,
	manifest snapshot.Manifest,
	inputEventID string,
	route snapshot.Route,
	value json.RawMessage,
	allowCritical bool,
) (bool, *routeFailure) {
	if !route.Enabled {
		if _, err := e.emit(ctx, session.ID, command, "route.evaluated", inputEventID, routeTrace(route, value, nil, false, "DISABLED", "")); err != nil {
			return false, &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", err.Error()}
		}
		return false, nil
	}

	matched, err := EvaluateCondition(route.ConditionDefinition, value)
	if err != nil {
		_ = e.emitRouteFailure(ctx, session.ID, command, inputEventID, route, "ROUTE_CONDITION_INVALID", err.Error())
		return false, &routeFailure{"ROUTE_CONDITION_INVALID", "VALIDATION", err.Error()}
	}
	if !matched {
		if _, err := e.emit(ctx, session.ID, command, "route.evaluated", inputEventID, routeTrace(route, value, nil, false, "NOT_MATCHED", "")); err != nil {
			return false, &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", err.Error()}
		}
		return false, nil
	}

	transformed, err := ApplyTransform(route.TransformDefinition, value)
	if err != nil {
		_ = e.emitRouteFailure(ctx, session.ID, command, inputEventID, route, "ROUTE_TRANSFORM_INVALID", err.Error())
		return false, &routeFailure{"ROUTE_TRANSFORM_INVALID", "VALIDATION", err.Error()}
	}
	if !allowCritical {
		if target := firstCriticalRouteTarget(manifest, route); target != nil {
			if failure := e.blockCriticalTestRoute(ctx, session, command, inputEventID, route, value, transformed, *target); failure != nil {
				return false, failure
			}
			return false, &routeFailure{"SAFETY_CONFIRMATION_REQUIRED", "SAFETY_BLOCK", "critical Route target requires explicit confirmation for test input"}
		}
	}
	if route.DebounceMS != nil && *route.DebounceMS > 0 {
		if !e.debouncer.Accept(route.ID, e.now().UTC(), time.Duration(*route.DebounceMS)*time.Millisecond) {
			if _, err := e.emit(ctx, session.ID, command, "route.evaluated", inputEventID, routeTrace(route, value, transformed, true, "DEBOUNCED", "")); err != nil {
				return false, &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", err.Error()}
			}
			return false, nil
		}
	}
	if route.DelayMS != nil && *route.DelayMS > 0 {
		timer := time.NewTimer(time.Duration(*route.DelayMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			_ = e.emitRouteFailure(context.WithoutCancel(ctx), session.ID, command, inputEventID, route, "ROUTE_CANCELLED", ctx.Err().Error())
			return false, &routeFailure{"ROUTE_CANCELLED", "CANCELLED", ctx.Err().Error()}
		case <-timer.C:
		}
	}

	triggerEvent, err := e.emit(ctx, session.ID, command, "route.triggered", inputEventID, routeTrace(route, value, transformed, true, "TRIGGERED", ""))
	if err != nil {
		return false, &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", err.Error()}
	}

	for _, action := range route.Actions {
		if action.OutputID != nil {
			if failure := e.dispatchOutput(ctx, session, command, manifest, triggerEvent.EventID, route, action, transformed, allowCritical); failure != nil {
				return true, failure
			}
			continue
		}
		if action.CueID != nil {
			if failure := e.dispatchCue(ctx, session, command, manifest, triggerEvent.EventID, route, action, allowCritical); failure != nil {
				return true, failure
			}
			continue
		}
		return true, &routeFailure{"ROUTE_ACTION_INVALID", "VALIDATION", "route action has no output or cue target"}
	}
	return true, nil
}

func firstCriticalRouteTarget(manifest snapshot.Manifest, route snapshot.Route) *criticalRouteTarget {
	for _, action := range route.Actions {
		if action.OutputID != nil {
			if output := manifest.ResolveOutput(*action.OutputID); output != nil && isCritical(output.Criticality) {
				return &criticalRouteTarget{actionID: action.ID, kind: "output", targetID: output.ID, criticality: output.Criticality}
			}
		}
		if action.CueID != nil {
			if cue := resolveCue(manifest, *action.CueID); cue != nil && isCritical(cue.Criticality) {
				return &criticalRouteTarget{actionID: action.ID, kind: "cue", targetID: cue.ID, criticality: cue.Criticality}
			}
		}
	}
	return nil
}

func (e *Engine) blockCriticalTestRoute(
	ctx context.Context,
	session domain.Session,
	command contracts.CommandEnvelope,
	inputEventID string,
	route snapshot.Route,
	value json.RawMessage,
	transformed json.RawMessage,
	target criticalRouteTarget,
) *routeFailure {
	const code = "SAFETY_CONFIRMATION_REQUIRED"
	if _, err := e.emit(ctx, session.ID, command, "route.evaluated", inputEventID, routeTrace(route, value, transformed, true, "SAFETY_BLOCKED", code)); err != nil {
		return &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", err.Error()}
	}
	payload := map[string]any{
		"route_id":        route.ID,
		"route_action_id": target.actionID,
		"criticality":     target.criticality,
		"error_code":      code,
		"result":          "REJECTED",
	}
	if target.kind == "output" {
		payload["output_id"] = target.targetID
	} else {
		payload["cue_id"] = target.targetID
	}
	if _, err := e.emit(ctx, session.ID, command, "route.action.failed", inputEventID, payload); err != nil {
		return &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", err.Error()}
	}
	return nil
}

func (e *Engine) dispatchOutput(
	ctx context.Context,
	session domain.Session,
	command contracts.CommandEnvelope,
	manifest snapshot.Manifest,
	causationID string,
	route snapshot.Route,
	action snapshot.RouteAction,
	transformed json.RawMessage,
	allowCritical bool,
) *routeFailure {
	output := manifest.ResolveOutput(*action.OutputID)
	if output == nil {
		message := "route output is not present in Runtime Snapshot"
		_ = e.emitRouteFailure(ctx, session.ID, command, causationID, route, "OUTPUT_NOT_FOUND", message)
		return &routeFailure{"OUTPUT_NOT_FOUND", "VALIDATION", message}
	}
	if !allowCritical && isCritical(output.Criticality) {
		message := "critical Route output requires explicit confirmation for test input"
		_ = e.emit(ctx, session.ID, command, "route.action.failed", causationID, map[string]any{
			"route_id":        route.ID,
			"route_action_id": action.ID,
			"output_id":       output.ID,
			"criticality":     output.Criticality,
			"error_code":      "SAFETY_CONFIRMATION_REQUIRED",
			"result":          "REJECTED",
		})
		return &routeFailure{"SAFETY_CONFIRMATION_REQUIRED", "SAFETY_BLOCK", message}
	}
	executionID, err := stageid.New()
	if err != nil {
		return &routeFailure{"ROUTE_EXECUTION_ID_FAILED", "INTERNAL", err.Error()}
	}
	parameters := action.Parameters
	if isNullish(parameters) {
		parameters = transformed
	}
	target := resolvedCapabilityTarget(manifest, output.TargetRef)
	started := e.now()
	result := e.executor.Execute(ctx, capability.Request{
		ExecutionID:   executionID,
		Capability:    output.CapabilityKey,
		Target:        target,
		Parameters:    parameters,
		Priority:      route.PriorityClass,
		CorrelationID: command.CorrelationID,
	})
	latencyMS := e.now().Sub(started).Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	eventType := "route.action.completed"
	if result.Result != domain.ExecutionCompleted {
		eventType = "route.action.failed"
	}
	_, emitErr := e.emit(ctx, session.ID, command, eventType, causationID, map[string]any{
		"route_id":         route.ID,
		"route_action_id":  action.ID,
		"execution_id":     executionID,
		"output_id":        output.ID,
		"capability_key":   output.CapabilityKey,
		"target_ref":       output.TargetRef,
		"result":           result.Result,
		"ack_level":        result.AckLevel,
		"latency_ms":       latencyMS,
		"response_summary": result.ResponseSummary,
		"error_code":       result.ErrorCode,
	})
	if emitErr != nil {
		return &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", emitErr.Error()}
	}
	if result.Result != domain.ExecutionCompleted {
		code := result.ErrorCode
		if code == "" {
			code = "ROUTE_OUTPUT_FAILED"
		}
		return &routeFailure{code, "EXECUTION", result.ResponseSummary}
	}
	return nil
}

func (e *Engine) dispatchCue(
	ctx context.Context,
	session domain.Session,
	command contracts.CommandEnvelope,
	manifest snapshot.Manifest,
	causationID string,
	route snapshot.Route,
	action snapshot.RouteAction,
	allowCritical bool,
) *routeFailure {
	cue := resolveCue(manifest, *action.CueID)
	if cue == nil {
		message := "route Cue is not present in Runtime Snapshot"
		_ = e.emitRouteFailure(ctx, session.ID, command, causationID, route, "CUE_NOT_FOUND", message)
		return &routeFailure{"CUE_NOT_FOUND", "VALIDATION", message}
	}
	if !allowCritical && isCritical(cue.Criticality) {
		message := "critical Route Cue requires explicit confirmation for test input"
		_ = e.emit(ctx, session.ID, command, "route.action.failed", causationID, map[string]any{
			"route_id":        route.ID,
			"route_action_id": action.ID,
			"cue_id":          cue.ID,
			"criticality":     cue.Criticality,
			"error_code":      "SAFETY_CONFIRMATION_REQUIRED",
			"result":          "REJECTED",
		})
		return &routeFailure{"SAFETY_CONFIRMATION_REQUIRED", "SAFETY_BLOCK", message}
	}
	commandID, err := stageid.New()
	if err != nil {
		return &routeFailure{"ROUTE_CUE_COMMAND_ID_FAILED", "INTERNAL", err.Error()}
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{RequestedNextCueID: action.CueID})
	cueCommand := contracts.CommandEnvelope{
		CommandID:         commandID,
		CommandType:       cueengine.CueGoCommandType,
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          e.now().UTC(),
		ProjectID:         command.ProjectID,
		RuntimeSnapshotID: command.RuntimeSnapshotID,
		Issuer:            "route:" + route.ID,
		CorrelationID:     command.CorrelationID,
		CausationID:       causationID,
		Priority:          "P1",
		IdempotencyKey:    command.CommandID + ":" + action.ID,
		Payload:           payload,
	}
	result := e.cueEngine.ExecuteCueGo(ctx, session.ID, cueCommand)
	eventType := "route.action.completed"
	if result.Status != contracts.CommandCompleted {
		eventType = "route.action.failed"
	}
	_, emitErr := e.emit(ctx, session.ID, command, eventType, causationID, map[string]any{
		"route_id":        route.ID,
		"route_action_id": action.ID,
		"cue_id":          *action.CueID,
		"command_id":      commandID,
		"command_status":  result.Status,
	})
	if emitErr != nil {
		return &routeFailure{"ROUTE_TRACE_FAILED", "INTERNAL", emitErr.Error()}
	}
	if result.Status != contracts.CommandCompleted {
		message := "routed cue command failed"
		code := "ROUTE_CUE_FAILED"
		if result.Error != nil {
			message = result.Error.Message
			if result.Error.ErrorCode != "" {
				code = result.Error.ErrorCode
			}
		}
		return &routeFailure{code, "EXECUTION", message}
	}
	return nil
}

func resolveCue(manifest snapshot.Manifest, cueID string) *snapshot.Cue {
	for i := range manifest.Cues {
		if manifest.Cues[i].ID == cueID {
			cue := manifest.Cues[i]
			return &cue
		}
	}
	return nil
}

func isCritical(criticality string) bool {
	switch strings.ToUpper(strings.TrimSpace(criticality)) {
	case "CRITICAL", "SAFETY_CRITICAL":
		return true
	default:
		return false
	}
}

func resolvedCapabilityTarget(manifest snapshot.Manifest, targetRef string) *capability.Target {
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

func routeTrace(route snapshot.Route, input, transformed json.RawMessage, conditionResult bool, disposition, errorCode string) map[string]any {
	payload := map[string]any{
		"route_id":         route.ID,
		"route_name":       route.Name,
		"input_id":         route.InputID,
		"input_value":      json.RawMessage(input),
		"condition_result": conditionResult,
		"disposition":      disposition,
	}
	if transformed != nil {
		payload["transformed_value"] = json.RawMessage(transformed)
	}
	if errorCode != "" {
		payload["error_code"] = errorCode
	}
	return payload
}

func (e *Engine) emitRouteFailure(ctx context.Context, sessionID string, command contracts.CommandEnvelope, causationID string, route snapshot.Route, code, message string) error {
	_, err := e.emit(ctx, sessionID, command, "route.failed", causationID, map[string]any{
		"route_id":    route.ID,
		"route_name":  route.Name,
		"error_code":  code,
		"message":     message,
		"disposition": "FAILED",
	})
	return err
}

func (e *Engine) emit(ctx context.Context, sessionID string, command contracts.CommandEnvelope, eventType, causationID string, payload any) (contracts.EventEnvelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return contracts.EventEnvelope{}, err
	}
	return e.store.AppendEvent(ctx, &sessionID, contracts.EventEnvelope{
		EventType:         eventType,
		SchemaVersion:     contracts.SchemaVersion1,
		Source:            "stagecore.routing",
		ProjectID:         command.ProjectID,
		RuntimeSnapshotID: command.RuntimeSnapshotID,
		CorrelationID:     command.CorrelationID,
		CausationID:       causationID,
		Priority:          command.Priority,
		TraceContext:      json.RawMessage(`{}`),
		Payload:           raw,
	})
}

func inputSource(command contracts.CommandEnvelope) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(command.Issuer)), "osc:") {
		return "OSC"
	}
	return "TEST"
}

func validateInjectEnvelope(command contracts.CommandEnvelope) *contracts.CommandResult {
	if command.CommandType != InputInjectTestCommandType {
		result := commandFailure(command.CommandID, "COMMAND_TYPE_INVALID", "VALIDATION", "expected input.inject_test command", command.CommandType)
		return &result
	}
	if command.CommandID == "" || command.SchemaVersion != contracts.SchemaVersion1 || command.ProjectID == "" || command.RuntimeSnapshotID == "" || command.Issuer == "" {
		result := commandFailure(command.CommandID, "COMMAND_ENVELOPE_INVALID", "VALIDATION", "command id, schema, project, snapshot and issuer are required", "")
		return &result
	}
	if command.DeadlineAt != nil && time.Now().After(*command.DeadlineAt) {
		result := commandFailure(command.CommandID, "COMMAND_DEADLINE_EXCEEDED", "TIMEOUT", "command deadline already expired", command.CommandID)
		return &result
	}
	return nil
}

func commandFailure(commandID, code, category, message, affected string) contracts.CommandResult {
	status := contracts.CommandFailed
	if category == "VALIDATION" || category == "SNAPSHOT_MISMATCH" || category == "SAFETY_BLOCK" {
		status = contracts.CommandRejected
	} else if category == "CANCELLED" {
		status = contracts.CommandCancelled
	} else if category == "TIMEOUT" {
		status = contracts.CommandTimedOut
	}
	return contracts.CommandResult{
		CommandID: commandID,
		Status:    status,
		Error: &contracts.ContractError{
			ErrorCode:        code,
			Category:         category,
			Message:          message,
			AffectedEntityID: affected,
		},
	}
}
