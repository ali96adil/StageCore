package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

type TimingObservationRecord struct {
	Sequence            int64           `json:"sequence"`
	SessionID           string          `json:"session_id"`
	RuntimeSnapshotID   string          `json:"runtime_snapshot_id"`
	SnapshotContentHash string          `json:"snapshot_content_hash"`
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
	if err := s.RequireProjectConfigurationMutable(ctx, projectID); err != nil {
		return TimingSessionSelection{}, err
	}
	session, err := s.GetSessionFoundation(ctx, sessionID)
	if err != nil {
		return TimingSessionSelection{}, err
	}
	if session.ProjectID != projectID {
		return TimingSessionSelection{}, domain.ErrNotFound
	}
	if session.Type != domain.SessionRehearsal {
		return TimingSessionSelection{}, fmt.Errorf("%w: only REHEARSAL sessions can be selected for timing intelligence", domain.ErrInvalidInput)
	}
	if mode == TimingSelectionInclude && session.LifecycleState != domain.SessionLifecycleCompleted && session.LifecycleState != domain.SessionLifecycleStopped {
		return TimingSessionSelection{}, fmt.Errorf("%w: only completed or stopped REHEARSAL sessions can be explicitly included", domain.ErrConflict)
	}
	if mode == TimingSelectionAuto {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM timing_session_selection WHERE session_id = ?`, sessionID); err != nil {
			return TimingSessionSelection{}, fmt.Errorf("clear timing session selection: %w", err)
		}
		return TimingSessionSelection{SessionID: sessionID, Mode: TimingSelectionAuto}, nil
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO timing_session_selection (session_id, mode, updated_by, updated_at_us)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			mode = excluded.mode,
			updated_by = excluded.updated_by,
			updated_at_us = excluded.updated_at_us`,
		sessionID, mode, strings.TrimSpace(updatedBy), clock.UnixMicros(now)); err != nil {
		return TimingSessionSelection{}, fmt.Errorf("set timing session selection: %w", err)
	}
	return TimingSessionSelection{SessionID: sessionID, Mode: mode, UpdatedBy: strings.TrimSpace(updatedBy), UpdatedAt: now}, nil
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
		SELECT er.sequence, er.session_id, se.runtime_snapshot_id, rs.content_hash, er.payload_json
		FROM event_records er
		JOIN sessions se ON se.session_id = er.session_id
		JOIN runtime_snapshots rs ON rs.runtime_snapshot_id = se.runtime_snapshot_id
		WHERE er.project_id = ?
		  AND er.event_type = 'cue.timing_observed'
		  AND se.session_type = 'REHEARSAL'
		ORDER BY er.sequence`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("list timing observations: %w", err)
	}
	defer rows.Close()
	out := make([]TimingObservationRecord, 0)
	for rows.Next() {
		var item TimingObservationRecord
		var payload string
		if err := rows.Scan(&item.Sequence, &item.SessionID, &item.RuntimeSnapshotID, &item.SnapshotContentHash, &payload); err != nil {
			return nil, fmt.Errorf("scan timing observation: %w", err)
		}
		item.Payload = json.RawMessage(payload)
		out = append(out, item)
	}
	return out, rows.Err()
}
