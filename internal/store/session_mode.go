package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) ActiveOperationalSessionType(ctx context.Context) (domain.SessionType, error) {
	var sessionType string
	err := s.db.QueryRowContext(ctx, `
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
