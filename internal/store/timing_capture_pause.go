package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

const timingCapturePaused = "timing.capture_paused"
const timingCaptureResumed = "timing.capture_resumed"

// SetTimingCapturePaused changes only the timing journal. The active
// rehearsal check and the state transition share one database transaction.
func (s *Store) SetTimingCapturePaused(ctx context.Context, projectID, sessionID, actor string, paused bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var snapshot, kind, lifecycle string
	err = tx.QueryRowContext(ctx, `
		SELECT runtime_snapshot_id, session_type, lifecycle_state FROM sessions
		WHERE session_id = ? AND project_id = ?`, sessionID, projectID).Scan(&snapshot, &kind, &lifecycle)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if kind != string(domain.SessionRehearsal) || lifecycle != string(domain.SessionLifecycleActive) {
		return fmt.Errorf("%w: timing capture controls require an active rehearsal", domain.ErrConflict)
	}

	previous := timingCaptureResumed
	err = tx.QueryRowContext(ctx, `
		SELECT event_type FROM event_records
		WHERE session_id = ? AND event_type IN (?, ?) ORDER BY sequence DESC LIMIT 1
	`, sessionID, timingCapturePaused, timingCaptureResumed).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	next := timingCaptureResumed
	if paused {
		next = timingCapturePaused
	}
	if next == previous {
		return nil
	}
	id, err := stageid.New()
	if err != nil {
		return err
	}
	now := clock.UnixMicros(s.clock.Now().UTC())
	_, err = tx.ExecContext(ctx, `
		INSERT INTO event_records (
			event_id, session_id, event_type, schema_version, occurred_at_us, observed_at_us,
			source_ref, project_id, runtime_snapshot_id, priority, trace_context_json, payload_json
		) VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, 'P2', '{}', '{}')
	`, id, sessionID, next, now, now, actor, projectID, snapshot)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// TimingCapturePausedSessions returns the latest pause/resume state by
// rehearsal. Journal ordering keeps it stable across restarts and clock jumps.
func (s *Store) TimingCapturePausedSessions(ctx context.Context, projectID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, event_type FROM event_records
		WHERE project_id = ? AND event_type IN (?, ?) ORDER BY sequence
	`, projectID, timingCapturePaused, timingCaptureResumed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var sessionID, kind string
		if err := rows.Scan(&sessionID, &kind); err != nil {
			return nil, err
		}
		result[sessionID] = kind == timingCapturePaused
	}
	return result, rows.Err()
}
