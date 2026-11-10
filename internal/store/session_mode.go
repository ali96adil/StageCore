package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) ActiveOperationalSessionType(ctx context.Context) (domain.SessionType, error) {
	return activeOperationalSessionType(ctx, s.db)
}

// ActiveOperationalSessionTypeInTransaction reads the operational state at
// the same database boundary as a write that must be restricted to idle mode.
func ActiveOperationalSessionTypeInTransaction(ctx context.Context, tx *sql.Tx) (domain.SessionType, error) {
	return activeOperationalSessionType(ctx, tx)
}

func activeOperationalSessionType(ctx context.Context, q queryer) (domain.SessionType, error) {
	var sessionType string
	err := q.QueryRowContext(ctx, `
		SELECT session_type
		FROM sessions
		WHERE status = 'ACTIVE'
		ORDER BY CASE session_type WHEN 'SHOW' THEN 0 WHEN 'REHEARSAL' THEN 1 ELSE 2 END,
		         started_at_us DESC
		LIMIT 1`).Scan(&sessionType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read active operational session type: %w", err)
	}
	return domain.SessionType(sessionType), nil
}

func (s *Store) EndSession(ctx context.Context, sessionID string, status domain.SessionStatus) error {
	switch status {
	case domain.SessionCompleted:
		return s.EndSessionLifecycle(ctx, sessionID, domain.SessionLifecycleCompleted, "")
	case domain.SessionAborted:
		return s.EndSessionLifecycle(ctx, sessionID, domain.SessionLifecycleAborted, "")
	default:
		return fmt.Errorf("%w: terminal session status required", domain.ErrInvalidInput)
	}
}

// CompleteOperationalStop commits the terminal session, its event, and the
// replayable command result together after the runtime confirms output safety.
func (s *Store) CompleteOperationalStop(ctx context.Context, sessionID string, event contracts.EventEnvelope, result contracts.CommandResult) error {
	return s.completeOperationalExit(ctx, sessionID, domain.SessionCompleted, domain.SessionLifecycleCompleted, "", event, result)
}

// CompleteForcedOperationalExit preserves the aborted lifecycle and its audit
// reason in the same transaction as the replayable result.
func (s *Store) CompleteForcedOperationalExit(ctx context.Context, sessionID, reason string, event contracts.EventEnvelope, result contracts.CommandResult) error {
	return s.completeOperationalExit(ctx, sessionID, domain.SessionAborted, domain.SessionLifecycleAborted, reason, event, result)
}

func (s *Store) completeOperationalExit(ctx context.Context, sessionID string, status domain.SessionStatus, lifecycle domain.SessionLifecycleState, reason string, event contracts.EventEnvelope, result contracts.CommandResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	updated, err := tx.ExecContext(ctx, endSessionLifecycleSQL, status,
		lifecycle, reason, clock.UnixMicros(s.clock.Now()), sessionID)
	if err != nil {
		return err
	}
	if err := requireOneRow(updated); err != nil {
		return err
	}
	if _, err := s.appendEvent(ctx, tx, &sessionID, event); err != nil {
		return err
	}
	if err := s.finishCommand(ctx, tx, result.CommandID, result); err != nil {
		return err
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// CompleteBlackout records the journal and replay result together.
// Clearing the latch also belongs to this transaction. An applied latch is
// persisted separately before any output dispatch.
func (s *Store) CompleteBlackout(ctx context.Context, sessionID *string, clearActor string, event contracts.EventEnvelope, result contracts.CommandResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if clearActor != "" {
		if sessionID == nil {
			return domain.ErrInvalidInput
		}
		cleared, err := tx.ExecContext(ctx, setSessionBlackoutSQL, 0, clearActor, clock.UnixMicros(s.clock.Now().UTC()), *sessionID)
		if err != nil {
			return err
		}
		if err := requireOneRow(cleared); err != nil {
			return err
		}
		if err := capability.CheckWriteAuthorization(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := s.appendEvent(ctx, tx, sessionID, event); err != nil {
		return err
	}
	if err := s.finishCommand(ctx, tx, result.CommandID, result); err != nil {
		return err
	}
	return tx.Commit()
}
