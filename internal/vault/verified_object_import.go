package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/ali96adil/StageCore/internal/store"
)

// ImportVerifiedObject accepts only bytes that exactly match a caller-provided
// SHA-256 identity and size. Mismatched or interrupted input remains staging
// data only and is removed; it is never promoted into the immutable Vault.
func (v *Vault) ImportVerifiedObject(
	ctx context.Context,
	expectedContentHash string,
	expectedSizeBytes int64,
	r io.Reader,
) (store.VaultObject, error) {
	if v == nil || v.store == nil {
		return store.VaultObject{}, fmt.Errorf("Vault is unavailable")
	}
	if r == nil {
		return store.VaultObject{}, fmt.Errorf("verified import reader is required")
	}
	expectedContentHash, err := validateContentHash(expectedContentHash)
	if err != nil {
		return store.VaultObject{}, err
	}
	if expectedSizeBytes < 0 {
		return store.VaultObject{}, fmt.Errorf("verified object size cannot be negative")
	}

	staged, err := os.CreateTemp(v.stagingRoot, "verified-object-*.part")
	if err != nil {
		return store.VaultObject{}, fmt.Errorf("create verified Vault staging file: %w", err)
	}
	stagedPath := staged.Name()
	removeStaging := true
	defer func() {
		_ = staged.Close()
		if removeStaging {
			_ = os.Remove(stagedPath)
		}
	}()

	// Read at most one byte beyond the declared size so an oversized sender
	// fails without allowing an unbounded stream to consume staging capacity.
	limit := expectedSizeBytes
	if expectedSizeBytes < math.MaxInt64 {
		limit++
	}
	hasher := sha256.New()
	size, err := v.streamToStaging(staged, hasher, io.LimitReader(r, limit))
	if err != nil {
		return store.VaultObject{}, fmt.Errorf("stream verified object into Vault staging: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return store.VaultObject{}, fmt.Errorf("sync verified Vault object: %w", err)
	}
	if err := staged.Close(); err != nil {
		return store.VaultObject{}, fmt.Errorf("close verified Vault object: %w", err)
	}

	actualContentHash := hex.EncodeToString(hasher.Sum(nil))
	if size != expectedSizeBytes || actualContentHash != expectedContentHash {
		return store.VaultObject{}, fmt.Errorf(
			"verified Vault object identity mismatch: got sha256=%s size=%d",
			actualContentHash, size,
		)
	}

	relativePath := objectRelativePath(expectedContentHash)
	objectPath := filepath.Join(v.root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(objectPath), directoryMode); err != nil {
		return store.VaultObject{}, fmt.Errorf("create verified Vault object directory: %w", err)
	}

	if info, statErr := os.Stat(objectPath); statErr == nil {
		if !info.Mode().IsRegular() || info.Size() != expectedSizeBytes {
			return store.VaultObject{}, fmt.Errorf("existing verified Vault object conflicts with content identity")
		}
	} else if !os.IsNotExist(statErr) {
		return store.VaultObject{}, fmt.Errorf("inspect verified Vault object: %w", statErr)
	} else if err := os.Link(stagedPath, objectPath); err != nil {
		if info, retryErr := os.Stat(objectPath); retryErr != nil ||
			!info.Mode().IsRegular() || info.Size() != expectedSizeBytes {
			return store.VaultObject{}, fmt.Errorf("atomically promote verified Vault object: %w", err)
		}
	}

	if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
		return store.VaultObject{}, fmt.Errorf("remove verified staging link: %w", err)
	}
	removeStaging = false

	object, err := v.store.RegisterVaultObject(ctx, store.RegisterVaultObjectParams{
		ContentHash:  expectedContentHash,
		SizeBytes:    expectedSizeBytes,
		RelativePath: filepath.ToSlash(relativePath),
	})
	if err != nil {
		return store.VaultObject{}, fmt.Errorf("register verified Vault object: %w", err)
	}
	return object, nil
}
