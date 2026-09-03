package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

// DeleteVaultObjectIfUnreferenced removes only metadata for a Vault object that
// is not referenced by durable media/software rows or F-025 environment JSON.
// It is intentionally conservative: any detected reference leaves the object
// untouched so rollback cleanup can never delete content another workflow uses.
func (s *Store) DeleteVaultObjectIfUnreferenced(ctx context.Context, contentHash string) (VaultObject, bool, error) {
	contentHash = strings.ToLower(strings.TrimSpace(contentHash))
	object, err := s.GetVaultObject(ctx, contentHash)
	if errors.Is(err, domain.ErrNotFound) {
		return VaultObject{}, false, nil
	}
	if err != nil {
		return VaultObject{}, false, err
	}

	var referenced int
	pattern := "%" + contentHash + "%"
	err = s.db.QueryRowContext(ctx, `
		SELECT CASE WHEN
			EXISTS (SELECT 1 FROM media_content_versions WHERE content_hash = ?)
			OR EXISTS (SELECT 1 FROM software_packages WHERE content_hash = ?)
			OR EXISTS (SELECT 1 FROM execution_environment_manifests WHERE lower(manifest_json) LIKE ?)
			OR EXISTS (SELECT 1 FROM execution_environment_snapshots WHERE lower(snapshot_json) LIKE ?)
		THEN 1 ELSE 0 END`, contentHash, contentHash, pattern, pattern).Scan(&referenced)
	if err != nil {
		return VaultObject{}, false, fmt.Errorf("check Vault object references: %w", err)
	}
	if referenced != 0 {
		return object, false, nil
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM vault_objects WHERE content_hash = ?`, contentHash)
	if err != nil {
		// A concurrent foreign-key reference wins over cleanup. Retaining bytes is
		// the safe outcome, so treat that race as a non-removal.
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			return object, false, nil
		}
		return VaultObject{}, false, fmt.Errorf("delete unreferenced Vault object: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return VaultObject{}, false, fmt.Errorf("delete unreferenced Vault object rows affected: %w", err)
	}
	return object, rows == 1, nil
}
