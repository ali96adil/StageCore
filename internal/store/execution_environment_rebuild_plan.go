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
	"github.com/ali96adil/StageCore/internal/executionenv"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type ExecutionEnvironmentRebuildPlan struct {
	ID                    string
	EnvironmentManifestID string
	RevisionID            string
	SourceSnapshotID      string
	Plan                  executionenv.AssistedRebuildPlan
	ContentSHA256         string
	CreatedBy             string
	CreatedAt             time.Time
	UpdatedBy             string
	UpdatedAt             time.Time
}

type executionEnvironmentRebuildPlanRaw struct {
	ExecutionEnvironmentRebuildPlan
	SourceSnapshotSHA256 string
	PlanJSON             string
}

func (s *Store) UpsertExecutionEnvironmentRebuildPlan(
	ctx context.Context,
	manifestID, sourceSnapshotID string,
	plan executionenv.AssistedRebuildPlan,
	actor string,
) (ExecutionEnvironmentRebuildPlan, error) {
	manifestID = strings.TrimSpace(manifestID)
	sourceSnapshotID = strings.TrimSpace(sourceSnapshotID)
	actor = strings.TrimSpace(actor)
	if manifestID == "" || sourceSnapshotID == "" || actor == "" || len(actor) > 256 {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: manifest, source snapshot and bounded actor are required", domain.ErrInvalidInput)
	}
	manifest, err := s.GetExecutionEnvironmentManifest(ctx, manifestID)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}
	revision, err := s.GetRevision(ctx, manifest.RevisionID)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}
	if err := s.ensureDraft(ctx, s.db, manifest.RevisionID); err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}
	source, err := s.GetExecutionEnvironmentSnapshot(ctx, sourceSnapshotID)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}
	if source.EnvironmentManifestID != manifest.ID || source.RevisionID != manifest.RevisionID {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: source snapshot does not belong to the execution environment revision", domain.ErrConflict)
	}
	normalized, err := executionenv.NormalizeAssistedRebuildPlan(plan, source.Snapshot)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: assisted rebuild plan: %v", domain.ErrInvalidInput, err)
	}
	canonical, err := executionenv.AssistedRebuildPlanCanonicalBytes(normalized, source.Snapshot)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: assisted rebuild plan: %v", domain.ErrInvalidInput, err)
	}
	contentHash, err := executionenv.AssistedRebuildPlanContentHash(normalized, source.Snapshot)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}

	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)
	planID, err := stageid.New()
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO execution_environment_rebuild_plans (
			rebuild_plan_id, environment_manifest_id, revision_id, source_snapshot_id,
			source_snapshot_sha256, plan_json, content_sha256,
			created_by, created_at_us, updated_by, updated_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(environment_manifest_id) DO UPDATE SET
			source_snapshot_id = excluded.source_snapshot_id,
			source_snapshot_sha256 = excluded.source_snapshot_sha256,
			plan_json = excluded.plan_json,
			content_sha256 = excluded.content_sha256,
			updated_by = excluded.updated_by,
			updated_at_us = excluded.updated_at_us`,
		planID, manifest.ID, manifest.RevisionID, source.ID,
		source.ContentSHA256, string(canonical), contentHash,
		actor, nowUS, actor, nowUS,
	)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, mapExecutionEnvironmentRebuildPlanWriteError("upsert assisted rebuild plan", err)
	}
	return s.GetExecutionEnvironmentRebuildPlan(ctx, manifest.ID)
}

func (s *Store) GetExecutionEnvironmentRebuildPlan(ctx context.Context, manifestID string) (ExecutionEnvironmentRebuildPlan, error) {
	raw, err := scanExecutionEnvironmentRebuildPlanRaw(s.db.QueryRowContext(ctx, `
		SELECT rebuild_plan_id, environment_manifest_id, revision_id, source_snapshot_id,
		       source_snapshot_sha256, plan_json, content_sha256,
		       created_by, created_at_us, updated_by, updated_at_us
		FROM execution_environment_rebuild_plans
		WHERE environment_manifest_id = ?`, strings.TrimSpace(manifestID)))
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, err
	}
	source, err := s.GetExecutionEnvironmentSnapshot(ctx, raw.SourceSnapshotID)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: assisted rebuild plan source snapshot unavailable: %v", domain.ErrConflict, err)
	}
	if source.EnvironmentManifestID != raw.EnvironmentManifestID ||
		source.RevisionID != raw.RevisionID ||
		!strings.EqualFold(source.ContentSHA256, raw.SourceSnapshotSHA256) {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: assisted rebuild plan source identity mismatch", domain.ErrConflict)
	}
	decoded, err := executionenv.DecodeCanonicalAssistedRebuildPlan([]byte(raw.PlanJSON), source.Snapshot)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: stored assisted rebuild plan is invalid: %v", domain.ErrConflict, err)
	}
	hash, err := executionenv.AssistedRebuildPlanContentHash(decoded, source.Snapshot)
	if err != nil {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("hash stored assisted rebuild plan: %w", err)
	}
	if !strings.EqualFold(hash, raw.ContentSHA256) ||
		!strings.EqualFold(decoded.SourceSnapshotSHA256, raw.SourceSnapshotSHA256) {
		return ExecutionEnvironmentRebuildPlan{}, fmt.Errorf("%w: stored assisted rebuild plan identity mismatch", domain.ErrConflict)
	}
	raw.Plan = decoded
	raw.ContentSHA256 = strings.ToLower(hash)
	return raw.ExecutionEnvironmentRebuildPlan, nil
}

func (s *Store) DeleteExecutionEnvironmentRebuildPlan(ctx context.Context, manifestID string) error {
	item, err := s.GetExecutionEnvironmentRebuildPlan(ctx, strings.TrimSpace(manifestID))
	if err != nil {
		return err
	}
	revision, err := s.GetRevision(ctx, item.RevisionID)
	if err != nil {
		return err
	}
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil {
		return err
	}
	if err := s.ensureDraft(ctx, s.db, item.RevisionID); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM execution_environment_rebuild_plans WHERE rebuild_plan_id = ?`, item.ID)
	if err != nil {
		return mapExecutionEnvironmentRebuildPlanWriteError("delete assisted rebuild plan", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete assisted rebuild plan rows affected: %w", err)
	}
	if rows != 1 {
		return domain.ErrNotFound
	}
	return nil
}

