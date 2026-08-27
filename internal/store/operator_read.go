package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT project_id, name, description, lifecycle_state, current_revision_id,
		       default_venue_profile_id, created_at_us, updated_at_us
		FROM projects
		ORDER BY updated_at_us DESC, project_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		var project domain.Project
		var lifecycle string
		var currentRevision, venue sql.NullString
		var createdUS, updatedUS int64
		if err := rows.Scan(
			&project.ID, &project.Name, &project.Description, &lifecycle,
			&currentRevision, &venue, &createdUS, &updatedUS,
		); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		project.LifecycleState = domain.ProjectLifecycle(lifecycle)
		if currentRevision.Valid {
			project.CurrentRevisionID = currentRevision.String
		}
		if venue.Valid {
			value := venue.String
			project.DefaultVenueProfileID = &value
		}
		project.CreatedAt = clock.FromUnixMicros(createdUS)
		project.UpdatedAt = clock.FromUnixMicros(updatedUS)
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return projects, nil
}

func (s *Store) LatestPublishedRuntimeSnapshotForProject(ctx context.Context, projectID string) (*domain.RuntimeSnapshot, error) {
	var snapshot domain.RuntimeSnapshot
	var createdUS int64
	var manifest, status string
	err := s.db.QueryRowContext(ctx, `
		SELECT runtime_snapshot_id, project_id, revision_id, snapshot_version,
		       created_at_us, created_by, content_hash, manifest_json, status
		FROM runtime_snapshots
		WHERE project_id = ? AND status = 'PUBLISHED'
		ORDER BY snapshot_version DESC
		LIMIT 1
	`, projectID).Scan(
		&snapshot.ID, &snapshot.ProjectID, &snapshot.RevisionID, &snapshot.SnapshotVersion,
		&createdUS, &snapshot.CreatedBy, &snapshot.ContentHash, &manifest, &status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read latest published snapshot: %w", err)
	}
	snapshot.CreatedAt = clock.FromUnixMicros(createdUS)
	snapshot.Manifest = json.RawMessage(manifest)
	snapshot.Status = domain.RuntimeSnapshotStatus(status)
	return &snapshot, nil
}

func (s *Store) ActiveSessionForProject(ctx context.Context, projectID string) (*domain.Session, error) {
	var session domain.Session
	var sessionType, status string
	var startedUS int64
	var endedUS sql.NullInt64
	var currentCue sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT session_id, project_id, runtime_snapshot_id, session_type, name,
		       started_at_us, ended_at_us, status, current_cue_id
		FROM sessions
		WHERE project_id = ? AND status = 'ACTIVE'
		ORDER BY started_at_us DESC
		LIMIT 1
	`, projectID).Scan(
		&session.ID, &session.ProjectID, &session.RuntimeSnapshotID, &sessionType,
		&session.Name, &startedUS, &endedUS, &status, &currentCue,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read active project session: %w", err)
	}
	session.Type = domain.SessionType(sessionType)
	session.Status = domain.SessionStatus(status)
	session.StartedAt = clock.FromUnixMicros(startedUS)
	if endedUS.Valid {
		value := clock.FromUnixMicros(endedUS.Int64)
		session.EndedAt = &value
	}
	if currentCue.Valid {
		value := currentCue.String
		session.CurrentCueID = &value
	}
	return &session, nil
}

func (s *Store) SessionRuntimeErrorCount(ctx context.Context, sessionID string) (int64, error) {
	if sessionID == "" {
		return 0, nil
	}
	var count int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM action_executions ae
		JOIN cue_executions ce ON ce.cue_execution_id = ae.cue_execution_id
		WHERE ce.session_id = ? AND ae.result IN ('FAILED', 'TIMED_OUT')
	`, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count session runtime errors: %w", err)
	}
	return count, nil
}
