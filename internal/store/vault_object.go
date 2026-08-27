package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

type RegisterVaultObjectParams struct {
	ContentHash  string
	SizeBytes    int64
	RelativePath string
}

func (s *Store) RegisterVaultObject(ctx context.Context, p RegisterVaultObjectParams) (VaultObject, error) {
	p.ContentHash = strings.ToLower(strings.TrimSpace(p.ContentHash))
	p.RelativePath = strings.TrimSpace(p.RelativePath)
	if len(p.ContentHash) != 64 || p.RelativePath == "" {
		return VaultObject{}, fmt.Errorf("%w: SHA-256 content hash and relative path are required", domain.ErrInvalidInput)
	}
	if p.SizeBytes < 0 {
		return VaultObject{}, fmt.Errorf("%w: size cannot be negative", domain.ErrInvalidInput)
	}
	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO vault_objects (content_hash, size_bytes, relative_path, created_at_us)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(content_hash) DO NOTHING`, p.ContentHash, p.SizeBytes, p.RelativePath, nowUS); err != nil {
		return VaultObject{}, fmt.Errorf("register Vault object: %w", err)
	}
	object, err := s.GetVaultObject(ctx, p.ContentHash)
	if err != nil {
		return VaultObject{}, err
	}
	if object.SizeBytes != p.SizeBytes || object.RelativePath != p.RelativePath {
		return VaultObject{}, fmt.Errorf("%w: existing Vault identity metadata conflicts with verified content", domain.ErrInvalidInput)
	}
	return object, nil
}
