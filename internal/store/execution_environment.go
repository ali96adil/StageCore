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

type ExecutionEnvironmentManifest struct {
	ID            string
	RevisionID    string
	Manifest      executionenv.Manifest
	ContentSHA256 string
	CreatedBy     string
	CreatedAt     time.Time
}

type executionEnvironmentScanner interface {
	Scan(dest ...any) error
}

func (s *Store) CreateExecutionEnvironmentManifest(ctx context.Context, revisionID string, manifest executionenv.Manifest, createdBy string) (ExecutionEnvironmentManifest, error) {
	revisionID = strings.TrimSpace(revisionID)
	createdBy = strings.TrimSpace(createdBy)
	if revisionID == "" || createdBy == "" || len(createdBy) > 256 {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: revision and bounded creation actor are required", domain.ErrInvalidInput)
	}
	revision, err := s.GetRevision(ctx, revisionID)
	if err != nil {
		return ExecutionEnvironmentManifest{}, err
	}
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil {
		return ExecutionEnvironmentManifest{}, err
	}
	if err := s.ensureDraft(ctx, s.db, revisionID); err != nil {
		return ExecutionEnvironmentManifest{}, err
	}
	canonical, err := executionenv.CanonicalBytes(manifest)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: execution environment manifest: %v", domain.ErrInvalidInput, err)
	}
	normalized, err := executionenv.DecodeCanonical(canonical)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: canonical execution environment manifest: %v", domain.ErrInvalidInput, err)
	}
	contentHash, err := executionenv.ContentHash(normalized)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("hash execution environment manifest: %w", err)
	}
	manifestID, err := stageid.New()
	if err != nil {
		return ExecutionEnvironmentManifest{}, err
	}
	now := s.clock.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO execution_environment_manifests (
			environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
			manifest_json, content_sha256, created_by, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		manifestID, revisionID, normalized.EnvironmentKey, normalized.AdapterKey, normalized.Application.Key,
		string(canonical), contentHash, createdBy, clock.UnixMicros(now),
	)
	if err != nil {
		return ExecutionEnvironmentManifest{}, mapExecutionEnvironmentWriteError("create execution environment manifest", err)
	}
	return s.GetExecutionEnvironmentManifest(ctx, manifestID)
}

func (s *Store) GetExecutionEnvironmentManifest(ctx context.Context, manifestID string) (ExecutionEnvironmentManifest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
		       manifest_json, content_sha256, created_by, created_at_us
		FROM execution_environment_manifests
		WHERE environment_manifest_id = ?`, strings.TrimSpace(manifestID))
	return scanExecutionEnvironmentManifest(row)
}

func (s *Store) ListExecutionEnvironmentManifests(ctx context.Context, revisionID string) ([]ExecutionEnvironmentManifest, error) {
	revisionID = strings.TrimSpace(revisionID)
	if revisionID == "" {
		return nil, fmt.Errorf("%w: revision is required", domain.ErrInvalidInput)
	}
	if _, err := s.GetRevision(ctx, revisionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
		       manifest_json, content_sha256, created_by, created_at_us
		FROM execution_environment_manifests
		WHERE revision_id = ?
		ORDER BY environment_key, environment_manifest_id`, revisionID)
	if err != nil {
		return nil, fmt.Errorf("list execution environment manifests: %w", err)
	}
	defer rows.Close()
	items := make([]ExecutionEnvironmentManifest, 0)
	for rows.Next() {
		item, err := scanExecutionEnvironmentManifest(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate execution environment manifests: %w", err)
	}
	return items, nil
}

func (s *Store) DeleteExecutionEnvironmentManifest(ctx context.Context, manifestID string) error {
	item, err := s.GetExecutionEnvironmentManifest(ctx, strings.TrimSpace(manifestID))
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
	result, err := s.db.ExecContext(ctx, `DELETE FROM execution_environment_manifests WHERE environment_manifest_id = ?`, item.ID)
	if err != nil {
		return mapExecutionEnvironmentWriteError("delete execution environment manifest", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete execution environment manifest rows affected: %w", err)
	}
	if rows != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func scanExecutionEnvironmentManifest(row executionEnvironmentScanner) (ExecutionEnvironmentManifest, error) {
	var item ExecutionEnvironmentManifest
	var environmentKey, adapterKey, applicationKey, manifestJSON string
	var createdUS int64
	if err := row.Scan(
		&item.ID, &item.RevisionID, &environmentKey, &adapterKey, &applicationKey,
		&manifestJSON, &item.ContentSHA256, &item.CreatedBy, &createdUS,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExecutionEnvironmentManifest{}, domain.ErrNotFound
		}
		return ExecutionEnvironmentManifest{}, fmt.Errorf("scan execution environment manifest: %w", err)
	}
	manifest, err := executionenv.DecodeCanonical([]byte(manifestJSON))
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: stored execution environment manifest is invalid: %v", domain.ErrConflict, err)
	}
	contentHash, err := executionenv.ContentHash(manifest)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("hash stored execution environment manifest: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(item.ContentSHA256), contentHash) {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: stored execution environment manifest hash mismatch", domain.ErrConflict)
	}
	if manifest.EnvironmentKey != environmentKey || manifest.AdapterKey != adapterKey || manifest.Application.Key != applicationKey {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: stored execution environment manifest identity columns mismatch", domain.ErrConflict)
	}
	item.Manifest = manifest
	item.ContentSHA256 = strings.ToLower(contentHash)
	item.CreatedAt = clock.FromUnixMicros(createdUS)
	return item, nil
}

func mapExecutionEnvironmentWriteError(operation string, err error) error {
	if IsShowConfigurationLockedError(err) {
		return fmt.Errorf("%w: %s", domain.ErrShowConfigurationLocked, operation)
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return fmt.Errorf("%w: execution environment key already exists for revision", domain.ErrConflict)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
