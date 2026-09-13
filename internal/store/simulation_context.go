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
// to decide whether a persisted Cue action may reach the physical capability
// executor or must remain inside the simulation boundary.
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
	return validateSimulationSessionType(raw, "action execution")
}

// SessionTypeForRuntimeSnapshotExecution resolves the active Session type for
// an execution that is scoped to a Runtime Snapshot but does not create an
// ActionExecution row first (for example a direct Route output action).
//
// The lookup is deliberately fail-closed. Exactly one ACTIVE Session must own
// the supplied immutable Runtime Snapshot. Zero or multiple matches are not
// guessed because that ambiguity could otherwise leak a simulated Route to a
// physical transport.
func (s *Store) SessionTypeForRuntimeSnapshotExecution(ctx context.Context, runtimeSnapshotID string) (domain.SessionType, error) {
	runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
	if runtimeSnapshotID == "" {
		return "", fmt.Errorf("%w: runtime snapshot ID is required", domain.ErrInvalidInput)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT session_type
		FROM sessions
		WHERE runtime_snapshot_id = ?
		  AND status = 'ACTIVE'
		ORDER BY started_at_us, session_id`, runtimeSnapshotID)
	if err != nil {
		return "", fmt.Errorf("resolve runtime snapshot session type: %w", err)
	}
	defer rows.Close()

	matches := make([]string, 0, 2)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return "", fmt.Errorf("scan runtime snapshot session type: %w", err)
		}
		matches = append(matches, raw)
		if len(matches) > 1 {
			return "", fmt.Errorf("%w: runtime snapshot has multiple active sessions", domain.ErrConflict)
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("resolve runtime snapshot session type rows: %w", err)
	}
	if len(matches) == 0 {
		return "", domain.ErrNotFound
	}
	return validateSimulationSessionType(matches[0], "runtime snapshot")
}

func validateSimulationSessionType(raw, source string) (domain.SessionType, error) {
	sessionType := domain.SessionType(raw)
	switch sessionType {
	case domain.SessionSimulation, domain.SessionRehearsal, domain.SessionShow:
		return sessionType, nil
	default:
		return "", fmt.Errorf("unknown session type %q for %s", raw, source)
	}
}
