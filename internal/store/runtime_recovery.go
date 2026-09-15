package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/recovery"
)

const runtimeRecoveryDecisionEventType = "runtime.recovery.decision"

type restartActiveSession struct {
	id         string
	projectID  string
	snapshotID string
	typeName   string
	lifecycle  string
	manifest   string
}

type restartCheckpointEvidence struct {
	id                   string
	stateContractVersion int
	contentHash          string
}

type restartDecisionEvidence struct {
	decision          recovery.Decision
	timecodeAuthority recovery.TimecodeAuthority
	hasInFlightWork   bool
	checkpoint        *restartCheckpointEvidence
}

// ReconcileInterruptedRuntimeForHub applies the fail-closed Hub restart
// boundary. A clean INTERNAL-timecode REHEARSAL is the only runtime that may
// remain active automatically. SHOW and unsafe REHEARSAL shapes are closed as
// before. An interrupted SIMULATION may expose a durable F-024 checkpoint as a
// manual reconstruction target, but the old Session still ends and no command,
// Cue, action, transport result, or Digital Twin execution is replayed.
func (s *Store) ReconcileInterruptedRuntimeForHub(ctx context.Context) (int64, error) {
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin Hub runtime restart reconciliation: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT se.session_id, se.project_id, se.runtime_snapshot_id, se.session_type, se.lifecycle_state, rs.manifest_json
		FROM sessions se
		JOIN runtime_snapshots rs ON rs.runtime_snapshot_id = se.runtime_snapshot_id
		WHERE se.status = 'ACTIVE'
		ORDER BY se.started_at_us, se.session_id`)
	if err != nil {
		return 0, fmt.Errorf("list active Sessions for Hub restart reconciliation: %w", err)
	}
	active := make([]restartActiveSession, 0)
	for rows.Next() {
		var item restartActiveSession
		if err := rows.Scan(&item.id, &item.projectID, &item.snapshotID, &item.typeName, &item.lifecycle, &item.manifest); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan active Session for Hub restart reconciliation: %w", err)
		}
		active = append(active, item)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close active Session restart rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate active Sessions for Hub restart reconciliation: %w", err)
	}

	decisions := make(map[string]restartDecisionEvidence, len(active))
	for _, session := range active {
		authority := recovery.ClassifyTimecodeAuthority([]byte(session.manifest))
		hasInFlightWork := false
		var checkpoint *restartCheckpointEvidence

		sessionType := domain.SessionType(session.typeName)
		lifecycle := domain.SessionLifecycleState(session.lifecycle)
		if sessionType == domain.SessionRehearsal &&
			lifecycle == domain.SessionLifecycleActive &&
			authority == recovery.TimecodeAuthorityInternal {
			hasInFlightWork, err = hasRunningRuntimeWorkTx(ctx, tx, session.id)
			if err != nil {
				return 0, err
			}
		}
		if sessionType == domain.SessionSimulation && lifecycle == domain.SessionLifecycleActive {
			checkpoint, err = latestSimulationRecoveryCheckpointTx(ctx, tx, session)
			if err != nil {
				return 0, err
			}
		}

		decision := recovery.EvaluateRestart(recovery.RestartContext{
			SessionType:          sessionType,
			LifecycleState:       lifecycle,
			TimecodeAuthority:    authority,
			HasInFlightWork:      hasInFlightWork,
			HasTrustedCheckpoint: checkpoint != nil,
		})
		decisions[session.id] = restartDecisionEvidence{
			decision:          decision,
			timecodeAuthority: authority,
			hasInFlightWork:   hasInFlightWork,
			checkpoint:        checkpoint,
		}
	}

	var reconciled int64
	for _, session := range active {
		evidence := decisions[session.id]
		if evidence.decision.Disposition != recovery.DispositionPreserve {
			if _, err := tx.ExecContext(ctx, `
				UPDATE action_executions
				SET completed_at_us = ?, result = 'CANCELLED', latency_ms = COALESCE(latency_ms, 0),
				    response_summary = CASE WHEN response_summary = '' THEN 'Hub restarted before a terminal action result' ELSE response_summary END,
				    error_code = COALESCE(error_code, 'HUB_RESTART_INTERRUPTED')
				WHERE result = 'RUNNING'
				  AND cue_execution_id IN (
				      SELECT cue_execution_id FROM cue_executions WHERE session_id = ?
				  )`, nowUS, session.id); err != nil {
				return 0, fmt.Errorf("reconcile interrupted action executions: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE cue_executions
				SET completed_at_us = ?, result = 'CANCELLED'
				WHERE result = 'RUNNING' AND session_id = ?`, nowUS, session.id); err != nil {
				return 0, fmt.Errorf("reconcile interrupted cue executions: %w", err)
			}
			result, err := tx.ExecContext(ctx, `
				UPDATE sessions SET ended_at_us = ?, status = 'ABORTED'
				WHERE session_id = ? AND status = 'ACTIVE'`, nowUS, session.id)
			if err != nil {
				return 0, fmt.Errorf("reconcile interrupted Session: %w", err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return 0, fmt.Errorf("interrupted Session rows affected: %w", err)
			}
			reconciled += count

			switch evidence.decision.Disposition {
			case recovery.DispositionManualConfirmation:
				if evidence.checkpoint == nil {
					return 0, fmt.Errorf("manual simulation reconstruction has no checkpoint evidence")
				}
				if _, err := tx.ExecContext(ctx, `
					UPDATE sessions
					SET restoration_status = 'MANUAL_CONFIRMATION_REQUIRED',
					    desired_state_ref = ?, verified_state_ref = NULL,
					    manual_confirmation_required = 1
					WHERE session_id = ? AND status = 'ABORTED'`, evidence.checkpoint.id, session.id); err != nil {
					return 0, fmt.Errorf("record simulation checkpoint recovery truth: %w", err)
				}
			case recovery.DispositionAbort:
				// The F-027 interruption trigger intentionally leaves REHEARSAL as a
				// manually inspectable SUSPENDED candidate. SHOW and SIMULATION have
				// no recoverable state unless a checkpoint path was selected above.
				if domain.SessionType(session.typeName) != domain.SessionRehearsal {
					if _, err := tx.ExecContext(ctx, `
						UPDATE sessions
						SET restoration_status = 'UNAVAILABLE', desired_state_ref = NULL,
						    verified_state_ref = NULL, manual_confirmation_required = 0
						WHERE session_id = ? AND status = 'ABORTED'`, session.id); err != nil {
						return 0, fmt.Errorf("record unavailable restart recovery truth: %w", err)
					}
				}
			}
		}
		if err := appendRuntimeRecoveryDecisionEventTx(ctx, tx, nowUS, session, evidence); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit Hub runtime restart reconciliation: %w", err)
	}
	return reconciled, nil
}

