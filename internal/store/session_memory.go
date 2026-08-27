package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type CreateNoteParams struct {
	SessionID *string
	CueID     *string
	Category  string
	Body      string
	CreatedBy string
}

type NoteFilter struct {
	Status    domain.NoteStatus
	Category  string
	SessionID string
	CueID     string
}

func (s *Store) CreateNote(ctx context.Context, projectID string, p CreateNoteParams) (domain.Note, error) {
	projectID = strings.TrimSpace(projectID)
	body := strings.TrimSpace(p.Body)
	if projectID == "" || body == "" {
		return domain.Note{}, fmt.Errorf("%w: project and note body are required", domain.ErrInvalidInput)
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return domain.Note{}, err
	}
	sessionID := cleanOptional(p.SessionID)
	cueID := cleanOptional(p.CueID)
	if err := s.validateNoteReferences(ctx, projectID, sessionID, cueID); err != nil {
		return domain.Note{}, err
	}
	noteID, err := stageid.New()
	if err != nil {
		return domain.Note{}, err
	}
	now := s.clock.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO operator_notes (
			note_id, project_id, session_id, cue_id, category, body, status,
			created_by, created_at_us, updated_at_us, resolved_at_us
		) VALUES (?, ?, ?, ?, ?, ?, 'OPEN', ?, ?, ?, NULL)`,
		noteID, projectID, nullableString(sessionID), nullableString(cueID), strings.TrimSpace(p.Category), body,
		strings.TrimSpace(p.CreatedBy), clock.UnixMicros(now), clock.UnixMicros(now),
	)
	if err != nil {
		return domain.Note{}, fmt.Errorf("insert operator note: %w", err)
	}
	return s.GetNote(ctx, projectID, noteID)
}

func (s *Store) GetNote(ctx context.Context, projectID, noteID string) (domain.Note, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT note_id, project_id, session_id, cue_id, category, body, status,
		       created_by, created_at_us, updated_at_us, resolved_at_us
		FROM operator_notes WHERE project_id = ? AND note_id = ?`, strings.TrimSpace(projectID), strings.TrimSpace(noteID))
	return scanNote(row)
}

func (s *Store) ListNotes(ctx context.Context, projectID string, filter NoteFilter) ([]domain.Note, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("%w: project is required", domain.ErrInvalidInput)
	}
	query := `SELECT note_id, project_id, session_id, cue_id, category, body, status,
	                 created_by, created_at_us, updated_at_us, resolved_at_us
	          FROM operator_notes WHERE project_id = ?`
	args := []any{projectID}
	if filter.Status != "" {
		if filter.Status != domain.NoteOpen && filter.Status != domain.NoteResolved {
			return nil, fmt.Errorf("%w: invalid note status", domain.ErrInvalidInput)
		}
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if category := strings.TrimSpace(filter.Category); category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	if sessionID := strings.TrimSpace(filter.SessionID); sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	if cueID := strings.TrimSpace(filter.CueID); cueID != "" {
		query += ` AND cue_id = ?`
		args = append(args, cueID)
	}
	query += ` ORDER BY CASE status WHEN 'OPEN' THEN 0 ELSE 1 END, updated_at_us DESC, note_id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list operator notes: %w", err)
	}
	defer rows.Close()
	notes := make([]domain.Note, 0)
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (s *Store) UpdateNote(ctx context.Context, projectID, noteID, body, category string) (domain.Note, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.Note{}, fmt.Errorf("%w: note body is required", domain.ErrInvalidInput)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE operator_notes SET body = ?, category = ?, updated_at_us = ?
		WHERE project_id = ? AND note_id = ?`, body, strings.TrimSpace(category), clock.UnixMicros(s.clock.Now()), strings.TrimSpace(projectID), strings.TrimSpace(noteID))
	if err != nil {
		return domain.Note{}, fmt.Errorf("update operator note: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Note{}, domain.ErrNotFound
		}
		return domain.Note{}, err
	}
	return s.GetNote(ctx, projectID, noteID)
}

