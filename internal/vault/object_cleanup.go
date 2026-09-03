package vault

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RemoveObjectIfUnreferenced is rollback cleanup for additive staged imports.
// Store metadata is removed first only when no durable reference exists. The
// immutable file is then removed; a leftover file after an OS error is an
// unregistered harmless cache and can be safely reused by a future import.
func (v *Vault) RemoveObjectIfUnreferenced(ctx context.Context, contentHash string) (bool, error) {
	if v == nil || v.store == nil {
		return false, fmt.Errorf("Vault is unavailable")
	}
	object, removed, err := v.store.DeleteVaultObjectIfUnreferenced(ctx, contentHash)
	if err != nil || !removed {
		return removed, err
	}
	path := filepath.Join(v.root, filepath.FromSlash(object.RelativePath))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return true, fmt.Errorf("remove unreferenced Vault object file: %w", err)
	}
	return true, nil
}
