package store

import (
	"context"
	"fmt"

	"github.com/ali96adil/StageCore/internal/domain"
)

// ReorderDraftCues atomically replaces the complete Cue order for one Draft
// revision. The caller must provide every Cue ID exactly once. A temporary
// positive offset avoids UNIQUE(revision_id, order_index) collisions while
// swapping adjacent Cues.
func (s *Store) ReorderDraftCues(ctx context.Context, revisionID string, cueIDs []string) ([]domain.Cue, error) {
	if len(cueIDs) == 0 {
		return nil, fmt.Errorf("%w: cue order cannot be empty", domain.ErrInvalidInput)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin cue reorder: %w", err)
	}
	defer tx.Rollback()
	if err := s.ensureDraft(ctx, tx, revisionID); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `SELECT cue_id FROM cues WHERE revision_id = ?`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list cue IDs for reorder: %w", err)
	}
	current := make(map[string]struct{})
	for rows.Next() {
		var cueID string
		if err := rows.Scan(&cueID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan cue ID for reorder: %w", err)
		}
		current[cueID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate cue IDs for reorder: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close cue reorder rows: %w", err)
	}
	if len(current) != len(cueIDs) {
		return nil, fmt.Errorf("%w: reorder must contain every Cue exactly once", domain.ErrInvalidInput)
	}
	seen := make(map[string]struct{}, len(cueIDs))
	for _, cueID := range cueIDs {
		if _, exists := current[cueID]; !exists {
			return nil, fmt.Errorf("%w: Cue %s does not belong to revision", domain.ErrInvalidInput, cueID)
		}
		if _, duplicate := seen[cueID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Cue in reorder", domain.ErrInvalidInput)
		}
		seen[cueID] = struct{}{}
	}

	const temporaryOffset = 1000000000
	if _, err := tx.ExecContext(ctx, `UPDATE cues SET order_index = order_index + ? WHERE revision_id = ?`, temporaryOffset, revisionID); err != nil {
		return nil, fmt.Errorf("stage cue reorder: %w", err)
	}
	for index, cueID := range cueIDs {
		result, err := tx.ExecContext(ctx, `UPDATE cues SET order_index = ? WHERE cue_id = ? AND revision_id = ?`, index+1, cueID, revisionID)
		if err != nil {
			return nil, fmt.Errorf("apply cue reorder: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			if err != nil {
				return nil, fmt.Errorf("cue reorder rows affected: %w", err)
			}
			return nil, domain.ErrConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit cue reorder: %w", err)
	}
	return s.ListCues(ctx, revisionID)
}
