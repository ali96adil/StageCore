package store

import (
	"context"
	"database/sql"
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
