package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ali96adil/StageCore/internal/store"
)

// ImportObject streams arbitrary immutable content through Vault staging,
// verifies its SHA-256 identity and promotes it atomically before metadata is
// exposed. It is used by non-media repositories such as local software.
func (v *Vault) ImportObject(ctx context.Context, r io.Reader) (store.VaultObject, error) {
	if v == nil || v.store == nil {
		return store.VaultObject{}, fmt.Errorf("Vault is unavailable")
	}
	if r == nil {
		return store.VaultObject{}, fmt.Errorf("import reader is required")
	}

	staged, err := os.CreateTemp(v.stagingRoot, "object-*.part")
	if err != nil {
		return store.VaultObject{}, fmt.Errorf("create Vault staging file: %w", err)
	}
	stagedPath := staged.Name()
	removeStaging := true
	defer func() {
		_ = staged.Close()
		if removeStaging {
			_ = os.Remove(stagedPath)
		}
	}()

	hasher := sha256.New()
	size, err := v.streamToStaging(staged, hasher, r)
	if err != nil {
		return store.VaultObject{}, fmt.Errorf("stream object into Vault staging: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return store.VaultObject{}, fmt.Errorf("sync staged Vault object: %w", err)
	}
	if err := staged.Close(); err != nil {
		return store.VaultObject{}, fmt.Errorf("close staged Vault object: %w", err)
	}

	contentHash := hex.EncodeToString(hasher.Sum(nil))
	relativePath := objectRelativePath(contentHash)
	objectPath := filepath.Join(v.root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(objectPath), directoryMode); err != nil {
		return store.VaultObject{}, fmt.Errorf("create Vault object directory: %w", err)
	}

	if info, statErr := os.Stat(objectPath); statErr == nil {
		if !info.Mode().IsRegular() || info.Size() != size {
			return store.VaultObject{}, fmt.Errorf("existing Vault object conflicts with verified content identity")
		}
	} else if !os.IsNotExist(statErr) {
		return store.VaultObject{}, fmt.Errorf("inspect Vault object: %w", statErr)
	} else if err := os.Link(stagedPath, objectPath); err != nil {
		if info, retryErr := os.Stat(objectPath); retryErr != nil || !info.Mode().IsRegular() || info.Size() != size {
			return store.VaultObject{}, fmt.Errorf("atomically promote Vault object: %w", err)
		}
	}

	if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
		return store.VaultObject{}, fmt.Errorf("remove promoted staging link: %w", err)
	}
	removeStaging = false

	object, err := v.store.RegisterVaultObject(ctx, store.RegisterVaultObjectParams{
		ContentHash: contentHash,
		SizeBytes: size,
		RelativePath: filepath.ToSlash(relativePath),
	})
	if err != nil {
		return store.VaultObject{}, fmt.Errorf("register imported Vault object: %w", err)
	}
	return object, nil
}
