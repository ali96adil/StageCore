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

type CreateProjectParams struct {
	Name        string
	Description string
	CreatedBy   string
	ChangeNote  string
}

func (s *Store) CreateProject(ctx context.Context, p CreateProjectParams) (domain.Project, domain.ProjectRevision, error) {
	if strings.TrimSpace(p.Name) == "" {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("%w: project name is required", domain.ErrInvalidInput)
	}
	projectID, err := stageid.New()
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	revisionID, err := stageid.New()
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, err
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("begin create project: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects (project_id, name, description, lifecycle_state, current_revision_id, created_at_us, updated_at_us)
		VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, projectID, strings.TrimSpace(p.Name), p.Description, nowUS, nowUS); err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("insert project: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_revisions (revision_id, project_id, revision_number, status, parent_revision_id, created_at_us, created_by, change_note)
		VALUES (?, ?, 1, 'DRAFT', NULL, ?, ?, ?)`, revisionID, projectID, nowUS, p.CreatedBy, p.ChangeNote); err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("insert initial revision: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET current_revision_id = ? WHERE project_id = ?`, revisionID, projectID); err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("set current revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.Project{}, domain.ProjectRevision{}, fmt.Errorf("commit create project: %w", err)
	}

	project := domain.Project{
		ID:                projectID,
		Name:              strings.TrimSpace(p.Name),
		Description:       p.Description,
		LifecycleState:    domain.ProjectActive,
		CurrentRevisionID: revisionID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	revision := domain.ProjectRevision{
		ID:             revisionID,
		ProjectID:      projectID,
		RevisionNumber: 1,
		Status:         domain.RevisionDraft,
		CreatedAt:      now,
		CreatedBy:      p.CreatedBy,
		ChangeNote:     p.ChangeNote,
	}
	return project, revision, nil
}

func (s *Store) GetProject(ctx context.Context, projectID string) (domain.Project, error) {
	var p domain.Project
	var lifecycle string
	var current sql.NullString
	var venue sql.NullString
	var createdUS, updatedUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT project_id, name, description, lifecycle_state, current_revision_id, default_venue_profile_id, created_at_us, updated_at_us
		FROM projects WHERE project_id = ?`, projectID).Scan(
		&p.ID, &p.Name, &p.Description, &lifecycle, &current, &venue, &createdUS, &updatedUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("get project: %w", err)
	}
	p.LifecycleState = domain.ProjectLifecycle(lifecycle)
	if current.Valid {
		p.CurrentRevisionID = current.String
	}
	if venue.Valid {
		p.DefaultVenueProfileID = &venue.String
	}
	p.CreatedAt = clock.FromUnixMicros(createdUS)
	p.UpdatedAt = clock.FromUnixMicros(updatedUS)
	return p, nil
}

func (s *Store) GetRevision(ctx context.Context, revisionID string) (domain.ProjectRevision, error) {
	var r domain.ProjectRevision
	var status string
	var parent sql.NullString
	var createdUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT revision_id, project_id, revision_number, status, parent_revision_id, created_at_us, created_by, change_note
		FROM project_revisions WHERE revision_id = ?`, revisionID).Scan(
		&r.ID, &r.ProjectID, &r.RevisionNumber, &status, &parent, &createdUS, &r.CreatedBy, &r.ChangeNote,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProjectRevision{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ProjectRevision{}, fmt.Errorf("get revision: %w", err)
	}
	r.Status = domain.RevisionStatus(status)
	if parent.Valid {
		r.ParentRevisionID = &parent.String
	}
	r.CreatedAt = clock.FromUnixMicros(createdUS)
	return r, nil
}

func (s *Store) SetRevisionStatus(ctx context.Context, revisionID string, next domain.RevisionStatus) error {
	current, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return err
	}
	valid := (current.Status == domain.RevisionDraft && next == domain.RevisionValidated) ||
		(current.Status == domain.RevisionValidated && next == domain.RevisionSuperseded)
	if !valid {
		return fmt.Errorf("%w: revision transition %s -> %s", domain.ErrConflict, current.Status, next)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE project_revisions SET status = ? WHERE revision_id = ? AND status = ?`, next, revisionID, current.Status)
	if err != nil {
		return fmt.Errorf("set revision status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revision status rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrConflict
	}
	return nil
}
