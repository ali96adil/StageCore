package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

// ReconcileInterruptedRuntimeForHub applies the normal fail-closed restart
// reconciliation while preserving the one runtime shape that is explicitly
// restart-continuous: a clean REHEARSAL driven by INTERNAL timecode.
//
// Preservation is deliberately narrow. SHOW, SIMULATION, external timecode,
// malformed/multiple timecode targets, and any Session with in-flight cue or
// action work all keep the existing interrupted-runtime behavior.
func (s *Store) ReconcileInterruptedRuntimeForHub(ctx context.Context) (int64, error) {
	nowUS := clock.UnixMicros(s.clock.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin Hub runtime restart reconciliation: %w", err)
	}
	defer tx.Rollback()

	type activeSession struct {
		id        string
		typeName  string
		lifecycle string
		manifest  string
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT se.session_id, se.session_type, se.lifecycle_state, rs.manifest_json
		FROM sessions se
		JOIN runtime_snapshots rs ON rs.runtime_snapshot_id = se.runtime_snapshot_id
		WHERE se.status = 'ACTIVE'
		ORDER BY se.started_at_us, se.session_id`)
	if err != nil {
		return 0, fmt.Errorf("list active Sessions for Hub restart reconciliation: %w", err)
	}
	active := make([]activeSession, 0)
	for rows.Next() {
		var item activeSession
		if err := rows.Scan(&item.id, &item.typeName, &item.lifecycle, &item.manifest); err != nil {
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

	preserve := make(map[string]struct{})
	for _, session := range active {
		if domain.SessionType(session.typeName) != domain.SessionRehearsal ||
			domain.SessionLifecycleState(session.lifecycle) != domain.SessionLifecycleActive ||
			!internalTimecodeRestartManifest(session.manifest) {
			continue
		}
		running, err := hasRunningRuntimeWorkTx(ctx, tx, session.id)
		if err != nil {
			return 0, err
		}
		if !running {
			preserve[session.id] = struct{}{}
		}
	}

	var reconciled int64
	for _, session := range active {
		if _, ok := preserve[session.id]; ok {
			continue
		}
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

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit Hub runtime restart reconciliation: %w", err)
	}
	return reconciled, nil
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

func internalTimecodeRestartManifest(raw string) bool {
	var manifest struct {
		Targets []struct {
			LogicalType   string          `json:"logical_type"`
			Configuration json.RawMessage `json:"configuration"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		return false
	}
	matches := 0
	internal := false
	for _, target := range manifest.Targets {
		if !strings.EqualFold(strings.TrimSpace(target.LogicalType), "TIMECODE_SOURCE") {
			continue
		}
		matches++
		var cfg struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(target.Configuration, &cfg); err != nil {
			return false
		}
		internal = strings.EqualFold(strings.TrimSpace(cfg.Kind), "INTERNAL")
	}
	return matches == 1 && internal
}
