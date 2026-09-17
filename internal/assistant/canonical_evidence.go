package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/doctor"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/timingintelligence"
)

const (
	maxRecentCueExecutions = 16
	maxRecentEvents        = 32
	maxEventPayloadBytes   = 2048
)

type RuntimeEvidenceStore interface {
	GetSession(context.Context, string) (domain.Session, error)
	ListCueExecutions(context.Context, string) ([]domain.CueExecution, error)
	ListActionExecutions(context.Context, string) ([]domain.ActionExecution, error)
	ListEvents(context.Context, string) ([]contracts.EventEnvelope, error)
}

type PreflightReader interface {
	Evaluate(context.Context, string, string) (preflight.Report, error)
}

type DoctorReader interface {
	Run(context.Context, doctor.Options) doctor.Report
}

type TimingReader interface {
	Report(context.Context, string, timingintelligence.ReportOptions) (timingintelligence.Report, error)
}

type CanonicalEvidenceSource struct {
	Store         RuntimeEvidenceStore
	Preflight     PreflightReader
	Doctor        DoctorReader
	DoctorOptions doctor.Options
	Timing        TimingReader
}

func (s CanonicalEvidenceSource) Collect(ctx context.Context, query EvidenceQuery) (EvidenceCollection, error) {
	query.ProjectID = strings.TrimSpace(query.ProjectID)
	query.RevisionID = strings.TrimSpace(query.RevisionID)
	query.RuntimeSnapshotID = strings.TrimSpace(query.RuntimeSnapshotID)
	query.SessionID = strings.TrimSpace(query.SessionID)
	query.CueExecutionID = strings.TrimSpace(query.CueExecutionID)
	if query.ProjectID == "" {
		return EvidenceCollection{}, invalid("evidence project_id is required")
	}
	if !query.Scope.Valid() {
		return EvidenceCollection{}, invalid("unsupported evidence scope %q", query.Scope)
	}

	switch query.Scope {
	case EvidenceExecution:
		return s.collectExecution(ctx, query)
	case EvidenceReadiness:
		return s.collectReadiness(ctx, query)
	case EvidenceTiming:
		return s.collectTiming(ctx, query)
	default:
		return EvidenceCollection{}, invalid("unsupported evidence scope %q", query.Scope)
	}
}

