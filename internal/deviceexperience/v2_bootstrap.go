package deviceexperience

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const ProtocolVersion2 = "stagecore.device/2"

// RegisterUnassignedV2 is only for an already-authenticated device with a
// matching Companion runtime identity. The caller MUST NOT take that identity
// from an anonymous device.hello or mDNS announcement.
//
// New identities always bootstrap UNASSIGNED. Already-v2 identities can
// reconnect in UNASSIGNED or BLOCKED to read the Hub-owned epoch/project and
// remain dark. This method never migrates a v1 node, transfers a project,
// enables v2 commands, accepts a client project or verifies physical blackout.
func (r *Repository) RegisterUnassignedV2(ctx context.Context, device Device) (Device, error) {
	device.ID = strings.TrimSpace(device.ID)
	device.ProjectID = strings.TrimSpace(device.ProjectID)
	device.ProfileID = strings.TrimSpace(device.ProfileID)
	device.DisplayName = strings.TrimSpace(device.DisplayName)
	device.Platform = strings.TrimSpace(device.Platform)
	device.Architecture = strings.TrimSpace(device.Architecture)
	device.ClientVersion = strings.TrimSpace(device.ClientVersion)
	device.GroupName = strings.TrimSpace(device.GroupName)
	device.LocationName = strings.TrimSpace(device.LocationName)
	device.Capabilities = normalizeCapabilities(device.Capabilities)
	if device.ID == "" || device.ProjectID != "" ||
		device.DisplayName == "" || !validDeviceKind(device.Kind) ||
		device.ProtocolVersion != ProtocolVersion2 {
		return Device{}, fmt.Errorf("%w: v2 bootstrap requires an unassigned authenticated identity", ErrInvalidDevice)
	}
	caps, err := json.Marshal(device.Capabilities)
	if err != nil {
		return Device{}, fmt.Errorf("encode v2 capabilities: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, fmt.Errorf("begin v2 bootstrap: %w", err)
	}
	defer tx.Rollback()

	// A known v1 node must use an explicit, authenticated migration; a new
	// device cannot seize its ID or overwrite project-linked command history.
	var protocol, kind, legacyProject, assignmentState, assignedProject string
	var enabled int
	err = tx.QueryRowContext(ctx, `
		SELECT d.protocol_version, d.device_kind, COALESCE(d.project_id, ''),
		       d.enabled, a.assignment_state, COALESCE(a.project_id, '')
		FROM stage_devices d
		JOIN stage_device_assignments a ON a.device_id = d.device_id
		WHERE d.device_id = ?
	`, device.ID).Scan(&protocol, &kind, &legacyProject, &enabled, &assignmentState, &assignedProject)
	switch {
	case err == nil:
		if protocol != ProtocolVersion2 || kind != string(device.Kind) ||
			legacyProject != "" || enabled != 1 ||
			(assignmentState != "UNASSIGNED" && assignmentState != "BLOCKED") ||
			(assignmentState == "UNASSIGNED" && assignedProject != "") ||
			(assignmentState == "BLOCKED" && assignedProject == "") {
			return Device{}, fmt.Errorf("%w: v2 enrollment conflicts with existing device authority", ErrInvalidDevice)
		}
		// No UPDATE for a reconnect: client metadata cannot alter the
		// Hub-owned identity/assignment or restore a disabled device.
	case errors.Is(err, sql.ErrNoRows):
		nowUS := r.now().UTC().UnixMicro()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO stage_devices
			(device_id, project_id, profile_id, device_kind, display_name,
			 platform, architecture, client_version, protocol_version,
			 capabilities_json, group_name, location_name, enabled,
			 created_at_us, updated_at_us)
			VALUES (?, NULL, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		`, device.ID, device.ProfileID, device.Kind, device.DisplayName,
			device.Platform, device.Architecture, device.ClientVersion,
			ProtocolVersion2, string(caps), device.GroupName, device.LocationName,
			nowUS, nowUS)
		if err != nil {
			return Device{}, fmt.Errorf("insert unassigned v2 device: %w", err)
		}
		// The schema-29 insert trigger creates the independent identity and a
		// LEGACY sidecar; convert it before transaction commit. At no point is
		// a project provided or any ordinary command accepted.
		result, err := tx.ExecContext(ctx, `
			UPDATE stage_device_assignments
			SET assignment_state = 'UNASSIGNED', updated_at_us = ?
			WHERE device_id = ? AND assignment_state = 'LEGACY' AND project_id IS NULL
		`, nowUS, device.ID)
		if err != nil {
			return Device{}, fmt.Errorf("quarantine new v2 device: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return Device{}, fmt.Errorf("%w: v2 assignment bootstrap was not isolated", ErrInvalidState)
		}
	default:
		return Device{}, fmt.Errorf("inspect v2 bootstrap identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Device{}, fmt.Errorf("commit v2 bootstrap: %w", err)
	}
	return r.GetDevice(ctx, device.ID)
}
