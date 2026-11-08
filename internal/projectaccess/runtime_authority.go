package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// RuntimeIntent is a reviewed future authority boundary, NOT an action to
// dispatch. The live Hub still uses its existing global runtime.control
// checks; this helper is intentionally unwired from every GO/STOP route.
type RuntimeIntent string

const (
	RuntimeGO             RuntimeIntent = "GO"
	RuntimeJump           RuntimeIntent = "JUMP"
	RuntimeStopCue        RuntimeIntent = "STOP_CUE"
	RuntimeStopSession    RuntimeIntent = "STOP_SESSION"
	RuntimeBlackoutEngage RuntimeIntent = "BLACKOUT_ENGAGE"
	RuntimeBlackoutClear  RuntimeIntent = "BLACKOUT_CLEAR"
	RuntimeForceExit      RuntimeIntent = "FORCE_EXIT_WITHOUT_BLACKOUT"
)

// CheckRuntimeIntent checks a current user + exact active Session identity
// against its immutable Snapshot and requested Project. An optional empty
// SessionID is legal ONLY for a sessionless Project blackout, which is never
// allowed to act as GO. Never use this as a standalone TOCTOU-prone
// dispatch-time authorization fence: production needs atomic rechecking
// while the actual command is committed/dispatched.
func (s *Service) CheckRuntimeIntent(
	ctx context.Context, userID, projectID, sessionID string, intent RuntimeIntent,
) error {
	if s == nil || s.db == nil || strings.TrimSpace(userID) == "" ||
		strings.TrimSpace(projectID) == "" {
		return ErrDenied
	}
	userID, projectID, sessionID = strings.TrimSpace(userID), strings.TrimSpace(projectID), strings.TrimSpace(sessionID)
	isBlackout := intent == RuntimeBlackoutEngage || intent == RuntimeBlackoutClear
	if !isBlackout && sessionID == "" {
		return ErrDenied
	}
	// Unknown intent always fails even for Owner. Do not accept a caller-
	// supplied permission name that was not reviewed in this intent enum.
	switch intent {
	case RuntimeGO, RuntimeJump, RuntimeStopCue, RuntimeStopSession,
		RuntimeBlackoutEngage, RuntimeBlackoutClear, RuntimeForceExit:
	default:
		return ErrDenied
	}

	sessionType := ""
	if sessionID != "" {
		// A Session ID can only be used in the Project whose published
		// Snapshot created it. This prevents a legitimate Project A
		// user from sending a Project B Session ID on an A-scoped URL.
		var status, lifecycle string
		err := s.db.QueryRowContext(ctx, `
			SELECT se.session_type, se.status, se.lifecycle_state
			FROM sessions se
			JOIN runtime_snapshots rs ON rs.runtime_snapshot_id=se.runtime_snapshot_id
			WHERE se.session_id=? AND se.project_id=? AND rs.project_id=se.project_id
		`, sessionID, projectID).Scan(&sessionType, &status, &lifecycle)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDenied
		}
		if err != nil {
			return fmt.Errorf("qualify active runtime Session: %w", err)
		}
		if status != "ACTIVE" || lifecycle != "ACTIVE" {
			return ErrDenied
		}
		if sessionType != "SHOW" && sessionType != "REHEARSAL" {
			return ErrDenied // never reuse live SHOW authority in SIMULATION
		}
	}

	var needed Permission
	switch intent {
	case RuntimeGO, RuntimeJump:
		needed = RehearsalControl
		if sessionType == "SHOW" { needed = ShowGO }
	case RuntimeStopCue:
		needed = RehearsalControl
		if sessionType == "SHOW" { needed = ShowStop }
	case RuntimeStopSession:
		needed = RehearsalControl
		if sessionType == "SHOW" { needed = ShowEnterExit }
	case RuntimeBlackoutEngage:
		// Safety action is independently granted even after SHOW GO is
		// revoked. Sessionless emergency only applies to ProjectID.
		needed = ShowBlackoutEngage
	case RuntimeBlackoutClear, RuntimeForceExit:
		// Clearing an engaged emergency blackout or skipping stop safety
		// is much riskier than engaging one: keep strictly Owner-only
		// until separately reviewed recovery/operator procedures exist.
		needed = ProjectView
	}
	if err := s.Check(ctx, userID, projectID, needed); err != nil {
		return err
	}
	if intent == RuntimeBlackoutClear || intent == RuntimeForceExit {
		var role string
		var enabled int
		err := s.db.QueryRowContext(ctx,
			`SELECT role, enabled FROM local_users WHERE user_id=?`, userID).Scan(&role,&enabled)
		if errors.Is(err, sql.ErrNoRows) { return ErrDenied }
		if err != nil { return fmt.Errorf("check privileged runtime recovery role: %w", err) }
		if role != "OWNER" || enabled != 1 { return ErrDenied }
	}
	return nil
}
