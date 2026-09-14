package store

import (
	"context"
	"database/sql"
	"encoding/json"
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

type restartDecisionEvidence struct {
	decision          recovery.Decision
	timecodeAuthority recovery.TimecodeAuthority
	hasInFlightWork   bool
}

// ReconcileInterruptedRuntimeForHub applies the normal fail-closed restart
// reconciliation while preserving the one runtime shape that is explicitly
// restart-continuous: a clean REHEARSAL driven by INTERNAL timecode.
//
// F-020 makes the recovery decision explicit and records it atomically, but
// this first slice deliberately does not expand automatic recovery. SHOW,
// SIMULATION, external/ambiguous timecode, and any Session with in-flight cue
// or action work keep the existing interrupted-runtime behavior. A restart or
// reconnect never authorizes command replay by itself.
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
		if domain.SessionType(session.typeName) == domain.SessionRehearsal &&
			domain.SessionLifecycleState(session.lifecycle) == domain.SessionLifecycleActive &&
			authority == recovery.TimecodeAuthorityInternal {
			hasInFlightWork, err = hasRunningRuntimeWorkTx(ctx, tx, session.id)
			if err != nil {
				return 0, err
			}
		}
		decision := recovery.EvaluateRestart(recovery.RestartContext{
			SessionType:       domain.SessionType(session.typeName),
			LifecycleState:    domain.SessionLifecycleState(session.lifecycle),
			TimecodeAuthority: authority,
			HasInFlightWork:   hasInFlightWork,
		})
		decisions[session.id] = restartDecisionEvidence{
			decision:          decision,
			timecodeAuthority: authority,
			hasInFlightWork:   hasInFlightWork,
		}
	}

	var reconciled int64
	for _, session := range active {
		evidence := decisions[session.id]
		if evidence.decision.Disposition == recovery.DispositionAbort {
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
	payload, err := json.Marshal(map[string]any{
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
	})
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