func appendRuntimeRecoveryDecisionEventTx(ctx context.Context, tx *sql.Tx, nowUS int64, session restartActiveSession, evidence restartDecisionEvidence) error {
	eventID, err := stageid.New()
	if err != nil {
		return fmt.Errorf("create runtime recovery event id: %w", err)
	}
	payloadMap := map[string]any{
		"decision_version":             1,
		"scope":                        "SESSION_RESTART",
		"session_id":                   session.id,
		"session_type":                 session.typeName,
		"lifecycle_state":              session.lifecycle,
		"disposition":                  evidence.decision.Disposition,
		"reason_code":                  evidence.decision.ReasonCode,
		"automatic":                    evidence.decision.Automatic,
		"replay_allowed":               evidence.decision.ReplayAllowed,
		"manual_confirmation_required": evidence.decision.ManualConfirmationRequired,
		"timecode_authority":           evidence.timecodeAuthority,
		"in_flight_work":               evidence.hasInFlightWork,
	}
	if evidence.checkpoint != nil {
		payloadMap["checkpoint_id"] = evidence.checkpoint.id
		payloadMap["checkpoint_state_contract_version"] = evidence.checkpoint.stateContractVersion
		payloadMap["checkpoint_content_hash"] = evidence.checkpoint.contentHash
		payloadMap["reconstruction_start_kind"] = string(domain.SessionStartCheckpoint)
		payloadMap["reconstruction_requires_new_session"] = true
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return fmt.Errorf("encode runtime recovery decision event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_records (
			event_id, session_id, event_type, schema_version, occurred_at_us, observed_at_us,
			source_ref, project_id, runtime_snapshot_id, correlation_id, causation_id,
			priority, trace_context_json, payload_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, '{}', ?)`,
		eventID, session.id, runtimeRecoveryDecisionEventType, contracts.SchemaVersion1,
		nowUS, nowUS, "hub.runtime_recovery", session.projectID, session.snapshotID, "P1", string(payload)); err != nil {
		return fmt.Errorf("append runtime recovery decision event: %w", err)
	}
	return nil
}

func latestSimulationRecoveryCheckpointTx(ctx context.Context, tx *sql.Tx, session restartActiveSession) (*restartCheckpointEvidence, error) {
	var checkpoint domain.SimulationCheckpoint
	var currentCue, lastCompleted, nextCue sql.NullString
	var twinState string
	err := tx.QueryRowContext(ctx, `
		SELECT checkpoint_id, source_session_id, project_id, runtime_snapshot_id,
		       state_contract_version, current_cue_id, last_completed_cue_id,
		       next_cue_id, twin_state_json, content_hash
		FROM simulation_checkpoints
		WHERE source_session_id = ? AND project_id = ? AND runtime_snapshot_id = ?
		ORDER BY captured_at_us DESC, checkpoint_id DESC
		LIMIT 1`, session.id, session.projectID, session.snapshotID).Scan(
		&checkpoint.ID, &checkpoint.SourceSessionID, &checkpoint.ProjectID, &checkpoint.RuntimeSnapshotID,
		&checkpoint.StateContractVersion, &currentCue, &lastCompleted, &nextCue,
		&twinState, &checkpoint.ContentHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read simulation checkpoint recovery evidence: %w", err)
	}
	checkpoint.TwinState = json.RawMessage(twinState)
	assignCheckpointOptional(currentCue, &checkpoint.CurrentCueID)
	assignCheckpointOptional(lastCompleted, &checkpoint.LastCompletedCueID)
	assignCheckpointOptional(nextCue, &checkpoint.NextCueID)
	expectedHash, err := simulationCheckpointContentHash(checkpoint)
	if err != nil || expectedHash != checkpoint.ContentHash {
		// Corrupt or structurally invalid checkpoint state cannot become recovery
		// authority. The caller will fall back to the normal fail-closed SIMULATION
		// restart decision rather than preventing the Hub itself from starting.
		return nil, nil
	}
	return &restartCheckpointEvidence{
		id:                   checkpoint.ID,
		stateContractVersion: checkpoint.StateContractVersion,
		contentHash:          checkpoint.ContentHash,
	}, nil
}

func hasRunningRuntimeWorkTx(ctx context.Context, tx *sql.Tx, sessionID string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM cue_executions WHERE session_id = ? AND result = 'RUNNING') +
			(SELECT COUNT(*)
			 FROM action_executions ae
			 JOIN cue_executions ce ON ce.cue_execution_id = ae.cue_execution_id
			 WHERE ce.session_id = ? AND ae.result = 'RUNNING')`, sessionID, sessionID).Scan(&count); err != nil {
		return false, fmt.Errorf("check in-flight runtime work for restart continuity: %w", err)
	}
	return count > 0, nil
}
