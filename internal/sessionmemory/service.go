package sessionmemory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type Service struct {
	store *store.Store
}

func New(s *store.Store) *Service { return &Service{store: s} }

type SessionStartPositionSummary struct {
	Version  int                             `json:"version"`
	Kind     domain.SessionStartPositionKind `json:"kind"`
	CueID    *string                         `json:"cue_id,omitempty"`
	Metadata json.RawMessage                 `json:"metadata"`
}

type SessionStateTruthSummary struct {
	Version                    int                             `json:"version"`
	RestorationStatus          domain.SessionRestorationStatus `json:"restoration_status"`
	DesiredStateRef            *string                         `json:"desired_state_ref,omitempty"`
	VerifiedStateRef           *string                         `json:"verified_state_ref,omitempty"`
	ManualConfirmationRequired bool                            `json:"manual_confirmation_required"`
}

type SessionSummary struct {
	SessionID          string                       `json:"session_id"`
	ProjectID          string                       `json:"project_id"`
	RuntimeSnapshotID  string                       `json:"runtime_snapshot_id"`
	Type               domain.SessionType           `json:"session_type"`
	Name               string                       `json:"name"`
	StartedAt          time.Time                    `json:"started_at"`
	EndedAt            *time.Time                   `json:"ended_at,omitempty"`
	Status             domain.SessionStatus         `json:"status"`
	ContractVersion    int                          `json:"session_contract_version"`
	LifecycleState     domain.SessionLifecycleState `json:"lifecycle_state"`
	EndReason          string                       `json:"end_reason,omitempty"`
	StartPosition      SessionStartPositionSummary  `json:"start_position"`
	CurrentCueID       *string                      `json:"current_cue_id,omitempty"`
	LastCompletedCueID *string                      `json:"last_completed_cue_id,omitempty"`
	NextCueID          *string                      `json:"next_cue_id,omitempty"`
	StateTruth         SessionStateTruthSummary     `json:"state_truth"`
}

type CueTrace struct {
	CueExecutionID string                 `json:"cue_execution_id"`
	CueID          string                 `json:"cue_id"`
	DisplayLabel   string                 `json:"display_label"`
	Name           string                 `json:"name"`
	CorrelationID  string                 `json:"correlation_id"`
	TriggerSource  string                 `json:"trigger_source"`
	StartedAt      time.Time              `json:"started_at"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty"`
	Result         domain.ExecutionResult `json:"result"`
	ManualOverride bool                   `json:"manual_override"`
	Interrupted    bool                   `json:"interrupted"`
	Actions        []ActionTrace          `json:"actions"`
}

type ActionTrace struct {
	ActionExecutionID string                 `json:"action_execution_id"`
	ActionID          string                 `json:"action_id"`
	TargetRef         string                 `json:"target_ref"`
	CapabilityKey     string                 `json:"capability_key"`
	StartedAt         time.Time              `json:"started_at"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	Result            domain.ExecutionResult `json:"result"`
	LatencyMS         *int64                 `json:"latency_ms,omitempty"`
	AckLevel          string                 `json:"ack_level"`
	DeliveryEvidence  string                 `json:"delivery_evidence"`
	DispatchAttempts  *int                   `json:"dispatch_attempts,omitempty"`
	ResponseSummary   string                 `json:"response_summary"`
	ErrorCode         *string                `json:"error_code,omitempty"`
	Interrupted       bool                   `json:"interrupted"`
}

// SessionEventTrace is a deliberately narrow projection of the canonical event
// stream. Raw event payloads can contain device or operator supplied data, so
// Session Memory exposes only the event identity and a small set of safe fields.
type SessionEventTrace struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	Category      string    `json:"category"`
	OccurredAt    time.Time `json:"occurred_at"`
	Priority      string    `json:"priority,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	WarningCount  int       `json:"warning_count,omitempty"`
	WarningKinds  []string  `json:"warning_kinds,omitempty"`
}

type actionEvidence struct {
	AckLevel         string
	DispatchAttempts *int
}

type SessionDetail struct {
	Session SessionSummary      `json:"session"`
	Cues    []CueTrace          `json:"cue_executions"`
	Events  []SessionEventTrace `json:"events"`
}

func (s *Service) List(ctx context.Context, projectID string, limit int) ([]SessionSummary, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("session memory service is unavailable")
	}
	if _, err := s.store.GetProject(ctx, strings.TrimSpace(projectID)); err != nil {
		return nil, err
	}
	sessions, err := s.store.ListSessionFoundationsForProject(ctx, projectID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, summary(session))
	}
	return out, nil
}

