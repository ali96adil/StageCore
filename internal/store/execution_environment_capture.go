package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
)

// CaptureExecutionEnvironmentAsset promotes one declared execution-environment
// asset to CONTENT_BOUND only when the immutable Vault already contains the
// exact SHA-256 object and byte size supplied by the caller.
//
// The manifest remains revision configuration: mutation is DRAFT-only and is
// protected by the existing F-012 SHOW configuration lock. Vault bytes are not
// copied or duplicated here; this operation only binds verified Vault identity
// into the canonical F-025 manifest.
func (s *Store) CaptureExecutionEnvironmentAsset(
	ctx context.Context,
	manifestID string,
	assetKey string,
	contentHash string,
	sizeBytes int64,
) (ExecutionEnvironmentManifest, error) {
	manifestID = strings.TrimSpace(manifestID)
	assetKey = strings.TrimSpace(assetKey)
	contentHash = strings.ToLower(strings.TrimSpace(contentHash))
	if manifestID == "" || assetKey == "" {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: execution environment and asset key are required", domain.ErrInvalidInput)
	}
	if len(contentHash) != 64 {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: content hash must be a SHA-256 digest", domain.ErrInvalidInput)
	}
	if _, err := hex.DecodeString(contentHash); err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: content hash must be hexadecimal SHA-256: %v", domain.ErrInvalidInput, err)
	}
	if sizeBytes < 0 {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: captured asset size cannot be negative", domain.ErrInvalidInput)
	}

	item, err := s.GetExecutionEnvironmentManifest(ctx, manifestID)
	if err != nil {
		return ExecutionEnvironmentManifest{}, err
	}
	revision, err := s.GetRevision(ctx, item.RevisionID)
	if err != nil {
		return ExecutionEnvironmentManifest{}, err
	}
	// SHOW lock intentionally takes precedence over frozen revision state.
	if err := s.RequireProjectConfigurationMutable(ctx, revision.ProjectID); err != nil {
		return ExecutionEnvironmentManifest{}, err
	}

	assetIndex := -1
	for i := range item.Manifest.Assets {
		if item.Manifest.Assets[i].Key == assetKey {
			assetIndex = i
			break
		}
	}
	if assetIndex < 0 {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: execution environment asset %q", domain.ErrNotFound, assetKey)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("begin execution environment asset capture: %w", err)
	}
	defer tx.Rollback()

	if err := s.ensureDraft(ctx, tx, item.RevisionID); err != nil {
		return ExecutionEnvironmentManifest{}, err
	}

	var vaultSize int64
	if err := tx.QueryRowContext(ctx, `
		SELECT size_bytes
		FROM vault_objects
		WHERE content_hash = ?`, contentHash).Scan(&vaultSize); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: verified Vault object is not registered", domain.ErrConflict)
		}
		return ExecutionEnvironmentManifest{}, fmt.Errorf("read captured Vault object: %w", err)
	}
	if vaultSize != sizeBytes {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: captured Vault object size mismatch", domain.ErrConflict)
	}

	updatedManifest := item.Manifest
	updatedManifest.Assets = append([]executionenv.AssetRequirement(nil), item.Manifest.Assets...)
	capturedSize := sizeBytes
	updatedManifest.Assets[assetIndex].CapturePolicy = executionenv.CaptureContentBound
	updatedManifest.Assets[assetIndex].ContentHash = contentHash
	updatedManifest.Assets[assetIndex].SizeBytes = &capturedSize

	canonical, err := executionenv.CanonicalBytes(updatedManifest)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: captured execution environment manifest: %v", domain.ErrInvalidInput, err)
	}
	normalized, err := executionenv.DecodeCanonical(canonical)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("decode captured execution environment manifest: %w", err)
	}
	manifestHash, err := executionenv.ContentHash(normalized)
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("hash captured execution environment manifest: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE execution_environment_manifests
		SET manifest_json = ?, content_sha256 = ?
		WHERE environment_manifest_id = ? AND content_sha256 = ?`,
		string(canonical), manifestHash, item.ID, item.ContentSHA256,
	)
	if err != nil {
		return ExecutionEnvironmentManifest{}, mapExecutionEnvironmentWriteError("capture execution environment asset", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("capture execution environment asset rows affected: %w", err)
	}
	if rows != 1 {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("%w: execution environment changed during capture", domain.ErrConflict)
	}
	if err := tx.Commit(); err != nil {
		return ExecutionEnvironmentManifest{}, fmt.Errorf("commit execution environment asset capture: %w", err)
	}
	return s.GetExecutionEnvironmentManifest(ctx, item.ID)
}
