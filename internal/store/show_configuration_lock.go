package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

const (
	ShowConfigurationLockVersion1 = 1
	showConfigurationLockReason   = "ACTIVE_SHOW_SESSION"
	showConfigurationLockScope    = "PROJECT_CONFIGURATION"
	showConfigurationUnlockAction = "SHOW_EXIT"
	showConfigurationDBMarker     = "SHOW_CONFIGURATION_LOCKED"
)

type ShowConfigurationLockState struct {
	Version            int        `json:"version"`
	Locked             bool       `json:"locked"`
	ProjectID          string     `json:"project_id"`
	ActiveShowSessionID *string    `json:"active_show_session_id"`
	RuntimeSnapshotID  *string    `json:"runtime_snapshot_id"`
	ShowStartedAt      *time.Time `json:"show_started_at"`
	Reason             string     `json:"reason"`
	Scope              string     `json:"scope"`
	UnlockAction       string     `json:"unlock_action"`
	OverrideSupported  bool       `json:"override_supported"`
}

func (s *Store) ShowConfigurationLockState(ctx context.Context, projectID string) (ShowConfigurationLockState, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ShowConfigurationLockState{}, fmt.Errorf("%w: project is required", domain.ErrInvalidInput)
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return ShowConfigurationLockState{}, err
	}
	state := ShowConfigurationLockState{
		Version: ShowConfigurationLockVersion1,
		Locked: false,
		ProjectID: projectID,
		Scope: showConfigurationLockScope,
		UnlockAction: showConfigurationUnlockAction,
		OverrideSupported: false,
	}
	var sessionID, snapshotID string
	var startedAtUS int64
	err := s.db.QueryRowContext(ctx, `
		SELECT session_id, runtime_snapshot_id, started_at_us
		FROM sessions
		WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'
		ORDER BY started_at_us DESC, session_id DESC
		LIMIT 1`, projectID).Scan(&sessionID, &snapshotID, &startedAtUS)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return ShowConfigurationLockState{}, fmt.Errorf("read show configuration lock: %w", err)
	}
	startedAt := clock.FromUnixMicros(startedAtUS)
	state.Locked = true
	state.ActiveShowSessionID = &sessionID
	state.RuntimeSnapshotID = &snapshotID
	state.ShowStartedAt = &startedAt
	state.Reason = showConfigurationLockReason
	return state, nil
}

func (s *Store) RequireProjectConfigurationMutable(ctx context.Context, projectID string) error {
	state, err := s.ShowConfigurationLockState(ctx, projectID)
	if err != nil {
		return err
	}
	if state.Locked {
		return fmt.Errorf("%w: project %s is locked by SHOW session %s", domain.ErrShowConfigurationLocked, state.ProjectID, dereferenceString(state.ActiveShowSessionID))
	}
	return nil
}

func IsShowConfigurationLockedError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, domain.ErrShowConfigurationLocked) || strings.Contains(err.Error(), showConfigurationDBMarker)
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
