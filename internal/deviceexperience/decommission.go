package deviceexperience

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	stageid "github.com/ali96adil/StageCore/internal/id"
)

type DecommissionRecord struct {
	ID                string `json:"decommission_id"`
	DeviceID          string `json:"device_id"`
	ActorID           string `json:"actor_id"`
	Reason            string `json:"reason"`
	AssignmentState   string `json:"assignment_state"`
	ProjectID         string `json:"project_id,omitempty"`
	RuntimeSnapshotID string `json:"runtime_snapshot_id,omitempty"`
	AssignmentEpoch   int64  `json:"assignment_epoch"`
}

func (r *Repository) DecommissionOfflineTablet(ctx context.Context, deviceID, actorID, reason string) (DecommissionRecord, error) {
	deviceID = strings.TrimSpace(deviceID)
	actorID = strings.TrimSpace(actorID)
	reason = strings.TrimSpace(reason)
	if deviceID == "" || actorID == "" {
		return DecommissionRecord{}, ErrInvalidDevice
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return DecommissionRecord{}, fmt.Errorf("begin tablet decommission: %w", err)
	}
	defer tx.Rollback()

	var kind DeviceKind
	var protocol string
	var enabled int
	if err := tx.QueryRowContext(ctx, `
		SELECT device_kind, protocol_version, enabled
		FROM stage_devices WHERE device_id = ?
	`, deviceID).Scan(&kind, &protocol, &enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DecommissionRecord{}, sql.ErrNoRows
		}
		return DecommissionRecord{}, fmt.Errorf("read tablet for decommission: %w", err)
	}
	if kind != DeviceTabletPlayer || protocol != ProtocolVersion2 {
		return DecommissionRecord{}, fmt.Errorf("%w: only v2 Tablet Player identities can be decommissioned", ErrInvalidDevice)
	}
	if enabled != 1 {
		return DecommissionRecord{}, fmt.Errorf("%w: tablet identity is already disabled", ErrInvalidState)
	}

	var connection sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT connection_state FROM stage_device_runtime_state WHERE device_id = ?
	`, deviceID).Scan(&connection); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DecommissionRecord{}, fmt.Errorf("read tablet runtime before decommission: %w", err)
	}
	if connection.Valid && connection.String == string(ConnectionOnline) {
		return DecommissionRecord{}, fmt.Errorf("%w: online Tablet Player cannot be decommissioned", ErrInvalidState)
	}

	var record DecommissionRecord
	var projectID, snapshotID sql.NullString
	record.DeviceID = deviceID
	record.ActorID = actorID
	record.Reason = reason
	if err := tx.QueryRowContext(ctx, `
		SELECT assignment_state, project_id, runtime_snapshot_id, assignment_epoch
		FROM stage_device_assignments WHERE device_id = ?
	`, deviceID).Scan(&record.AssignmentState, &projectID, &snapshotID, &record.AssignmentEpoch); err != nil {
		return DecommissionRecord{}, fmt.Errorf("read tablet assignment before decommission: %w", err)
	}
	if projectID.Valid {
		record.ProjectID = projectID.String
	}
	if snapshotID.Valid {
		record.RuntimeSnapshotID = snapshotID.String
	}

	if record.ProjectID != "" {
		var activeShowCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sessions
			WHERE project_id = ? AND status = 'ACTIVE' AND session_type = 'SHOW'
		`, record.ProjectID).Scan(&activeShowCount); err != nil {
			return DecommissionRecord{}, fmt.Errorf("check active SHOW before decommission: %w", err)
		}
		if activeShowCount != 0 {
			return DecommissionRecord{}, fmt.Errorf("%w: cannot decommission Tablet Player during active SHOW", ErrInvalidState)
		}
	}

	var pending int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id = ? AND status = 'ACCEPTED'
	`, deviceID).Scan(&pending); err != nil {
		return DecommissionRecord{}, fmt.Errorf("check pending tablet commands: %w", err)
	}
	if pending != 0 {
		return DecommissionRecord{}, fmt.Errorf("%w: Tablet Player has pending commands", ErrInvalidState)
	}

	nowUS := r.now().UTC().UnixMicro()
	if _, err := tx.ExecContext(ctx, `
		UPDATE stage_devices SET enabled = 0, updated_at_us = ? WHERE device_id = ? AND enabled = 1
	`, nowUS, deviceID); err != nil {
		return DecommissionRecord{}, fmt.Errorf("disable tablet identity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE stage_device_runtime_state
		SET connection_state = 'REVOKED', readiness = 'BLOCKER'
		WHERE device_id = ?
	`, deviceID); err != nil {
		return DecommissionRecord{}, fmt.Errorf("revoke tablet runtime state: %w", err)
	}

	record.ID, err = stageid.New()
	if err != nil {
		return DecommissionRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO stage_device_decommission_audit
		(decommission_id, device_id, actor_id, reason, assignment_state, project_id,
		 runtime_snapshot_id, assignment_epoch, decommissioned_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ID, record.DeviceID, record.ActorID, record.Reason, record.AssignmentState,
		record.ProjectID, record.RuntimeSnapshotID, record.AssignmentEpoch, nowUS); err != nil {
		return DecommissionRecord{}, fmt.Errorf("record tablet decommission audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return DecommissionRecord{}, fmt.Errorf("commit tablet decommission: %w", err)
	}
	return record, nil
}
