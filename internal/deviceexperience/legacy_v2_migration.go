package deviceexperience

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

type LegacyLightingV2Migration struct {
	MigrationID string `json:"migration_id"`
	DeviceID    string `json:"device_id"`
	ProjectID   string `json:"project_id"`
	FromEpoch   int64  `json:"from_epoch"`
	ToEpoch     int64  `json:"to_epoch"`
	NextState   string `json:"next_state"`
}

func (r *Repository) MigrateLegacyLightingToV2Blocked(
	ctx context.Context,
	deviceID, projectID, actorID string,
) (LegacyLightingV2Migration, error) {
	deviceID = strings.TrimSpace(deviceID)
	projectID = strings.TrimSpace(projectID)
	actorID = strings.TrimSpace(actorID)
	if deviceID == "" || projectID == "" || actorID == "" {
		return LegacyLightingV2Migration{}, ErrInvalidState
	}

	migrationID, err := stageid.New()
	if err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("allocate legacy v2 migration ID: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("begin legacy v2 migration: %w", err)
	}
	defer tx.Rollback()

	var (
		deviceProject, profileID, kind, protocol string
		assignmentProject, assignmentState, snapshotID string
		enabled int
		epoch int64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(d.project_id,''), COALESCE(d.profile_id,''),
		       d.device_kind, d.protocol_version, d.enabled,
		       COALESCE(a.project_id,''), a.assignment_state,
		       a.runtime_snapshot_id, a.assignment_epoch
		FROM stage_devices d
		JOIN stage_device_assignments a ON a.device_id = d.device_id
		WHERE d.device_id = ?
	`, deviceID).Scan(
		&deviceProject, &profileID, &kind, &protocol, &enabled,
		&assignmentProject, &assignmentState, &snapshotID, &epoch,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return LegacyLightingV2Migration{}, err
		}
		return LegacyLightingV2Migration{}, fmt.Errorf("read legacy lighting migration state: %w", err)
	}
	if enabled != 1 ||
		protocol != ProtocolVersion1 ||
		deviceProject != projectID ||
		profileID != lightingnode.ProfileID ||
		kind != string(DeviceGeneric) ||
		assignmentState != AssignmentLegacy ||
		assignmentProject != projectID ||
		snapshotID != "" ||
		epoch <= 0 {
		return LegacyLightingV2Migration{}, fmt.Errorf(
			"%w: device is not an eligible legacy lighting node", ErrInvalidState)
	}

	var activeSessions int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sessions
		WHERE project_id = ? AND status = 'ACTIVE'
	`, projectID).Scan(&activeSessions); err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("inspect active sessions for legacy v2 migration: %w", err)
	}
	if activeSessions != 0 {
		return LegacyLightingV2Migration{}, fmt.Errorf(
			"%w: active session blocks legacy lighting migration", ErrInvalidState)
	}

	var pending int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id = ? AND status = 'ACCEPTED'
	`, deviceID).Scan(&pending); err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("inspect pending commands for legacy v2 migration: %w", err)
	}
	if pending != 0 {
		return LegacyLightingV2Migration{}, fmt.Errorf(
			"%w: pending command blocks legacy lighting migration", ErrInvalidState)
	}

	nowUS := r.now().UTC().UnixMicro()
	result, err := tx.ExecContext(ctx, `
		UPDATE stage_devices
		SET project_id = NULL, protocol_version = ?, updated_at_us = ?
		WHERE device_id = ? AND project_id = ? AND protocol_version = ?
		      AND profile_id = ? AND device_kind = ? AND enabled = 1
	`, ProtocolVersion2, nowUS, deviceID, projectID, ProtocolVersion1,
		lightingnode.ProfileID, DeviceGeneric)
	if err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("fence legacy lighting device authority: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return LegacyLightingV2Migration{}, fmt.Errorf(
			"%w: legacy lighting authority changed during migration", ErrInvalidState)
	}

	nextEpoch := epoch + 1
	result, err = tx.ExecContext(ctx, `
		UPDATE stage_device_assignments
		SET assignment_state = 'BLOCKED', project_id = ?,
		    assignment_epoch = ?, runtime_snapshot_id = '', updated_at_us = ?
		WHERE device_id = ? AND assignment_state = 'LEGACY'
		      AND project_id = ? AND assignment_epoch = ?
		      AND runtime_snapshot_id = ''
	`, projectID, nextEpoch, nowUS, deviceID, projectID, epoch)
	if err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("enter BLOCKED v2 assignment: %w", err)
	}
	changed, err = result.RowsAffected()
	if err != nil || changed != 1 {
		return LegacyLightingV2Migration{}, fmt.Errorf(
			"%w: legacy assignment changed during migration", ErrInvalidState)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO stage_device_legacy_v2_migrations
		(migration_id, device_id, actor_id, project_id, from_protocol,
		 to_protocol, from_epoch, to_epoch, next_state, migrated_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'BLOCKED', ?)
	`, migrationID, deviceID, actorID, projectID, ProtocolVersion1,
		ProtocolVersion2, epoch, nextEpoch, nowUS); err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("record legacy v2 migration audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return LegacyLightingV2Migration{}, fmt.Errorf("commit legacy v2 migration: %w", err)
	}
	return LegacyLightingV2Migration{
		MigrationID: migrationID,
		DeviceID: deviceID,
		ProjectID: projectID,
		FromEpoch: epoch,
		ToEpoch: nextEpoch,
		NextState: "BLOCKED",
	}, nil
}
