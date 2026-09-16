package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

type VisualEngineConfiguration struct {
	RevisionID string
	Mode       visualengine.EngineMode
	Explicit   bool
	UpdatedBy  string
	UpdatedAt  time.Time
}

// GetVisualEngineConfiguration returns revision-scoped authoring truth. Older
// revisions with no explicit F-026 Slice F2 row remain compatible and resolve
// to NATIVE without mutating persisted state.
func (s *Store) GetVisualEngineConfiguration(ctx context.Context, revisionID string) (VisualEngineConfiguration, error) {
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		return VisualEngineConfiguration{}, fmt.Errorf("%w: revision is required", domain.ErrInvalidInput)
	}
	if _, err := s.GetRevision(ctx, revisionID); err != nil {
		return VisualEngineConfiguration{}, err
	}
	var mode string
	var updatedBy string
	var updatedAtUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT engine_mode, updated_by, updated_at_us
		FROM visual_engine_configurations
		WHERE revision_id = ?`, revisionID).Scan(&mode, &updatedBy, &updatedAtUS)
	if errors.Is(err, sql.ErrNoRows) {
		return VisualEngineConfiguration{RevisionID: revisionID, Mode: visualengine.ModeNative, Explicit: false}, nil
	}
	if err != nil {
		return VisualEngineConfiguration{}, fmt.Errorf("read Visual Engine configuration: %w", err)
	}
	parsed, err := visualengine.ParseEngineMode(mode)
	if err != nil {
		return VisualEngineConfiguration{}, fmt.Errorf("%w: persisted Visual Engine mode is invalid", domain.ErrConflict)
	}
	return VisualEngineConfiguration{
		RevisionID: revisionID,
		Mode:       parsed,
		Explicit:   true,
		UpdatedBy:  updatedBy,
		UpdatedAt:  clock.FromUnixMicros(updatedAtUS),
	}, nil
}

// SetVisualEngineConfiguration updates only the authoring workflow mode. It
// never dispatches renderer commands or changes Cue/Output/Target authority.
func (s *Store) SetVisualEngineConfiguration(ctx context.Context, revisionID string, mode visualengine.EngineMode, updatedBy string) (VisualEngineConfiguration, error) {
	revisionID = strings.TrimSpace(revisionID)
	updatedBy = strings.TrimSpace(updatedBy)
	if revisionID == "" || updatedBy == "" || len(updatedBy) > 256 || !mode.Valid() {
		return VisualEngineConfiguration{}, fmt.Errorf("%w: revision, supported mode and bounded update actor are required", domain.ErrInvalidInput)
	}
	revision, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return VisualEngineConfiguration{}, err
	}
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil {
		return VisualEngineConfiguration{}, err
	}
	if err := s.ensureDraft(ctx, s.db, revisionID); err != nil {
		return VisualEngineConfiguration{}, err
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO visual_engine_configurations (revision_id, engine_mode, updated_by, updated_at_us)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(revision_id) DO UPDATE SET
			engine_mode = excluded.engine_mode,
			updated_by = excluded.updated_by,
			updated_at_us = excluded.updated_at_us`,
		revisionID, string(mode), updatedBy, clock.UnixMicros(now)); err != nil {
		if IsShowConfigurationLockedError(err) {
			return VisualEngineConfiguration{}, fmt.Errorf("%w: Visual Engine configuration is immutable while SHOW is active", domain.ErrShowConfigurationLocked)
		}
		return VisualEngineConfiguration{}, fmt.Errorf("set Visual Engine configuration: %w", err)
	}
	return s.GetVisualEngineConfiguration(ctx, revisionID)
}