func (s *Service) Detail(ctx context.Context, projectID, sessionID string) (SessionDetail, error) {
	if s == nil || s.store == nil {
		return SessionDetail{}, fmt.Errorf("session memory service is unavailable")
	}
	session, err := s.store.GetSessionFoundation(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return SessionDetail{}, err
	}
	if session.ProjectID != strings.TrimSpace(projectID) {
		return SessionDetail{}, domain.ErrNotFound
	}
	runtimeSnapshot, err := s.store.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
	if err != nil {
		return SessionDetail{}, err
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return SessionDetail{}, err
	}
	cueMeta := make(map[string]snapshot.Cue, len(manifest.Cues))
	actionMeta := make(map[string]snapshot.Action)
	for _, cue := range manifest.Cues {
		cueMeta[cue.ID] = cue
		for _, action := range cue.Actions {
			actionMeta[action.ID] = action
		}
	}
	evidenceByExecution, events, err := s.sessionEventData(ctx, session.ID)
	if err != nil {
		return SessionDetail{}, err
	}
	cueExecutions, err := s.store.ListCueExecutions(ctx, session.ID)
	if err != nil {
		return SessionDetail{}, err
	}
	traces := make([]CueTrace, 0, len(cueExecutions))
	for _, execution := range cueExecutions {
		meta := cueMeta[execution.CueID]
		trace := CueTrace{
			CueExecutionID: execution.ID, CueID: execution.CueID,
			DisplayLabel: meta.DisplayLabel, Name: meta.Name,
			CorrelationID: execution.CorrelationID, TriggerSource: execution.TriggerSource,
			StartedAt: execution.StartedAt, CompletedAt: execution.CompletedAt, Result: execution.Result,
			ManualOverride: execution.ManualOverride, Actions: []ActionTrace{},
		}
		actions, err := s.store.ListActionExecutions(ctx, execution.ID)
		if err != nil {
			return SessionDetail{}, err
		}
		for _, actionExecution := range actions {
			meta := actionMeta[actionExecution.ActionID]
			evidence := evidenceByExecution[actionExecution.ID]
			if evidence.AckLevel == "" {
				evidence.AckLevel = string(contracts.AckNone)
			}
			interrupted := actionExecution.ErrorCode != nil && *actionExecution.ErrorCode == "HUB_RESTART_INTERRUPTED"
			if interrupted {
				trace.Interrupted = true
			}
			trace.Actions = append(trace.Actions, ActionTrace{
				ActionExecutionID: actionExecution.ID, ActionID: actionExecution.ActionID,
				TargetRef: meta.TargetRef, CapabilityKey: meta.CapabilityKey,
				StartedAt: actionExecution.StartedAt, CompletedAt: actionExecution.CompletedAt,
				Result: actionExecution.Result, LatencyMS: actionExecution.LatencyMS,
				AckLevel: evidence.AckLevel, DeliveryEvidence: deliveryEvidence(evidence.AckLevel),
				DispatchAttempts: evidence.DispatchAttempts, ResponseSummary: actionExecution.ResponseSummary,
				ErrorCode: actionExecution.ErrorCode, Interrupted: interrupted,
			})
		}
		traces = append(traces, trace)
	}
	return SessionDetail{Session: summary(session), Cues: traces, Events: events}, nil
}

func deliveryEvidence(ackLevel string) string {
	// Evidence follows the explicit acknowledgement level only. A completed
	// Hub Action by itself must never promote external execution evidence.
	switch contracts.AckLevel(ackLevel) {
	case contracts.AckTransportOnly:
		return "EXTERNAL_EXECUTION_UNKNOWN"
	case contracts.AckAccepted:
		return "OUTPUT_ACCEPTED"
	case contracts.AckDevice:
		return "DEVICE_ACKNOWLEDGED"
	case contracts.AckApplication:
		return "APPLICATION_ACKNOWLEDGED"
	case contracts.AckVerifiedState:
		return "STATE_VERIFIED"
	default:
		return "UNKNOWN"
	}
}

