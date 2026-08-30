package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type CreateSessionFoundationParams struct {
	SnapshotID    string
	SessionType   domain.SessionType
	Name          string
	StartPosition domain.SessionStartPosition
}

func (s *Store) CreateSessionAtPosition(ctx context.Context, p CreateSessionFoundationParams) (domain.Session, error) {
	if p.SessionType != domain.SessionSimulation && p.SessionType != domain.SessionRehearsal && p.SessionType != domain.SessionShow {
		return domain.Session{}, fmt.Errorf("%w: unsupported session type", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(p.SnapshotID) == "" {
		return domain.Session{}, fmt.Errorf("%w: runtime snapshot is required", domain.ErrInvalidInput)
	}

	position := p.StartPosition
	if position.Version == 0 {
		position.Version = domain.SessionContractVersion1
	}
	if position.Version != domain.SessionContractVersion1 {
		return domain.Session{}, fmt.Errorf("%w: unsupported start position version", domain.ErrInvalidInput)
	}
	if position.Kind == "" {
		position.Kind = domain.SessionStartBeginning
	}
	metadataJSON, err := normalizeJSON(position.Metadata, "{}")
	if err != nil {
		return domain.Session{}, fmt.Errorf("start position metadata: %w", err)
	}
	position.Metadata = json.RawMessage(metadataJSON)

	var projectID, revisionID, snapshotStatus string
	if err := s.db.QueryRowContext(ctx, `
		SELECT project_id, revision_id, status
		FROM runtime_snapshots
		WHERE runtime_snapshot_id = ?`, p.SnapshotID).Scan(&projectID, &revisionID, &snapshotStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, domain.ErrNotFound
		}
		return domain.Session{}, fmt.Errorf("read session snapshot: %w", err)
	}
	if domain.RuntimeSnapshotStatus(snapshotStatus) != domain.SnapshotPublished {
		return domain.Session{}, fmt.Errorf("%w: session requires PUBLISHED snapshot", domain.ErrConflict)
	}

	var nextCueID *string
	truth := domain.SessionStateTruth{
		Version:           domain.SessionContractVersion1,
		RestorationStatus: domain.SessionRestorationNotAssessed,
	}
	switch position.Kind {
	case domain.SessionStartBeginning:
		if position.CueID != nil {
			return domain.Session{}, fmt.Errorf("%w: BEGINNING start cannot include cue_id", domain.ErrInvalidInput)
		}
		truth.RestorationStatus = domain.SessionRestorationNotRequired
	case domain.SessionStartCue:
		if p.SessionType == domain.SessionShow {
			return domain.Session{}, fmt.Errorf("%w: SHOW selected-cue start requires the later recovery/preparation flow", domain.ErrConflict)
		}
		cueID := cleanOptional(position.CueID)
		if cueID == nil {
			return domain.Session{}, fmt.Errorf("%w: CUE start requires cue_id", domain.ErrInvalidInput)
		}
		var enabled int
		if err := s.db.QueryRowContext(ctx, `
			SELECT enabled FROM cues WHERE cue_id = ? AND revision_id = ?`, *cueID, revisionID).Scan(&enabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Session{}, fmt.Errorf("%w: start cue is not part of the Runtime Snapshot", domain.ErrConflict)
			}
			return domain.Session{}, fmt.Errorf("read session start cue: %w", err)
		}
		if enabled != 1 {
			return domain.Session{}, fmt.Errorf("%w: start cue is disabled", domain.ErrConflict)
		}
		position.CueID = cueID
		nextCueID = cueID
		truth.RestorationStatus = domain.SessionRestorationManualConfirmationRequired
		truth.ManualConfirmationRequired = true
	case domain.SessionStartScene, domain.SessionStartRange, domain.SessionStartCheckpoint:
		return domain.Session{}, fmt.Errorf("%w: %s start is reserved for a later F-027 slice", domain.ErrConflict, position.Kind)
	case domain.SessionStartUnspecified:
		return domain.Session{}, fmt.Errorf("%w: UNSPECIFIED is reserved for migrated historical sessions", domain.ErrInvalidInput)
	default:
		return domain.Session{}, fmt.Errorf("%w: unsupported start position kind", domain.ErrInvalidInput)
	}

	sessionID, err := stageid.New()
	if err != nil {
		return domain.Session{}, err
	}
	now := s.clock.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			session_id, project_id, runtime_snapshot_id, session_type, name,
			started_at_us, status, session_contract_version, lifecycle_state,
			start_position_version, start_position_kind, start_cue_id,
			start_position_metadata_json, next_cue_id, state_truth_version,
			restoration_status, desired_state_ref, verified_state_ref,
			manual_confirmation_required
		) VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE', ?, 'ACTIVE', ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)`,
		sessionID, projectID, p.SnapshotID, p.SessionType, strings.TrimSpace(p.Name), clock.UnixMicros(now),
		domain.SessionContractVersion1, position.Version, position.Kind, nullableString(position.CueID), metadataJSON,
		nullableString(nextCueID), truth.Version, truth.RestorationStatus, boolInt(truth.ManualConfirmationRequired),
	)
	if err != nil {
		return domain.Session{}, fmt.Errorf("insert session foundation: %w", err)
	}
	return s.GetSessionFoundation(ctx, sessionID)
}

