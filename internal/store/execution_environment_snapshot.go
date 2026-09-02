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

type ExecutionEnvironmentSnapshot struct {
	ID string
	EnvironmentManifestID string
	RevisionID string
	Snapshot executionenv.Snapshot
	ContentSHA256 string
	CreatedBy string
	CreatedAt time.Time
}

type executionEnvironmentSnapshotScanner interface { Scan(dest ...any) error }

func (s *Store) CreateExecutionEnvironmentSnapshot(ctx context.Context, manifestID string, snapshot executionenv.Snapshot, createdBy string) (ExecutionEnvironmentSnapshot, error) {
	manifestID = strings.TrimSpace(manifestID)
	createdBy = strings.TrimSpace(createdBy)
	if manifestID == "" || createdBy == "" || len(createdBy) > 256 {
		return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: execution environment manifest and bounded creation actor are required", domain.ErrInvalidInput)
	}
	manifest, err := s.GetExecutionEnvironmentManifest(ctx, manifestID)
	if err != nil { return ExecutionEnvironmentSnapshot{}, err }
	revision, err := s.GetRevision(ctx, manifest.RevisionID)
	if err != nil { return ExecutionEnvironmentSnapshot{}, err }
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil { return ExecutionEnvironmentSnapshot{}, err }
	if err := s.ensureDraft(ctx, s.db, manifest.RevisionID); err != nil { return ExecutionEnvironmentSnapshot{}, err }

	canonical, err := executionenv.SnapshotCanonicalBytes(snapshot)
	if err != nil { return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: execution environment snapshot: %v", domain.ErrInvalidInput, err) }
	normalized, err := executionenv.DecodeCanonicalSnapshot(canonical)
	if err != nil { return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: canonical execution environment snapshot: %v", domain.ErrInvalidInput, err) }
	if normalized.EnvironmentKey != manifest.Manifest.EnvironmentKey || normalized.AdapterKey != manifest.Manifest.AdapterKey || !strings.EqualFold(normalized.SourceManifestSHA256, manifest.ContentSHA256) {
		return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: snapshot source identity does not match execution environment manifest", domain.ErrConflict)
	}
	for _, item := range normalized.Items {
		if item.Portability != executionenv.SnapshotContentBound { continue }
		var storedSize int64
		err := s.db.QueryRowContext(ctx, `SELECT size_bytes FROM vault_objects WHERE content_hash = ?`, item.ContentHash).Scan(&storedSize)
		if errors.Is(err, sql.ErrNoRows) {
			return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: snapshot item %q claims CONTENT_BOUND bytes missing from Vault", domain.ErrConflict, item.Key)
		}
		if err != nil { return ExecutionEnvironmentSnapshot{}, fmt.Errorf("verify snapshot Vault object: %w", err) }
		if item.SizeBytes == nil || storedSize != *item.SizeBytes {
			return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: snapshot item %q Vault size mismatch", domain.ErrConflict, item.Key)
		}
	}
	contentHash, err := executionenv.SnapshotContentHash(normalized)
	if err != nil { return ExecutionEnvironmentSnapshot{}, fmt.Errorf("hash execution environment snapshot: %w", err) }
	snapshotID, err := stageid.New()
	if err != nil { return ExecutionEnvironmentSnapshot{}, err }
	now := s.clock.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO execution_environment_snapshots (
			environment_snapshot_id, environment_manifest_id, revision_id, source_manifest_sha256,
			snapshot_json, content_sha256, created_by, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshotID, manifest.ID, manifest.RevisionID, normalized.SourceManifestSHA256,
		string(canonical), contentHash, createdBy, clock.UnixMicros(now),
	)
	if err != nil { return ExecutionEnvironmentSnapshot{}, mapExecutionEnvironmentSnapshotWriteError("create execution environment snapshot", err) }
	return s.GetExecutionEnvironmentSnapshot(ctx, snapshotID)
}

func (s *Store) GetExecutionEnvironmentSnapshot(ctx context.Context, snapshotID string) (ExecutionEnvironmentSnapshot, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT ees.environment_snapshot_id, ees.environment_manifest_id, ees.revision_id,
		       ees.source_manifest_sha256, ees.snapshot_json, ees.content_sha256, ees.created_by, ees.created_at_us,
		       eem.environment_key, eem.adapter_key, eem.content_sha256
		FROM execution_environment_snapshots ees
		JOIN execution_environment_manifests eem ON eem.environment_manifest_id = ees.environment_manifest_id
		WHERE ees.environment_snapshot_id = ?`, strings.TrimSpace(snapshotID))
	return scanExecutionEnvironmentSnapshot(row)
}

func (s *Store) ListExecutionEnvironmentSnapshots(ctx context.Context, manifestID string) ([]ExecutionEnvironmentSnapshot, error) {
	manifestID = strings.TrimSpace(manifestID)
	if manifestID == "" { return nil, fmt.Errorf("%w: execution environment manifest is required", domain.ErrInvalidInput) }
	if _, err := s.GetExecutionEnvironmentManifest(ctx, manifestID); err != nil { return nil, err }
	rows, err := s.db.QueryContext(ctx, `
		SELECT ees.environment_snapshot_id, ees.environment_manifest_id, ees.revision_id,
		       ees.source_manifest_sha256, ees.snapshot_json, ees.content_sha256, ees.created_by, ees.created_at_us,
		       eem.environment_key, eem.adapter_key, eem.content_sha256
		FROM execution_environment_snapshots ees
		JOIN execution_environment_manifests eem ON eem.environment_manifest_id = ees.environment_manifest_id
		WHERE ees.environment_manifest_id = ?
		ORDER BY ees.created_at_us, ees.environment_snapshot_id`, manifestID)
	if err != nil { return nil, fmt.Errorf("list execution environment snapshots: %w", err) }
	defer rows.Close()
	items := make([]ExecutionEnvironmentSnapshot, 0)
	for rows.Next() {
		item, err := scanExecutionEnvironmentSnapshot(rows)
		if err != nil { return nil, err }
		items = append(items, item)
	}
	if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate execution environment snapshots: %w", err) }
	return items, nil
}

func (s *Store) GetLatestExecutionEnvironmentSnapshot(ctx context.Context, manifestID string) (ExecutionEnvironmentSnapshot, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT ees.environment_snapshot_id, ees.environment_manifest_id, ees.revision_id,
		       ees.source_manifest_sha256, ees.snapshot_json, ees.content_sha256, ees.created_by, ees.created_at_us,
		       eem.environment_key, eem.adapter_key, eem.content_sha256
		FROM execution_environment_snapshots ees
		JOIN execution_environment_manifests eem ON eem.environment_manifest_id = ees.environment_manifest_id
		WHERE ees.environment_manifest_id = ?
		ORDER BY ees.created_at_us DESC, ees.environment_snapshot_id DESC
		LIMIT 1`, strings.TrimSpace(manifestID))
	return scanExecutionEnvironmentSnapshot(row)
}

func (s *Store) DeleteExecutionEnvironmentSnapshot(ctx context.Context, snapshotID string) error {
	item, err := s.GetExecutionEnvironmentSnapshot(ctx, strings.TrimSpace(snapshotID))
	if err != nil { return err }
	revision, err := s.GetRevision(ctx, item.RevisionID)
	if err != nil { return err }
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil { return err }
	if err := s.ensureDraft(ctx, s.db, item.RevisionID); err != nil { return err }
	result, err := s.db.ExecContext(ctx, `DELETE FROM execution_environment_snapshots WHERE environment_snapshot_id = ?`, item.ID)
	if err != nil { return mapExecutionEnvironmentSnapshotWriteError("delete execution environment snapshot", err) }
	rows, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("delete execution environment snapshot rows affected: %w", err) }
	if rows != 1 { return domain.ErrNotFound }
	return nil
}

