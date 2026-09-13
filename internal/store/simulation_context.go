package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

// SessionTypeForActionExecution resolves the authoritative Session type for an
// already-created ActionExecution. It is intentionally narrow: F-024 uses it
// only to decide whether a Cue action is allowed to reach the physical
// capability executor or must remain inside the simulation boundary.
func (s *Store) SessionTypeForActionExecution(ctx context.Context, actionExecutionID string) (domain.SessionType, error) {
	if strings.TrimSpace(actionExecutionID) == "" {
		return "", fmt.Errorf("%w: action execution ID is required", domain.ErrInvalidInput)
	}

	var raw string
	err := s.db.QueryRowContext(ctx, `
		SELECT sessions.session_type
		FROM action_executions
		JOIN cue_executions
		  ON cue_executions.cue_execution_id = action_executions.cue_execution_id
		JOIN sessions
		  ON sessions.session_id = cue_executions.session_id
		WHERE action_executions.action_execution_id = ?`, strings.TrimSpace(actionExecutionID)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("resolve action execution session type: %w", err)
	}

	sessionType := domain.SessionType(raw)
	switch sessionType {
	case domain.SessionSimulation, domain.SessionRehearsal, domain.SessionShow:
		return sessionType, nil
	default:
		return "", fmt.Errorf("unknown session type %q for action execution", raw)
	}
}
