package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

type TimingSelectionMode string

const (
	TimingSelectionAuto    TimingSelectionMode = "AUTO"
	TimingSelectionInclude TimingSelectionMode = "INCLUDE"
	TimingSelectionExclude TimingSelectionMode = "EXCLUDE"
)

type TimingSessionSelection struct {
	SessionID string              `json:"session_id"`
	Mode      TimingSelectionMode `json:"mode"`
	UpdatedBy string              `json:"updated_by,omitempty"`
	UpdatedAt time.Time           `json:"updated_at,omitempty"`
}

// TimingSelectionReview binds an approval to the snapshot and selection the operator reviewed.
type TimingSelectionReview struct {
	RuntimeSnapshotID string              `json:"runtime_snapshot_id"`
	Mode              TimingSelectionMode `json:"mode"`
}

type timingSelectionReviewKey struct{}

func WithTimingSelectionReview(ctx context.Context, review TimingSelectionReview) context.Context {
	return context.WithValue(ctx, timingSelectionReviewKey{}, review)
}

type TimingObservationRecord struct {
	Sequence            int64  `json:"sequence"`
	SessionID           string `json:"session_id"`
	RuntimeSnapshotID   string `json:"runtime_snapshot_id"`
	SnapshotContentHash string `json:"snapshot_content_hash"`
	TrainingExcluded    bool
	Payload             json.RawMessage `json:"payload"`
}

func (s *Store) SetTimingSessionSelection(ctx context.Context, projectID, sessionID string, mode TimingSelectionMode, updatedBy string) (TimingSessionSelection, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	if projectID == "" || sessionID == "" {
		return TimingSessionSelection{}, fmt.Errorf("%w: project and session are required", domain.ErrInvalidInput)
	}
	mode = TimingSelectionMode(strings.ToUpper(strings.TrimSpace(string(mode))))
	if mode != TimingSelectionAuto && mode != TimingSelectionInclude && mode != TimingSelectionExclude {
		return TimingSessionSelection{}, fmt.Errorf("%w: timing selection mode must be AUTO, INCLUDE or EXCLUDE", domain.ErrInvalidInput)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TimingSessionSelection{}, err
	}
	defer tx.Rollback()
	var session domain.Session
	var showLocked bool
	err = tx.QueryRowContext(ctx, `
		SELECT session_type, lifecycle_state, rehearsal_purpose, runtime_snapshot_id, EXISTS (
			SELECT 1 FROM sessions live WHERE live.project_id = ? AND live.session_type = 'SHOW' AND live.status = 'ACTIVE'
		) FROM sessions WHERE session_id = ? AND project_id = ?`, projectID, sessionID, projectID).
		Scan(&session.Type, &session.LifecycleState, &session.RehearsalPurpose, &session.RuntimeSnapshotID, &showLocked)
	if errors.Is(err, sql.ErrNoRows) {
		return TimingSessionSelection{}, domain.ErrNotFound
	}
	if err != nil {
		return TimingSessionSelection{}, err
	}
	if showLocked {
		return TimingSessionSelection{}, domain.ErrShowConfigurationLocked
	}
	if session.Type != domain.SessionRehearsal {
		return TimingSessionSelection{}, fmt.Errorf("%w: only REHEARSAL sessions can be selected for timing intelligence", domain.ErrInvalidInput)
	}
	if mode == TimingSelectionInclude &&
		session.LifecycleState != domain.SessionLifecycleCompleted &&
		session.LifecycleState != domain.SessionLifecycleStopped &&
		(session.RehearsalPurpose == "" || session.LifecycleState != domain.SessionLifecycleActive) {
		return TimingSessionSelection{}, fmt.Errorf("%w: only eligible rehearsals or active rehearsals with a recorded purpose can be approved", domain.ErrConflict)
	}
	if review, ok := ctx.Value(timingSelectionReviewKey{}).(TimingSelectionReview); ok {
		if review.RuntimeSnapshotID != session.RuntimeSnapshotID {
			return TimingSessionSelection{}, domain.ErrConflict
		}
		var current TimingSelectionMode
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT mode FROM timing_session_selection WHERE session_id = ?), 'AUTO')`, sessionID).Scan(&current); err != nil {
			return TimingSessionSelection{}, err
		}
		if current != review.Mode {
			return TimingSessionSelection{}, domain.ErrConflict
		}
	}
	if mode == TimingSelectionAuto {
		if _, err := tx.ExecContext(ctx, `DELETE FROM timing_session_selection WHERE session_id = ?`, sessionID); err != nil {
			return TimingSessionSelection{}, fmt.Errorf("clear timing session selection: %w", err)
		}
		if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
			return TimingSessionSelection{}, err
		}
		return TimingSessionSelection{SessionID: sessionID, Mode: TimingSelectionAuto}, tx.Commit()
	}
	now := s.clock.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO timing_session_selection (session_id, mode, updated_by, updated_at_us)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			mode = excluded.mode,
			updated_by = excluded.updated_by,
			updated_at_us = excluded.updated_at_us`,
		sessionID, mode, strings.TrimSpace(updatedBy), clock.UnixMicros(now)); err != nil {
		return TimingSessionSelection{}, fmt.Errorf("set timing session selection: %w", err)
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return TimingSessionSelection{}, err
	}
	return TimingSessionSelection{SessionID: sessionID, Mode: mode, UpdatedBy: strings.TrimSpace(updatedBy), UpdatedAt: now}, tx.Commit()
}

