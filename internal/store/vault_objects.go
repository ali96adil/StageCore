package store

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

// RegisterVaultObject records immutable content-addressed bytes that have
// already been verified and promoted into the Vault. It deliberately does not
// create media/software semantics; callers attach those separately after the
// shared object identity is durable.
func (s *Store) RegisterVaultObject(ctx context.Context, contentHash string, sizeBytes int64, relativePath string) (VaultObject, error) {
	contentHash = strings.ToLower(strings.TrimSpace(contentHash))
	relativePath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(relativePath)))
	decoded, err := hex.DecodeString(contentHash)
	if err != nil || len(decoded) != 32 {
		return VaultObject{}, fmt.Errorf("%w: Vault object SHA-256 is invalid", domain.ErrInvalidInput)
	}
	if sizeBytes < 0 {
		return VaultObject{}, fmt.Errorf("%w: Vault object size cannot be negative", domain.ErrInvalidInput)
	}
	if relativePath == "." || filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, "../") {
		return VaultObject{}, fmt.Errorf("%w: Vault object path must be a safe relative path", domain.ErrInvalidInput)
	}
	now := s.clock.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO vault_objects (content_hash, size_bytes, relative_path, created_at_us)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(content_hash) DO NOTHING`, contentHash, sizeBytes, relativePath, clock.UnixMicros(now)); err != nil {
		return VaultObject{}, fmt.Errorf("register Vault object: %w", err)
	}
	object, err := s.GetVaultObject(ctx, contentHash)
	if err != nil {
		return VaultObject{}, err
	}
	if object.SizeBytes != sizeBytes || object.RelativePath != relativePath {
		return VaultObject{}, fmt.Errorf("%w: existing Vault object metadata conflicts with verified content", domain.ErrConflict)
	}
	return object, nil
}
