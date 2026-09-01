package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

// DeleteExtensionInstallation removes the installation-owned durable state in
// foreign-key-safe order. Package/library metadata is intentionally retained so
// the same immutable package can be installed again later.
func (s *Store) DeleteExtensionInstallation(ctx context.Context, installationID string) error {
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return fmt.Errorf("%w: installation ID is required", domain.ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin extension uninstall transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM extension_permission_reviews WHERE installation_id = ?`, installationID); err != nil {
		return fmt.Errorf("delete extension permission reviews: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM extension_runtime_lifecycle WHERE installation_id = ?`, installationID); err != nil {
		return fmt.Errorf("delete extension runtime lifecycle: %w", err)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM extension_installations WHERE installation_id = ?`, installationID)
	if err != nil {
		return fmt.Errorf("delete extension installation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("extension uninstall rows affected: %w", err)
	}
	if rows != 1 {
		return domain.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit extension uninstall transaction: %w", err)
	}
	return nil
}