func (s *Store) SetNoteStatus(ctx context.Context, projectID, noteID string, status domain.NoteStatus) (domain.Note, error) {
	if status != domain.NoteOpen && status != domain.NoteResolved {
		return domain.Note{}, fmt.Errorf("%w: invalid note status", domain.ErrInvalidInput)
	}
	now := s.clock.Now().UTC()
	var resolved any
	if status == domain.NoteResolved {
		resolved = clock.UnixMicros(now)
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE operator_notes
		SET status = ?, resolved_at_us = ?, updated_at_us = ?
		WHERE project_id = ? AND note_id = ?`, status, resolved, clock.UnixMicros(now), strings.TrimSpace(projectID), strings.TrimSpace(noteID))
	if err != nil {
		return domain.Note{}, fmt.Errorf("set operator note status: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.Note{}, domain.ErrNotFound
		}
		return domain.Note{}, err
	}
	return s.GetNote(ctx, projectID, noteID)
}

func (s *Store) ListSessionsForProject(ctx context.Context, projectID string, limit int) ([]domain.Session, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, project_id, runtime_snapshot_id, session_type, name,
		       started_at_us, ended_at_us, status, current_cue_id
		FROM sessions WHERE project_id = ?
		ORDER BY started_at_us DESC, session_id DESC LIMIT ?`, strings.TrimSpace(projectID), limit)
	if err != nil {
		return nil, fmt.Errorf("list project sessions: %w", err)
	}
	defer rows.Close()
	sessions := make([]domain.Session, 0)
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// ReconcileInterruptedRuntime marks runtime work that could not have survived a
// Hub process restart as aborted/cancelled. It never invents successful
// completion and leaves an explicit action error code for operator history.
func (s *Store) ReconcileInterruptedRuntime(ctx context.Context) (int64, error) {
	nowUS := clock.UnixMicros(s.clock.Now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin runtime restart reconciliation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE action_executions
		SET completed_at_us = ?, result = 'CANCELLED', latency_ms = COALESCE(latency_ms, 0),
		    response_summary = CASE WHEN response_summary = '' THEN 'Hub restarted before a terminal action result' ELSE response_summary END,
		    error_code = COALESCE(error_code, 'HUB_RESTART_INTERRUPTED')
		WHERE result = 'RUNNING'
		  AND cue_execution_id IN (
		      SELECT ce.cue_execution_id FROM cue_executions ce
		      JOIN sessions se ON se.session_id = ce.session_id
		      WHERE se.status = 'ACTIVE'
		  )`, nowUS); err != nil {
		return 0, fmt.Errorf("reconcile interrupted action executions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cue_executions
		SET completed_at_us = ?, result = 'CANCELLED'
		WHERE result = 'RUNNING'
		  AND session_id IN (SELECT session_id FROM sessions WHERE status = 'ACTIVE')`, nowUS); err != nil {
		return 0, fmt.Errorf("reconcile interrupted cue executions: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions SET ended_at_us = ?, status = 'ABORTED'
		WHERE status = 'ACTIVE'`, nowUS)
	if err != nil {
		return 0, fmt.Errorf("reconcile interrupted sessions: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("interrupted session rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit runtime restart reconciliation: %w", err)
	}
	return count, nil
}

func (s *Store) validateNoteReferences(ctx context.Context, projectID string, sessionID, cueID *string) error {
	if sessionID != nil {
		var sessionProject string
		if err := s.db.QueryRowContext(ctx, `SELECT project_id FROM sessions WHERE session_id = ?`, *sessionID).Scan(&sessionProject); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read note session: %w", err)
		}
		if sessionProject != projectID {
			return fmt.Errorf("%w: note session belongs to another project", domain.ErrInvalidInput)
		}
	}
	if cueID != nil {
		var cueProject string
		if err := s.db.QueryRowContext(ctx, `
			SELECT pr.project_id FROM cues c
			JOIN project_revisions pr ON pr.revision_id = c.revision_id
			WHERE c.cue_id = ?`, *cueID).Scan(&cueProject); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read note cue: %w", err)
		}
		if cueProject != projectID {
			return fmt.Errorf("%w: note cue belongs to another project", domain.ErrInvalidInput)
		}
	}
	return nil
}

func scanNote(row rowScanner) (domain.Note, error) {
	var note domain.Note
	var sessionID, cueID sql.NullString
	var status string
	var createdUS, updatedUS int64
	var resolvedUS sql.NullInt64
	if err := row.Scan(&note.ID, &note.ProjectID, &sessionID, &cueID, &note.Category, &note.Body, &status,
		&note.CreatedBy, &createdUS, &updatedUS, &resolvedUS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Note{}, domain.ErrNotFound
		}
		return domain.Note{}, fmt.Errorf("scan operator note: %w", err)
	}
	if sessionID.Valid {
		v := sessionID.String
		note.SessionID = &v
	}
	if cueID.Valid {
		v := cueID.String
		note.CueID = &v
	}
	note.Status = domain.NoteStatus(status)
	note.CreatedAt = clock.FromUnixMicros(createdUS)
	note.UpdatedAt = clock.FromUnixMicros(updatedUS)
	if resolvedUS.Valid {
		v := clock.FromUnixMicros(resolvedUS.Int64)
		note.ResolvedAt = &v
	}
	return note, nil
}

func scanSession(row rowScanner) (domain.Session, error) {
	var session domain.Session
	var sessionType, status string
	var startedUS int64
	var endedUS sql.NullInt64
	var current sql.NullString
	if err := row.Scan(&session.ID, &session.ProjectID, &session.RuntimeSnapshotID, &sessionType, &session.Name,
		&startedUS, &endedUS, &status, &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, domain.ErrNotFound
		}
		return domain.Session{}, fmt.Errorf("scan session: %w", err)
	}
	session.Type = domain.SessionType(sessionType)
	session.Status = domain.SessionStatus(status)
	session.StartedAt = clock.FromUnixMicros(startedUS)
	if endedUS.Valid {
		v := clock.FromUnixMicros(endedUS.Int64)
		session.EndedAt = &v
	}
	if current.Valid {
		v := current.String
		session.CurrentCueID = &v
	}
	return session, nil
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
