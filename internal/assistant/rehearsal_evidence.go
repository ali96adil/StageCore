package assistant

import (
	"context"
	"fmt"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulationreport"
)

type SimulationReportReader interface {
	Generate(context.Context, string) (simulationreport.Report, error)
}

func (s CanonicalEvidenceSource) collectRehearsal(ctx context.Context, query EvidenceQuery) (EvidenceCollection, error) {
	var out EvidenceCollection
	if query.SessionID == "" {
		return out, invalid("REHEARSAL evidence requires a SIMULATION session_id")
	}
	if s.Store == nil {
		addMissing(&out, "Flight Recorder store is unavailable")
		return out, nil
	}
	if s.Simulation == nil {
		addMissing(&out, "F-024 simulation report evidence is unavailable")
		return out, nil
	}

	session, err := s.Store.GetSession(ctx, query.SessionID)
	if err != nil {
		return out, err
	}
	if session.ProjectID != query.ProjectID {
		return out, domain.ErrNotFound
	}
	if session.Type != domain.SessionSimulation {
		return out, invalid("REHEARSAL evidence requires a SIMULATION session")
	}

	report, err := s.Simulation.Generate(ctx, query.SessionID)
	if err != nil {
		return out, err
	}
	if report.ProjectID != query.ProjectID || report.SessionID != query.SessionID {
		return out, invalid("simulation report authority does not match the Assistant request")
	}
	if report.RuntimeSnapshotID == "" || report.RuntimeSnapshotID != session.RuntimeSnapshotID {
		return out, invalid("simulation report runtime snapshot does not match the selected session")
	}

	appendEvidenceFact(&out, ContextSimulationEvidence, "simulation-report:"+report.SessionID, fmt.Sprintf(
		"simulation_report version=%d session_status=%s runtime_snapshot_id=%s snapshot_content_hash=%s evidence_scope=%s generated_at=%s cue_executions=%d action_executions=%d flight_recorder_events=%d missing_mappings=%d timing_risks=%d unhandled_failures=%d stage_differences=%d stage_unknown=%d",
		report.Version, report.SessionStatus, report.RuntimeSnapshotID, report.SnapshotContentHash, report.EvidenceScope,
		formatTime(report.GeneratedAt), report.Summary.CueExecutions, report.Summary.ActionExecutions,
		report.Summary.FlightRecorderEvents, report.Summary.MissingMappings, report.Summary.TimingRisks,
		report.Summary.UnhandledFailures, report.Summary.StageDifferences, report.Summary.StageUnknown,
	))
	for index, finding := range report.MissingMappings {
		appendEvidenceFact(&out, ContextSimulationEvidence, fmt.Sprintf("simulation-missing-mapping:%d", index), fmt.Sprintf(
			"simulation_missing_mapping kind=%s cue_id=%s action_id=%s output_id=%s target_ref=%s capability=%s reason_code=%s",
			finding.Kind, finding.CueID, finding.ActionID, finding.OutputID, finding.TargetRef, finding.Capability, finding.ReasonCode,
		))
	}
	for index, finding := range report.TimingRisks {
		latency := "unknown"
		if finding.LatencyMS != nil {
			latency = fmt.Sprintf("%dms", *finding.LatencyMS)
		}
		appendEvidenceFact(&out, ContextSimulationEvidence, fmt.Sprintf("simulation-timing-risk:%d", index), fmt.Sprintf(
			"simulation_timing_risk cue_execution_id=%s action_execution_id=%s cue_id=%s action_id=%s target_ref=%s capability=%s result=%s latency=%s timeout_ms=%d severity=%s reason_code=%s evidence_scope=%s",
			finding.CueExecutionID, finding.ActionExecutionID, finding.CueID, finding.ActionID, finding.TargetRef, finding.Capability,
			finding.Result, latency, finding.TimeoutMS, finding.Severity, finding.ReasonCode, finding.EvidenceScope,
		))
	}
	for index, finding := range report.UnhandledFailures {
		appendEvidenceFact(&out, ContextSimulationEvidence, fmt.Sprintf("simulation-unhandled-failure:%d", index), fmt.Sprintf(
			"simulation_unhandled_failure cue_execution_id=%s action_execution_id=%s cue_id=%s action_id=%s target_ref=%s capability=%s result=%s error_code=%s on_error=%s reason_code=%s evidence_scope=%s",
			finding.CueExecutionID, finding.ActionExecutionID, finding.CueID, finding.ActionID, finding.TargetRef, finding.Capability,
			finding.Result, finding.ErrorCode, finding.OnError, finding.ReasonCode, finding.EvidenceScope,
		))
	}
	for index, finding := range report.StageDifferences {
		appendEvidenceFact(&out, ContextSimulationEvidence, fmt.Sprintf("simulation-stage-difference:%d", index), fmt.Sprintf(
			"simulation_stage_difference target_ref=%s logical_type=%s stage_device_id=%s comparison=%s reason_code=%s actual_connection=%s actual_readiness=%s evidence_scope=%s",
			finding.TargetRef, finding.LogicalType, finding.StageDeviceID, finding.Comparison, finding.ReasonCode,
			finding.ActualConnection, finding.ActualReadiness, finding.EvidenceScope,
		))
	}

	executionEvidence, err := s.collectExecution(ctx, query)
	if err != nil {
		return out, err
	}
	mergeEvidenceCollection(&out, executionEvidence)

	timingEvidence, err := s.collectTiming(ctx, EvidenceQuery{
		Scope:             EvidenceTiming,
		ProjectID:         query.ProjectID,
		RevisionID:        query.RevisionID,
		RuntimeSnapshotID: report.RuntimeSnapshotID,
	})
	if err != nil {
		return out, err
	}
	mergeEvidenceCollection(&out, timingEvidence)
	return out, nil
}

func mergeEvidenceCollection(target *EvidenceCollection, source EvidenceCollection) {
	for _, fact := range source.Facts {
		appendEvidenceFact(target, fact.Kind, fact.RefID, fact.Summary)
	}
	for _, missing := range source.MissingContext {
		addMissing(target, missing)
	}
}
