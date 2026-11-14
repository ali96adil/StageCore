package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

const timingSegmentSelection = "timing.segment_selection"

type TimingSegment struct {
	First    int64 `json:"first"`
	Last     int64 `json:"last"`
	Excluded bool  `json:"excluded"`
}

// SetTimingSegment changes training selection while preserving raw observations.
// Endpoint identities and the SHOW fence are checked in the journal transaction.
func (s *Store) SetTimingSegment(ctx context.Context, projectID, sessionID, actor string, segment TimingSegment) error {
	if segment.First <= 0 || segment.Last < segment.First {
		return fmt.Errorf("%w: invalid timing segment bounds", domain.ErrInvalidInput)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var snapshotID, kind, lifecycle string
	var showLocked bool
	err = tx.QueryRowContext(ctx, `
		SELECT runtime_snapshot_id, session_type, lifecycle_state, EXISTS (
			SELECT 1 FROM sessions live WHERE live.project_id = ? AND live.session_type = 'SHOW'
			  AND live.status = 'ACTIVE'
		) FROM sessions WHERE session_id = ? AND project_id = ?`,
		projectID, sessionID, projectID).Scan(&snapshotID, &kind, &lifecycle, &showLocked)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if showLocked {
		return domain.ErrShowConfigurationLocked
	}
	if kind != string(domain.SessionRehearsal) ||
		(lifecycle != string(domain.SessionLifecycleCompleted) && lifecycle != string(domain.SessionLifecycleStopped)) {
		return fmt.Errorf("%w: timing segments require a completed or stopped rehearsal", domain.ErrConflict)
	}

	var count int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM event_records
		WHERE project_id = ? AND session_id = ? AND event_type = 'cue.timing_observed'
		  AND sequence IN (?, ?)`, projectID, sessionID, segment.First, segment.Last).Scan(&count)
	if err != nil {
		return err
	}
	expected := 2
	if segment.First == segment.Last {
		expected = 1
	}
	if count != expected {
		return fmt.Errorf("%w: timing segment boundaries must be observations in this rehearsal", domain.ErrInvalidInput)
	}

	var excluded bool
	err = tx.QueryRowContext(ctx, `
		SELECT json_extract(payload_json, '$.excluded') FROM event_records
		WHERE project_id = ? AND session_id = ? AND event_type = ?
		  AND json_extract(payload_json, '$.first') = ? AND json_extract(payload_json, '$.last') = ?
		ORDER BY sequence DESC LIMIT 1`,
		projectID, sessionID, timingSegmentSelection, segment.First, segment.Last).Scan(&excluded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if excluded == segment.Excluded {
		return nil
	}
	eventID, err := stageid.New()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(segment)
	if err != nil {
		return err
	}
	now := clock.UnixMicros(s.clock.Now().UTC())
	_, err = tx.ExecContext(ctx, `
		INSERT INTO event_records (
			event_id, session_id, event_type, schema_version, occurred_at_us, observed_at_us,
			source_ref, project_id, runtime_snapshot_id, priority, trace_context_json, payload_json
		) VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?, 'P2', '{}', ?)
	`, eventID, sessionID, timingSegmentSelection, now, now, actor, projectID, snapshotID, string(payload))
	if err != nil {
		return err
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListTimingSegments(ctx context.Context, projectID string) (map[string][]TimingSegment, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH ranked AS (
			SELECT session_id, payload_json, ROW_NUMBER() OVER (
				PARTITION BY session_id, json_extract(payload_json, '$.first'), json_extract(payload_json, '$.last')
				ORDER BY sequence DESC
			) AS latest
			FROM event_records WHERE project_id = ? AND event_type = ?
		)
		SELECT session_id, payload_json FROM ranked
		WHERE latest = 1 AND json_extract(payload_json, '$.excluded') = 1
		ORDER BY json_extract(payload_json, '$.first'), json_extract(payload_json, '$.last')`,
		projectID, timingSegmentSelection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]TimingSegment)
	for rows.Next() {
		var sessionID, payload string
		if err := rows.Scan(&sessionID, &payload); err != nil {
			return nil, err
		}
		var segment TimingSegment
		if err := json.Unmarshal([]byte(payload), &segment); err != nil {
			return nil, err
		}
		result[sessionID] = append(result[sessionID], segment)
	}
	return result, rows.Err()
}