func (s *Service) sessionEventData(ctx context.Context, sessionID string) (map[string]actionEvidence, []SessionEventTrace, error) {
	events, err := s.store.ListEvents(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	evidenceByExecution := make(map[string]actionEvidence)
	traces := make([]SessionEventTrace, 0, len(events))
	for _, event := range events {
		if trace, ok := projectSessionEvent(event); ok {
			traces = append(traces, trace)
		}
		if event.EventType != "action.completed" && event.EventType != "action.failed" && event.EventType != "action.timed_out" {
			continue
		}
		var payload struct {
			ActionExecutionID string `json:"action_execution_id"`
			AckLevel          string `json:"ack_level"`
			DispatchAttempts  *int   `json:"dispatch_attempts"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.ActionExecutionID) != "" && strings.TrimSpace(payload.AckLevel) != "" {
			evidenceByExecution[payload.ActionExecutionID] = actionEvidence{
				AckLevel: payload.AckLevel, DispatchAttempts: payload.DispatchAttempts,
			}
		}
	}
	sort.SliceStable(traces, func(i, j int) bool {
		return traces[i].OccurredAt.Before(traces[j].OccurredAt)
	})
	return evidenceByExecution, traces, nil
}

func projectSessionEvent(event contracts.EventEnvelope) (SessionEventTrace, bool) {
	category := ""
	switch event.EventType {
	case "rehearsal.started", "rehearsal.stopped", "rehearsal.force_stopped",
		"show.entered", "show.exited", "show.force_exited":
		category = "session"
	case "runtime.emergency_blackout.applied", "runtime.emergency_blackout.cleared", "runtime.emergency_blackout.clear_failed",
		"runtime.edit_blackout.applied", "runtime.edit_blackout.cleared", "runtime.edit_blackout.clear_failed":
		category = "safety"
	case "stage_device.command.accepted", "stage_device.command.completed", "stage_device.command.timed_out",
		"stage_device.command.cancelled", "stage_device.command.rejected", "stage_device.command.failed":
		category = "device"
	case "simulation.stopped", "simulation.execution.completed", "simulation.execution.failed",
		"simulation.execution.timed_out", "simulation.execution.cancelled":
		category = "simulation"
	default:
		return SessionEventTrace{}, false
	}
	trace := SessionEventTrace{
		EventID: event.EventID, EventType: event.EventType, Category: category,
		OccurredAt: event.OccurredAt, CorrelationID: event.CorrelationID,
	}
	switch event.Priority {
	case "P0", "P1", "P2", "P3":
		trace.Priority = event.Priority
	}
	if category == "session" && (event.EventType == "rehearsal.started" || event.EventType == "show.entered") {
		var warning struct {
			DegradedStart   bool     `json:"degraded_start"`
			DegradedReasons []string `json:"degraded_reasons"`
			DeviceScope     string   `json:"device_scope_warning"`
			Preflight       string   `json:"preflight_warning"`
		}
		if json.Unmarshal(event.Payload, &warning) == nil {
			if warning.DegradedStart {
				trace.WarningKinds = append(trace.WarningKinds, "degraded_start")
				trace.WarningCount += len(warning.DegradedReasons)
				if len(warning.DegradedReasons) == 0 {
					trace.WarningCount++
				}
			}
			if strings.TrimSpace(warning.DeviceScope) != "" {
				trace.WarningKinds = append(trace.WarningKinds, "device_scope")
				trace.WarningCount++
			}
			if strings.TrimSpace(warning.Preflight) != "" {
				trace.WarningKinds = append(trace.WarningKinds, "preflight")
				trace.WarningCount++
			}
		}
	}
	return trace, true
}

func summary(session domain.Session) SessionSummary {
	return SessionSummary{
		SessionID: session.ID, ProjectID: session.ProjectID, RuntimeSnapshotID: session.RuntimeSnapshotID,
		Type: session.Type, Name: session.Name, StartedAt: session.StartedAt,
		EndedAt: session.EndedAt, Status: session.Status,
		ContractVersion: session.ContractVersion, LifecycleState: session.LifecycleState, EndReason: session.EndReason,
		StartPosition: SessionStartPositionSummary{
			Version: session.StartPosition.Version, Kind: session.StartPosition.Kind,
			CueID: session.StartPosition.CueID, Metadata: session.StartPosition.Metadata,
		},
		CurrentCueID: session.CurrentCueID, LastCompletedCueID: session.LastCompletedCueID, NextCueID: session.NextCueID,
		StateTruth: SessionStateTruthSummary{
			Version: session.StateTruth.Version, RestorationStatus: session.StateTruth.RestorationStatus,
			DesiredStateRef: session.StateTruth.DesiredStateRef, VerifiedStateRef: session.StateTruth.VerifiedStateRef,
			ManualConfirmationRequired: session.StateTruth.ManualConfirmationRequired,
		},
	}
}