func (s *Store) ListTimingSessionSelections(ctx context.Context, projectID string) ([]TimingSessionSelection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tss.session_id, tss.mode, tss.updated_by, tss.updated_at_us
		FROM timing_session_selection tss
		JOIN sessions se ON se.session_id = tss.session_id
		WHERE se.project_id = ?
		ORDER BY tss.updated_at_us DESC, tss.session_id`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("list timing session selections: %w", err)
	}
	defer rows.Close()
	out := make([]TimingSessionSelection, 0)
	for rows.Next() {
		var item TimingSessionSelection
		var mode string
		var updatedUS int64
		if err := rows.Scan(&item.SessionID, &mode, &item.UpdatedBy, &updatedUS); err != nil {
			return nil, fmt.Errorf("scan timing session selection: %w", err)
		}
		item.Mode = TimingSelectionMode(mode)
		item.UpdatedAt = clock.FromUnixMicros(updatedUS)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetTimingSessionSelection(ctx context.Context, projectID, sessionID string) (TimingSessionSelection, error) {
	var item TimingSessionSelection
	var mode string
	var updatedUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT tss.session_id, tss.mode, tss.updated_by, tss.updated_at_us
		FROM timing_session_selection tss
		JOIN sessions se ON se.session_id = tss.session_id
		WHERE se.project_id = ? AND tss.session_id = ?`, strings.TrimSpace(projectID), strings.TrimSpace(sessionID)).Scan(
		&item.SessionID, &mode, &item.UpdatedBy, &updatedUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TimingSessionSelection{SessionID: strings.TrimSpace(sessionID), Mode: TimingSelectionAuto}, nil
	}
	if err != nil {
		return TimingSessionSelection{}, fmt.Errorf("get timing session selection: %w", err)
	}
	item.Mode = TimingSelectionMode(mode)
	item.UpdatedAt = clock.FromUnixMicros(updatedUS)
	return item, nil
}

func (s *Store) ListTimingObservationRecords(ctx context.Context, projectID string) ([]TimingObservationRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT er.sequence, er.session_id, se.runtime_snapshot_id, rs.content_hash, er.event_type, er.payload_json
		FROM event_records er
		JOIN sessions se ON se.session_id = er.session_id
		JOIN runtime_snapshots rs ON rs.runtime_snapshot_id = se.runtime_snapshot_id
		WHERE er.project_id = ?
		  AND er.event_type IN ('cue.timing_observed', ?, ?)
		  AND se.session_type = 'REHEARSAL'
		ORDER BY er.sequence`, strings.TrimSpace(projectID), timingCapturePaused, timingCaptureResumed)
	if err != nil {
		return nil, fmt.Errorf("list timing observations: %w", err)
	}
	defer rows.Close()
	out := make([]TimingObservationRecord, 0)
	type captureState struct{ paused, reanchor bool }
	states := map[string]captureState{}
	for rows.Next() {
		var item TimingObservationRecord
		var kind, payload string
		if err := rows.Scan(&item.Sequence, &item.SessionID, &item.RuntimeSnapshotID, &item.SnapshotContentHash, &kind, &payload); err != nil {
			return nil, fmt.Errorf("scan timing observation: %w", err)
		}
		state := states[item.SessionID]
		if kind != "cue.timing_observed" {
			states[item.SessionID] = captureState{paused: kind == timingCapturePaused, reanchor: true}
			continue
		}
		item.TrainingExcluded = state.paused || state.reanchor
		state.reanchor = false
		states[item.SessionID] = state
		item.Payload = json.RawMessage(payload)
		out = append(out, item)
	}
	return out, rows.Err()
}