func (s CanonicalEvidenceSource) collectExecution(ctx context.Context, query EvidenceQuery) (EvidenceCollection, error) {
	var out EvidenceCollection
	if query.SessionID == "" {
		return out, invalid("EXECUTION evidence requires session_id")
	}
	if s.Store == nil {
		addMissing(&out, "Flight Recorder store is unavailable")
		return out, nil
	}

	session, err := s.Store.GetSession(ctx, query.SessionID)
	if err != nil {
		return out, err
	}
	if session.ProjectID != query.ProjectID {
		return out, domain.ErrNotFound
	}

	executions, err := s.Store.ListCueExecutions(ctx, query.SessionID)
	if err != nil {
		return out, err
	}
	var selected []domain.CueExecution
	var selectedCorrelation string
	if query.CueExecutionID != "" {
		for _, execution := range executions {
			if execution.ID == query.CueExecutionID {
				selected = []domain.CueExecution{execution}
				selectedCorrelation = execution.CorrelationID
				break
			}
		}
		if len(selected) == 0 {
			return out, domain.ErrNotFound
		}
	} else {
		selected = recentCueExecutions(executions, maxRecentCueExecutions)
	}

	for _, execution := range selected {
		appendEvidenceFact(&out, ContextFlightRecorderEvidence, execution.ID, fmt.Sprintf(
			"cue_execution cue_id=%s result=%s trigger_source=%s correlation_id=%s manual_override=%t started_at=%s completed_at=%s",
			execution.CueID, execution.Result, execution.TriggerSource, execution.CorrelationID, execution.ManualOverride,
			formatTime(execution.StartedAt), formatOptionalTime(execution.CompletedAt),
		))

		actions, err := s.Store.ListActionExecutions(ctx, execution.ID)
		if err != nil {
			return out, err
		}
		for _, action := range actions {
			latency := "unknown"
			if action.LatencyMS != nil {
				latency = fmt.Sprintf("%dms", *action.LatencyMS)
			}
			errorCode := ""
			if action.ErrorCode != nil {
				errorCode = *action.ErrorCode
			}
			appendEvidenceFact(&out, ContextFlightRecorderEvidence, action.ID, fmt.Sprintf(
				"action_execution cue_execution_id=%s action_id=%s result=%s latency=%s error_code=%s response_summary=%s started_at=%s completed_at=%s",
				action.CueExecutionID, action.ActionID, action.Result, latency, errorCode,
				boundedString(strings.TrimSpace(action.ResponseSummary), 1024), formatTime(action.StartedAt), formatOptionalTime(action.CompletedAt),
			))
		}
	}

	events, err := s.Store.ListEvents(ctx, query.SessionID)
	if err != nil {
		return out, err
	}
	if query.CueExecutionID != "" {
		filtered := events[:0]
		for _, event := range events {
			if event.CorrelationID == selectedCorrelation || event.CausationID == query.CueExecutionID || event.CausationID == selectedCorrelation {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	} else if len(events) > maxRecentEvents {
		events = events[len(events)-maxRecentEvents:]
	}
	for _, event := range events {
		payload := boundedString(strings.TrimSpace(string(event.Payload)), maxEventPayloadBytes)
		appendEvidenceFact(&out, ContextFlightRecorderEvidence, event.EventID, fmt.Sprintf(
			"event event_type=%s source=%s priority=%s sequence=%d correlation_id=%s causation_id=%s occurred_at=%s observed_at=%s payload=%s",
			event.EventType, event.Source, event.Priority, event.Sequence, event.CorrelationID, event.CausationID,
			formatTime(event.OccurredAt), formatTime(event.ObservedAt), payload,
		))
	}

	if len(out.Facts) == 0 {
		addMissing(&out, "no Flight Recorder evidence exists for the selected session")
	}
	return out, nil
}

func (s CanonicalEvidenceSource) collectReadiness(ctx context.Context, query EvidenceQuery) (EvidenceCollection, error) {
	var out EvidenceCollection
	if s.Preflight == nil {
		addMissing(&out, "SHOW Preflight evidence is unavailable")
	} else {
		report, err := s.Preflight.Evaluate(ctx, query.ProjectID, query.RuntimeSnapshotID)
		if err != nil {
			return out, err
		}
		refID := report.RuntimeSnapshotID
		if refID == "" {
			refID = query.ProjectID
		}
		appendEvidenceFact(&out, ContextPreflightFinding, "preflight-report:"+refID, fmt.Sprintf(
			"preflight_report status=%s project_id=%s runtime_snapshot_id=%s snapshot_version=%d evaluated_at=%s allows_show=%t",
			report.Status, report.ProjectID, report.RuntimeSnapshotID, report.SnapshotVersion, formatTime(report.EvaluatedAt), report.AllowsShow(),
		))
		for _, check := range report.Checks {
			appendEvidenceFact(&out, ContextPreflightFinding, check.Key, fmt.Sprintf(
				"preflight_check category=%s status=%s summary=%s detail=%s entity_id=%s evaluated_at=%s",
				check.Category, check.Status, check.Summary, check.Detail, check.EntityID, formatTime(report.EvaluatedAt),
			))
		}
	}

	if s.Doctor == nil {
		addMissing(&out, "Doctor evidence is unavailable")
	} else {
		report := s.Doctor.Run(ctx, s.DoctorOptions)
		appendEvidenceFact(&out, ContextDoctorFinding, fmt.Sprintf("doctor-report:%d", report.GeneratedAt.UnixMicro()), fmt.Sprintf(
			"doctor_report schema_version=%d overall=%s ready=%d warning=%d advisory=%d blocker=%d generated_at=%s",
			report.SchemaVersion, report.Overall, report.Counts.Ready, report.Counts.Warning, report.Counts.Advisory, report.Counts.Blocker, formatTime(report.GeneratedAt),
		))
		for _, check := range report.Checks {
			appendEvidenceFact(&out, ContextDoctorFinding, check.ID, fmt.Sprintf(
				"doctor_check status=%s message_key=%s message_args=%s detail=%s remedy_key=%s remedy_args=%s generated_at=%s",
				check.Status, check.MessageKey, strings.Join(check.MessageArgs, ","), check.Detail, check.RemedyKey,
				strings.Join(check.RemedyArgs, ","), formatTime(report.GeneratedAt),
			))
		}
	}
	return out, nil
}

func (s CanonicalEvidenceSource) collectTiming(ctx context.Context, query EvidenceQuery) (EvidenceCollection, error) {
	var out EvidenceCollection
	if s.Timing == nil {
		addMissing(&out, "timing intelligence evidence is unavailable")
		return out, nil
	}
	report, err := s.Timing.Report(ctx, query.ProjectID, timingintelligence.ReportOptions{
		RuntimeSnapshotID: query.RuntimeSnapshotID,
		SessionID:         query.SessionID,
	})
	if err != nil {
		return out, err
	}
	if !report.AdvisoryOnly {
		return out, invalid("timing evidence must remain advisory-only")
	}
	appendEvidenceFact(&out, ContextTimingEvidence, "timing-report:"+report.RuntimeSnapshotID, fmt.Sprintf(
		"timing_report runtime_snapshot_id=%s snapshot_content_hash=%s generated_at=%s sessions=%d transitions=%d advisory_only=%t",
		report.RuntimeSnapshotID, report.SnapshotContentHash, formatTime(report.GeneratedAt), len(report.Sessions), len(report.Transitions), report.AdvisoryOnly,
	))
	for _, session := range report.Sessions {
		appendEvidenceFact(&out, ContextTimingEvidence, session.SessionID, fmt.Sprintf(
			"timing_session name=%s lifecycle_state=%s runtime_snapshot_id=%s snapshot_match=%t selection_mode=%s eligible=%t effective=%t observation_count=%d reason=%s started_at=%s ended_at=%s",
			session.Name, session.LifecycleState, session.RuntimeSnapshotID, session.SnapshotMatch, session.SelectionMode,
			session.Eligible, session.Effective, session.ObservationCount, session.Reason, formatTime(session.StartedAt), formatOptionalTime(session.EndedAt),
		))
	}
	for _, transition := range report.Transitions {
		refID := "transition:" + transition.From.CueID + ":" + transition.To.CueID
		stats := transition.Statistics
		appendEvidenceFact(&out, ContextTimingEvidence, refID, fmt.Sprintf(
			"timing_transition from_cue_id=%s to_cue_id=%s sample_count=%d trusted_session_count=%d mean_us=%d median_us=%d lower_us=%d upper_us=%d spread_ratio=%g confidence=%s",
			transition.From.CueID, transition.To.CueID, stats.SampleCount, stats.TrustedSessionCount, stats.MeanUS,
			stats.MedianUS, stats.LowerUS, stats.UpperUS, stats.SpreadRatio, stats.Confidence,
		))
	}
	if report.Projection != nil {
		projection := report.Projection
		appendEvidenceFact(&out, ContextTimingEvidence, "projection:"+projection.SessionID, fmt.Sprintf(
			"timing_projection session_id=%s pace=%s confidence=%s divergence_kind=%s reason=%s expected_at=%s window_start_at=%s window_end_at=%s advisory_only=%t",
			projection.SessionID, projection.Pace, projection.Confidence, projection.DivergenceKind, projection.Reason,
			formatOptionalTime(projection.ExpectedAt), formatOptionalTime(projection.WindowStartAt), formatOptionalTime(projection.WindowEndAt), projection.AdvisoryOnly,
		))
	}
	return out, nil
}

func recentCueExecutions(items []domain.CueExecution, limit int) []domain.CueExecution {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[len(items)-limit:]
}

func appendEvidenceFact(collection *EvidenceCollection, kind ContextKind, refID, summary string) {
	if collection == nil {
		return
	}
	if len(collection.Facts) >= MaxContextFacts {
		addMissing(collection, "additional canonical evidence was omitted because the Assistant context limit was reached")
		return
	}
	refID = boundedString(strings.TrimSpace(refID), 256)
	summary = boundedString(strings.TrimSpace(summary), MaxContextSummaryLen)
	if refID == "" || summary == "" {
		return
	}
	collection.Facts = append(collection.Facts, ContextFact{Kind: kind, RefID: refID, Summary: summary})
}

func addMissing(collection *EvidenceCollection, item string) {
	if collection == nil {
		return
	}
	item = strings.TrimSpace(item)
	if item == "" {
		return
	}
	for _, existing := range collection.MissingContext {
		if existing == item {
			return
		}
	}
	collection.MissingContext = append(collection.MissingContext, item)
}

func boundedString(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	limit := maxBytes
	if limit > 3 {
		limit -= 3
	}
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		if size <= 0 {
			break
		}
		value = value[:len(value)-size]
	}
	if maxBytes > 3 {
		return value + "..."
	}
	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return "unknown"
	}
	return formatTime(*value)
}

func compactPayload(raw json.RawMessage) string {
	return boundedString(strings.TrimSpace(string(raw)), maxEventPayloadBytes)
}
