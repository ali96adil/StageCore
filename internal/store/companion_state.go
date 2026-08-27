package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

func (s *Store) GetRoleAssignment(ctx context.Context, assignmentID string) (domain.RoleAssignment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT role_assignment_id, machine_role_id, companion_id, state, assigned_at_us, released_at_us, last_evaluated_at_us
		FROM role_assignments WHERE role_assignment_id = ?`, assignmentID)
	return scanRoleAssignment(row)
}

func (s *Store) GetActiveRoleAssignmentForCompanion(ctx context.Context, companionID string) (domain.RoleAssignment, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT role_assignment_id, machine_role_id, companion_id, state, assigned_at_us, released_at_us, last_evaluated_at_us
		FROM role_assignments WHERE companion_id = ? AND state <> 'RELEASED'`, companionID)
	return scanRoleAssignment(row)
}

func (s *Store) SetMachineRoleRuntimeRequirement(ctx context.Context, machineRoleID, runtimeSnapshotID, configHash string) error {
	machineRoleID = strings.TrimSpace(machineRoleID)
	runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
	if machineRoleID == "" || runtimeSnapshotID == "" {
		return fmt.Errorf("%w: machine role and Runtime Snapshot are required", domain.ErrInvalidInput)
	}
	role, err := s.GetMachineRole(ctx, machineRoleID)
	if err != nil {
		return err
	}
	runtimeSnapshot, err := s.GetRuntimeSnapshot(ctx, runtimeSnapshotID)
	if err != nil {
		return err
	}
	if runtimeSnapshot.ProjectID != role.ProjectID || runtimeSnapshot.Status != domain.SnapshotPublished {
		return fmt.Errorf("%w: published Runtime Snapshot must belong to the Machine Role Project", domain.ErrConflict)
	}
	nowUS := clock.UnixMicros(s.clock.Now())
	result, err := s.db.ExecContext(ctx, `
		UPDATE machine_roles
		SET required_runtime_snapshot_id = ?, required_config_hash = ?, updated_at_us = ?
		WHERE machine_role_id = ?`, runtimeSnapshotID, strings.TrimSpace(configHash), nowUS, machineRoleID)
	if err != nil {
		return fmt.Errorf("set machine role Runtime Snapshot requirement: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("machine role Runtime Snapshot rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) SetRoleAssignmentState(ctx context.Context, assignmentID string, state domain.RoleAssignmentState) error {
	if !validActiveRoleAssignmentState(state) {
		return fmt.Errorf("%w: invalid active role assignment state %q", domain.ErrInvalidInput, state)
	}
	nowUS := clock.UnixMicros(s.clock.Now())
	result, err := s.db.ExecContext(ctx, `
		UPDATE role_assignments
		SET state = ?, last_evaluated_at_us = ?
		WHERE role_assignment_id = ? AND state <> 'RELEASED'`, state, nowUS, assignmentID)
	if err != nil {
		return fmt.Errorf("set role assignment state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("role assignment state rows affected: %w", err)
	}
	if affected != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ExpireCompanionHeartbeats(ctx context.Context, timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		return 0, fmt.Errorf("%w: heartbeat timeout must be positive", domain.ErrInvalidInput)
	}
	now := s.clock.Now().UTC()
	nowUS := clock.UnixMicros(now)
	cutoffUS := clock.UnixMicros(now.Add(-timeout))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin heartbeat expiry: %w", err)
	}
	defer tx.Rollback()

	assignmentResult, err := tx.ExecContext(ctx, `
		UPDATE role_assignments
		SET state = 'OFFLINE', last_evaluated_at_us = ?
		WHERE state <> 'RELEASED'
		  AND companion_id IN (
		      SELECT companion_id FROM companions WHERE last_seen_at_us < ?
		  )`, nowUS, cutoffUS)
	if err != nil {
		return 0, fmt.Errorf("expire Companion role assignments: %w", err)
	}
	if _, err := assignmentResult.RowsAffected(); err != nil {
		return 0, fmt.Errorf("expired role assignment rows affected: %w", err)
	}

	companionResult, err := tx.ExecContext(ctx, `
		UPDATE companions
		SET readiness = 'OFFLINE', updated_at_us = ?
		WHERE last_seen_at_us < ? AND readiness <> 'OFFLINE'`, nowUS, cutoffUS)
	if err != nil {
		return 0, fmt.Errorf("expire Companion heartbeats: %w", err)
	}
	expired, err := companionResult.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("expired Companion rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit heartbeat expiry: %w", err)
	}
	return expired, nil
}

func validActiveRoleAssignmentState(state domain.RoleAssignmentState) bool {
	switch state {
	case domain.RoleAssigned, domain.RoleSyncing, domain.RoleReady, domain.RoleDegraded, domain.RoleOffline, domain.RoleMismatch:
		return true
	default:
		return false
	}
}