type executionEnvironmentRebuildPlanScanner interface {
	Scan(dest ...any) error
}

func scanExecutionEnvironmentRebuildPlanRaw(row executionEnvironmentRebuildPlanScanner) (executionEnvironmentRebuildPlanRaw, error) {
	var raw executionEnvironmentRebuildPlanRaw
	var createdUS, updatedUS int64
	if err := row.Scan(
		&raw.ID, &raw.EnvironmentManifestID, &raw.RevisionID, &raw.SourceSnapshotID,
		&raw.SourceSnapshotSHA256, &raw.PlanJSON, &raw.ContentSHA256,
		&raw.CreatedBy, &createdUS, &raw.UpdatedBy, &updatedUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return executionEnvironmentRebuildPlanRaw{}, domain.ErrNotFound
		}
		return executionEnvironmentRebuildPlanRaw{}, fmt.Errorf("scan assisted rebuild plan: %w", err)
	}
	raw.CreatedAt = clock.FromUnixMicros(createdUS)
	raw.UpdatedAt = clock.FromUnixMicros(updatedUS)
	return raw, nil
}

func mapExecutionEnvironmentRebuildPlanWriteError(operation string, err error) error {
	if IsShowConfigurationLockedError(err) {
		return fmt.Errorf("%w: %s", domain.ErrShowConfigurationLocked, operation)
	}
	if strings.Contains(err.Error(), "EXECUTION_ENVIRONMENT_REBUILD_PLAN_SCOPE_MISMATCH") {
		return fmt.Errorf("%w: assisted rebuild plan source scope mismatch", domain.ErrConflict)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
