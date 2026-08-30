package sessionmemory

import (
	"context"
	"encoding/json"
	"fmt"
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
	Version                    int                              `json:"version"`
	RestorationStatus          domain.SessionRestorationStatus `json:"restoration_status"`
	DesiredStateRef            *string                          `json:"desired_state_ref,omitempty"`
	VerifiedStateRef           *string                          `json:"verified_state_ref,omitempty"`
	ManualConfirmationRequired bool                             `json:"manual_confirmation_required"`
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
	ResponseSummary   string                 `json:"response_summary"`
	ErrorCode         *string                `json:"error_code,omitempty"`
	Interrupted       bool                   `json:"interrupted"`
}

type SessionDetail struct {
	Session SessionSummary `json:"session"`
	Cues    []CueTrace     `json:"cue_executions"`
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
	ackByExecution, err := s.actionAcknowledgements(ctx, session.ID)
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
			ack := ackByExecution[actionExecution.ID]
			if ack == "" {
				ack = string(contracts.AckNone)
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
				AckLevel: ack, ResponseSummary: actionExecution.ResponseSummary,
				ErrorCode: actionExecution.ErrorCode, Interrupted: interrupted,
			})
		}
		traces = append(traces, trace)
	}
	return SessionDetail{Session: summary(session), Cues: traces}, nil
}

func (s *Service) actionAcknowledgements(ctx context.Context, sessionID string) (map[string]string, error) {
	events, err := s.store.ListEvents(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	acks := make(map[string]string)
	for _, event := range events {
		switch event.EventType {
		case "action.completed", "action.failed", "action.timed_out":
		default:
			continue
		}
		var payload struct {
			ActionExecutionID string `json:"action_execution_id"`
			AckLevel          string `json:"ack_level"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.ActionExecutionID) != "" && strings.TrimSpace(payload.AckLevel) != "" {
			acks[payload.ActionExecutionID] = payload.AckLevel
		}
	}
	return acks, nil
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