func (s *Store) GetSessionFoundation(ctx context.Context, sessionID string) (domain.Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT session_id, project_id, runtime_snapshot_id, session_type, name,
		       started_at_us, ended_at_us, status, current_cue_id,
		       session_contract_version, lifecycle_state, end_reason,
		       start_position_version, start_position_kind, start_cue_id,
		       start_position_metadata_json, last_completed_cue_id, next_cue_id,
		       state_truth_version, restoration_status, desired_state_ref,
		       verified_state_ref, manual_confirmation_required
		FROM sessions WHERE session_id = ?`, strings.TrimSpace(sessionID))
	return scanSessionFoundation(row)
}

func (s *Store) ListSessionFoundationsForProject(ctx context.Context, projectID string, limit int) ([]domain.Session, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, project_id, runtime_snapshot_id, session_type, name,
		       started_at_us, ended_at_us, status, current_cue_id,
		       session_contract_version, lifecycle_state, end_reason,
		       start_position_version, start_position_kind, start_cue_id,
		       start_position_metadata_json, last_completed_cue_id, next_cue_id,
		       state_truth_version, restoration_status, desired_state_ref,
		       verified_state_ref, manual_confirmation_required
		FROM sessions WHERE project_id = ?
		ORDER BY started_at_us DESC, session_id DESC LIMIT ?`, strings.TrimSpace(projectID), limit)
	if err != nil {
		return nil, fmt.Errorf("list project session foundations: %w", err)
	}
	defer rows.Close()
	out := make([]domain.Session, 0)
	for rows.Next() {
		session, err := scanSessionFoundation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (s *Store) EndSessionLifecycle(ctx context.Context, sessionID string, lifecycle domain.SessionLifecycleState, reason string) error {
	var coarse domain.SessionStatus
	switch lifecycle {
	case domain.SessionLifecycleCompleted, domain.SessionLifecycleStopped:
		coarse = domain.SessionCompleted
	case domain.SessionLifecycleSuspended:
		var sessionType string
		if err := s.db.QueryRowContext(ctx, `SELECT session_type FROM sessions WHERE session_id = ?`, sessionID).Scan(&sessionType); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read session type for suspension: %w", err)
		}
		if domain.SessionType(sessionType) != domain.SessionRehearsal {
			return fmt.Errorf("%w: only REHEARSAL can be suspended by the foundation lifecycle", domain.ErrConflict)
		}
		coarse = domain.SessionAborted
	case domain.SessionLifecycleAborted:
		coarse = domain.SessionAborted
	default:
		return fmt.Errorf("%w: terminal session lifecycle required", domain.ErrInvalidInput)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET status = ?, lifecycle_state = ?, end_reason = ?, ended_at_us = ?
		WHERE session_id = ? AND status = 'ACTIVE' AND lifecycle_state = 'ACTIVE'`,
		coarse, lifecycle, strings.TrimSpace(reason), clock.UnixMicros(s.clock.Now()), sessionID)
	if err != nil {
		return fmt.Errorf("end session lifecycle: %w", err)
	}
	return requireOneRow(result)
}

type sessionFoundationRowScanner interface {
	Scan(...any) error
}

func scanSessionFoundation(row sessionFoundationRowScanner) (domain.Session, error) {
	var session domain.Session
	var sessionType, status, lifecycle, startKind, metadata, restoration string
	var startedUS int64
	var endedUS sql.NullInt64
	var current, startCue, lastCompleted, nextCue sql.NullString
	var desiredState, verifiedState sql.NullString
	var manualConfirmation int
	if err := row.Scan(
		&session.ID, &session.ProjectID, &session.RuntimeSnapshotID, &sessionType, &session.Name,
		&startedUS, &endedUS, &status, &current,
		&session.ContractVersion, &lifecycle, &session.EndReason,
		&session.StartPosition.Version, &startKind, &startCue, &metadata,
		&lastCompleted, &nextCue, &session.StateTruth.Version, &restoration,
		&desiredState, &verifiedState, &manualConfirmation,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, domain.ErrNotFound
		}
		return domain.Session{}, fmt.Errorf("scan session foundation: %w", err)
	}
	session.Type = domain.SessionType(sessionType)
	session.Status = domain.SessionStatus(status)
	session.LifecycleState = domain.SessionLifecycleState(lifecycle)
	session.StartPosition.Kind = domain.SessionStartPositionKind(startKind)
	session.StartPosition.Metadata = json.RawMessage(metadata)
	session.StateTruth.RestorationStatus = domain.SessionRestorationStatus(restoration)
	session.StateTruth.ManualConfirmationRequired = manualConfirmation == 1
	session.StartedAt = clock.FromUnixMicros(startedUS)
	if endedUS.Valid {
		value := clock.FromUnixMicros(endedUS.Int64)
		session.EndedAt = &value
	}
	assignOptional := func(source sql.NullString, target **string) {
		if source.Valid {
			value := source.String
			*target = &value
		}
	}
	assignOptional(current, &session.CurrentCueID)
	assignOptional(startCue, &session.StartPosition.CueID)
	assignOptional(lastCompleted, &session.LastCompletedCueID)
	assignOptional(nextCue, &session.NextCueID)
	assignOptional(desiredState, &session.StateTruth.DesiredStateRef)
	assignOptional(verifiedState, &session.StateTruth.VerifiedStateRef)
	return session, nil
}
