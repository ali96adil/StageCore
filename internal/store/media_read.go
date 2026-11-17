package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) GetMediaContentVersion(ctx context.Context, contentVersionID string) (MediaContentVersion, error) {
	var version MediaContentVersion
	var createdUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT content_version_id, media_asset_id, content_hash, original_filename, size_bytes, created_at_us
		FROM media_content_versions
		WHERE content_version_id = ?`, strings.TrimSpace(contentVersionID)).Scan(
		&version.ID, &version.MediaAssetID, &version.ContentHash, &version.OriginalFilename, &version.SizeBytes, &createdUS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaContentVersion{}, domain.ErrNotFound
	}
	if err != nil {
		return MediaContentVersion{}, fmt.Errorf("get media content version: %w", err)
	}
	version.CreatedAt = clock.FromUnixMicros(createdUS)
	return version, nil
}

// CountUnreferencedProjectMediaContentVersions counts immutable Project file
// versions absent from the selected Runtime Snapshot without loading file rows
// or Vault data into the Capsule preview process.
func (s *Store) CountUnreferencedProjectMediaContentVersions(ctx context.Context, projectID string, referencedVersionIDs []string) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return 0, fmt.Errorf("%w: project is required", domain.ErrInvalidInput)
	}
	encodedReferences, err := json.Marshal(referencedVersionIDs)
	if err != nil {
		return 0, fmt.Errorf("encode Snapshot media references: %w", err)
	}
	var count int64
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM media_content_versions v
		JOIN media_assets a ON a.media_asset_id = v.media_asset_id
		WHERE a.project_id = ? AND v.content_version_id NOT IN (
			SELECT value FROM json_each(?)
		)`, projectID, string(encodedReferences)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count unreferenced Project media content versions: %w", err)
	}
	return count, nil
}

// ProjectHasMediaContentHash reports whether a hash is registered as a media
// version in the project. It is used only for the bounded legacy Companion
// Vault path when a Machine Role predates Runtime Snapshot requirements.
func (s *Store) ProjectHasMediaContentHash(ctx context.Context, projectID, contentHash string) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	contentHash = strings.ToLower(strings.TrimSpace(contentHash))
	if projectID == "" || !validSHA256Hex(contentHash) {
		return false, fmt.Errorf("%w: project and media content hash are required", domain.ErrInvalidInput)
	}
	var found bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM media_assets ma
			JOIN media_content_versions mcv ON mcv.media_asset_id = ma.media_asset_id
			WHERE ma.project_id = ? AND mcv.content_hash = ?
		)`, projectID, contentHash).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check project media content hash: %w", err)
	}
	return found, nil
}
