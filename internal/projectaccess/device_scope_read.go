package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ResolveActiveV2DeviceProject is a read-only, NOT YET WIRED qualification
// primitive for StageCore's Project-scoped V2 device metadata routes.
//
// Identity/project information from device.hello, stage_devices.project_id,
// a requested URL, or a stale Project assignment is not authorization.
// Only a Hub-owned ACTIVE v2 assignment to a published Snapshot, with an
// exact matching profile-specific safe-assignment/activation audit, qualifies.
//
// This is deliberately NOT an operational/actuation authority API. Real
// DMX, StageLaser and Tablet commands also require authenticated transport,
// epoch, session, snapshot, output safety and dispatch-time checks.
// Unassigned/BLOCKED/PREPARING devices belong to Owner-only global inventory,
// not to any ordinary Operator's Project-scoped device list.
func (s *Service) ResolveActiveV2DeviceProject(ctx context.Context, deviceID string) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrDenied
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", ErrDenied
	}

	var projectID string
	err := s.db.QueryRowContext(ctx, `
		SELECT a.project_id
		FROM stage_devices d
		JOIN stage_device_assignments a ON a.device_id = d.device_id
		JOIN runtime_snapshots rs
		  ON rs.runtime_snapshot_id = a.runtime_snapshot_id
		 AND rs.project_id = a.project_id
		 AND rs.status = 'PUBLISHED'
		WHERE d.device_id = ?
		  AND d.enabled = 1
		  AND d.protocol_version = 'stagecore.device/2'
		  AND d.project_id IS NULL
		  AND a.assignment_state = 'ACTIVE'
		  AND a.assignment_epoch > 1
		  AND a.project_id IS NOT NULL
		  AND a.project_id <> ''
		  AND a.runtime_snapshot_id <> ''
		  AND (
		    (
		      d.device_kind = 'TABLET_PLAYER'
		      AND COALESCE(d.profile_id, '') = 'stagecore.tablet-player'
		      AND EXISTS (
		        SELECT 1 FROM stage_device_tablet_assignment_audit x
		        WHERE x.device_id = a.device_id
		          AND x.to_epoch = a.assignment_epoch
		          AND x.to_project_id = a.project_id
		          AND x.to_runtime_snapshot_id = a.runtime_snapshot_id
		          AND x.next_state = 'ACTIVE'
		      )
		    )
		    OR
		    (
		      d.device_kind = 'GENERIC'
		      AND COALESCE(d.profile_id, '') = 'stagecore.esp32-dmx-lighting-node'
		      AND EXISTS (
		        SELECT 1 FROM stage_device_lighting_activation_audit x
		        WHERE x.device_id = a.device_id
		          AND x.assignment_epoch = a.assignment_epoch
		          AND x.project_id = a.project_id
		          AND x.runtime_snapshot_id = a.runtime_snapshot_id
		      )
		    )
		    OR
		    (
		      d.device_kind = 'GENERIC'
		      AND COALESCE(d.profile_id, '') = 'stagecore.esp32-stagelaser'
		      AND EXISTS (
		        SELECT 1 FROM stage_device_stagelaser_assignment_audit x
		        WHERE x.device_id = a.device_id
		          AND x.to_epoch = a.assignment_epoch
		          AND x.project_id = a.project_id
		          AND x.runtime_snapshot_id = a.runtime_snapshot_id
		      )
		    )
		  )
	`, deviceID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		// No authoritative ACTIVE assignment; do not disclose whether the
		// ID is unknown, unassigned, disabled, stale, or assigned elsewhere.
		return "", ErrDenied
	}
	if err != nil {
		return "", fmt.Errorf("resolve ACTIVE v2 device Project: %w", err)
	}
	if strings.TrimSpace(projectID) == "" {
		return "", ErrDenied
	}
	return projectID, nil
}

// CheckActiveV2DeviceRead enforces a live project.view grant against the
// database-resolved ACTIVE V2 assignment. It is for future metadata reads
// only: NEVER reuse this method as show.go/device.configure/blackout authority.
func (s *Service) CheckActiveV2DeviceRead(
	ctx context.Context, userID, expectedProjectID, deviceID string,
) error {
	expectedProjectID = strings.TrimSpace(expectedProjectID)
	if expectedProjectID == "" {
		return ErrDenied
	}
	actual, err := s.ResolveActiveV2DeviceProject(ctx, deviceID)
	if err != nil {
		return err
	}
	if actual != expectedProjectID {
		return ErrDenied
	}
	return s.Check(ctx, userID, actual, ProjectView)
}
