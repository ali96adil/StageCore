package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

// GetVisualEngineMode returns the revision-scoped operator choice. Revisions
// created before F-026 Slice F2 intentionally default to EXTERNAL.
func (s *Store) GetVisualEngineMode(ctx context.Context, revisionID string) (visualengine.EngineMode, error) {
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		return "", domain.ErrInvalidInput
	}
	if _, err := s.GetRevision(ctx, revisionID); err != nil {
		return "", err
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `
		SELECT engine_mode
		FROM visual_engine_revision_settings
		WHERE revision_id = ?
	`, revisionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return visualengine.EngineModeExternal, nil
	}
	if err != nil {
		return "", fmt.Errorf("read Visual Engine mode: %w", err)
	}
	mode, ok := visualengine.ParseEngineMode(raw)
	if !ok {
		return "", fmt.Errorf("%w: unsupported Visual Engine mode %q", domain.ErrConflict, raw)
	}
	return mode, nil
}

// SetVisualEngineMode updates only an editable Draft revision. Callers that
// operate at Project scope should use EnsureProjectDraft first so validated
// revisions and published Runtime Snapshots remain immutable.
func (s *Store) SetVisualEngineMode(ctx context.Context, revisionID string, mode visualengine.EngineMode, updatedBy string) error {
	revisionID = strings.TrimSpace(revisionID)
	updatedBy = strings.TrimSpace(updatedBy)
	if revisionID == "" || updatedBy == "" || !mode.Valid() {
		return domain.ErrInvalidInput
	}
	revision, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return err
	}
	if revision.Status != domain.RevisionDraft {
		return fmt.Errorf("%w: Visual Engine mode requires a DRAFT revision", domain.ErrConflict)
	}
	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO visual_engine_revision_settings (revision_id, engine_mode, updated_by, updated_at_us)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(revision_id) DO UPDATE SET
			engine_mode = excluded.engine_mode,
			updated_by = excluded.updated_by,
			updated_at_us = excluded.updated_at_us
	`, revisionID, string(mode), updatedBy, nowUS); err != nil {
		return fmt.Errorf("persist Visual Engine mode: %w", err)
	}
	return nil
}
