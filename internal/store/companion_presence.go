package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

// MarkCompanionRuntimeDisconnected records loss of the authoritative runtime
// transport immediately. The caller is responsible for fencing stale
// connection generations before invoking this method.
func (s *Store) MarkCompanionRuntimeDisconnected(ctx context.Context, companionID string) error {
	companionID = strings.TrimSpace(companionID)
	if companionID == "" {
		return fmt.Errorf("%w: companion id is required", domain.ErrInvalidInput)
	}
	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Companion runtime disconnect: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE companions
		SET readiness = 'OFFLINE', updated_at_us = ?
		WHERE companion_id = ?`, nowUS, companionID)
	if err != nil {
		return fmt.Errorf("mark Companion runtime offline: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("Companion runtime offline rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE role_assignments
		SET state = 'OFFLINE', last_evaluated_at_us = ?
		WHERE companion_id = ? AND state <> 'RELEASED'`, nowUS, companionID); err != nil {
		return fmt.Errorf("mark Companion role runtime offline: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Companion runtime disconnect: %w", err)
	}
	return nil
}