func scanExecutionEnvironmentSnapshot(row executionEnvironmentSnapshotScanner) (ExecutionEnvironmentSnapshot, error) {
	var item ExecutionEnvironmentSnapshot
	var sourceHash, snapshotJSON, environmentKey, adapterKey, manifestHash string
	var createdUS int64
	if err := row.Scan(&item.ID, &item.EnvironmentManifestID, &item.RevisionID, &sourceHash, &snapshotJSON, &item.ContentSHA256, &item.CreatedBy, &createdUS, &environmentKey, &adapterKey, &manifestHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) { return ExecutionEnvironmentSnapshot{}, domain.ErrNotFound }
		return ExecutionEnvironmentSnapshot{}, fmt.Errorf("scan execution environment snapshot: %w", err)
	}
	snapshot, err := executionenv.DecodeCanonicalSnapshot([]byte(snapshotJSON))
	if err != nil { return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: stored execution environment snapshot is invalid: %v", domain.ErrConflict, err) }
	hash, err := executionenv.SnapshotContentHash(snapshot)
	if err != nil { return ExecutionEnvironmentSnapshot{}, fmt.Errorf("hash stored execution environment snapshot: %w", err) }
	if !strings.EqualFold(hash, item.ContentSHA256) || !strings.EqualFold(snapshot.SourceManifestSHA256, sourceHash) || !strings.EqualFold(sourceHash, manifestHash) || snapshot.EnvironmentKey != environmentKey || snapshot.AdapterKey != adapterKey {
		return ExecutionEnvironmentSnapshot{}, fmt.Errorf("%w: stored execution environment snapshot identity mismatch", domain.ErrConflict)
	}
	item.Snapshot = snapshot
	item.ContentSHA256 = strings.ToLower(hash)
	item.CreatedAt = clock.FromUnixMicros(createdUS)
	return item, nil
}

func mapExecutionEnvironmentSnapshotWriteError(operation string, err error) error {
	if IsShowConfigurationLockedError(err) { return fmt.Errorf("%w: %s", domain.ErrShowConfigurationLocked, operation) }
	if strings.Contains(err.Error(), "EXECUTION_ENVIRONMENT_SNAPSHOT_REVISION_MISMATCH") { return fmt.Errorf("%w: execution environment snapshot revision mismatch", domain.ErrConflict) }
	if strings.Contains(err.Error(), "UNIQUE constraint failed") { return fmt.Errorf("%w: identical execution environment snapshot already exists", domain.ErrConflict) }
	return fmt.Errorf("%s: %w", operation, err)
}
