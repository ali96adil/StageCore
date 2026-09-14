package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

// SessionForActionExecution resolves the authoritative Session for an already-
// created ActionExecution. F-024 uses the persisted ancestry instead of caller
// supplied metadata so a simulation request cannot select another Session.
func (s *Store) SessionForActionExecution(ctx context.Context, actionExecutionID string) (domain.Session, error) {
	actionExecutionID = strings.TrimSpace(actionExecutionID)
	if actionExecutionID == "" {
		return domain.Session{}, fmt.Errorf("%w: action execution ID is required", domain.ErrInvalidInput)
	}

	var sessionID string
	err := s.db.QueryRowContext(ctx, `
		SELECT sessions.session_id
		FROM action_executions
		JOIN cue_executions
		  ON cue_executions.cue_execution_id = action_executions.cue_execution_id
		JOIN sessions
		  ON sessions.session_id = cue_executions.session_id
		WHERE action_executions.action_execution_id = ?`, actionExecutionID).Scan(&sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, domain.ErrNotFound
		}
		return domain.Session{}, fmt.Errorf("resolve action execution session: %w", err)
	}
	return s.GetSession(ctx, sessionID)
}

// SessionForRuntimeSnapshotExecution resolves the single ACTIVE Session that
// owns a Runtime Snapshot for execution paths that do not persist an
// ActionExecution first, such as direct Route outputs.
//
// The lookup is deliberately fail-closed. Zero or multiple active owners are
// never guessed because ambiguity could leak simulated work to a real output.
func (s *Store) SessionForRuntimeSnapshotExecution(ctx context.Context, runtimeSnapshotID string) (domain.Session, error) {
	runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
	if runtimeSnapshotID == "" {
		return domain.Session{}, fmt.Errorf("%w: runtime snapshot ID is required", domain.ErrInvalidInput)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id
		FROM sessions
		WHERE runtime_snapshot_id = ?
		  AND status = 'ACTIVE'
		ORDER BY started_at_us, session_id`, runtimeSnapshotID)
	if err != nil {
		return domain.Session{}, fmt.Errorf("resolve runtime snapshot session: %w", err)
	}
	defer rows.Close()

	matches := make([]string, 0, 2)
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return domain.Session{}, fmt.Errorf("scan runtime snapshot session: %w", err)
		}
		matches = append(matches, sessionID)
		if len(matches) > 1 {
			return domain.Session{}, fmt.Errorf("%w: runtime snapshot has multiple active sessions", domain.ErrConflict)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.Session{}, fmt.Errorf("resolve runtime snapshot session rows: %w", err)
	}
	if len(matches) == 0 {
		return domain.Session{}, domain.ErrNotFound
	}
	return s.GetSession(ctx, matches[0])
}

// SessionTypeForActionExecution is retained as a narrow compatibility helper.
func (s *Store) SessionTypeForActionExecution(ctx context.Context, actionExecutionID string) (domain.SessionType, error) {
	session, err := s.SessionForActionExecution(ctx, actionExecutionID)
	if err != nil {
		return "", err
	}
	return validateSimulationSessionType(string(session.Type), "action execution")
}

// SessionTypeForRuntimeSnapshotExecution is retained as a narrow compatibility
// helper for callers that only need mode information.
func (s *Store) SessionTypeForRuntimeSnapshotExecution(ctx context.Context, runtimeSnapshotID string) (domain.SessionType, error) {
	session, err := s.SessionForRuntimeSnapshotExecution(ctx, runtimeSnapshotID)
	if err != nil {
		return "", err
	}
	return validateSimulationSessionType(string(session.Type), "runtime snapshot")
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
