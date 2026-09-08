package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

// DiscardProjectDraft abandons the current editable successor and restores its
// validated parent as the project's current revision. The abandoned revision
// is retained as SUPERSEDED so discard is auditable and no destructive cascade
// is required. A project whose current revision is already validated is an
// idempotent no-op. The initial parentless Draft cannot be discarded because
// there is no prior configuration to restore.
func (s *Store) DiscardProjectDraft(ctx context.Context, projectID, actor, reason string) (domain.ProjectRevision, bool, error) {
	projectID = strings.TrimSpace(projectID)
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if projectID == "" {
		return domain.ProjectRevision{}, false, fmt.Errorf("%w: project ID is required", domain.ErrInvalidInput)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, projectID); err != nil {
		return domain.ProjectRevision{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ProjectRevision{}, false, fmt.Errorf("begin discard draft: %w", err)
	}
	defer tx.Rollback()

	var currentID string
	if err := tx.QueryRowContext(ctx, `SELECT current_revision_id FROM projects WHERE project_id = ?`, projectID).Scan(&currentID); err != nil {
		if err == sql.ErrNoRows {
			return domain.ProjectRevision{}, false, domain.ErrNotFound
		}
		return domain.ProjectRevision{}, false, fmt.Errorf("read current revision for discard: %w", err)
	}

	var status string
	var parentID sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT status, parent_revision_id
		FROM project_revisions
		WHERE revision_id = ? AND project_id = ?
	`, currentID, projectID).Scan(&status, &parentID); err != nil {
		if err == sql.ErrNoRows {
			return domain.ProjectRevision{}, false, domain.ErrNotFound
		}
		return domain.ProjectRevision{}, false, fmt.Errorf("read current draft for discard: %w", err)
	}

	if domain.RevisionStatus(status) != domain.RevisionDraft {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			return domain.ProjectRevision{}, false, err
		}
		revision, err := s.GetRevision(ctx, currentID)
		return revision, false, err
	}
	if !parentID.Valid || strings.TrimSpace(parentID.String) == "" {
		return domain.ProjectRevision{}, false, fmt.Errorf("%w: initial Draft has no previous revision to restore", domain.ErrConflict)
	}

	var parentStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT status FROM project_revisions
		WHERE revision_id = ? AND project_id = ?
	`, parentID.String, projectID).Scan(&parentStatus); err != nil {
		if err == sql.ErrNoRows {
			return domain.ProjectRevision{}, false, domain.ErrConflict
		}
		return domain.ProjectRevision{}, false, fmt.Errorf("read parent revision for discard: %w", err)
	}
	if domain.RevisionStatus(parentStatus) != domain.RevisionValidated {
		return domain.ProjectRevision{}, false, fmt.Errorf("%w: Draft parent is %s, want VALIDATED", domain.ErrConflict, parentStatus)
	}

	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	result, err := tx.ExecContext(ctx, `
		UPDATE projects
		SET current_revision_id = ?, updated_at_us = ?
		WHERE project_id = ? AND current_revision_id = ?
	`, parentID.String, nowUS, projectID, currentID)
	if err != nil {
		return domain.ProjectRevision{}, false, fmt.Errorf("restore parent revision: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return domain.ProjectRevision{}, false, err
		}
		return domain.ProjectRevision{}, false, domain.ErrConflict
	}

	changeNote := "Discarded Draft"
	if actor != "" {
		changeNote += " by " + actor
	}
	if reason != "" {
		changeNote += ": " + reason
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE project_revisions
		SET status = 'SUPERSEDED', change_note = CASE
			WHEN trim(change_note) = '' THEN ?
			ELSE change_note || ' | ' || ?
		END
		WHERE revision_id = ? AND project_id = ? AND status = 'DRAFT'
	`, changeNote, changeNote, currentID, projectID)
	if err != nil {
		return domain.ProjectRevision{}, false, fmt.Errorf("supersede discarded Draft: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return domain.ProjectRevision{}, false, err
		}
		return domain.ProjectRevision{}, false, domain.ErrConflict
	}

	if err := tx.Commit(); err != nil {
		return domain.ProjectRevision{}, false, fmt.Errorf("commit discard draft: %w", err)
	}
	restored, err := s.GetRevision(ctx, parentID.String)
	if err != nil {
		return domain.ProjectRevision{}, false, err
	}
	return restored, true, nil
}
