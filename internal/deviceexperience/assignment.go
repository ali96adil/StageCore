package deviceexperience

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AssignmentRecord is preparatory metadata for the future Hub-owned v2
// assignment protocol. LEGACY entries mirror a stagecore.device/1 registration
// and DO NOT grant independent project, command, or transfer authority.
//
// Existing stage_devices.project_id remains authoritative until the reviewed
// v2 migration, authenticated zero-output acknowledgment and runtime fencing
// have all shipped. This read-only API cannot perform a transfer.
type AssignmentRecord struct {
	DeviceID          string    `json:"device_id"`
	ProjectID         string    `json:"project_id,omitempty"`
	Epoch             int64     `json:"assignment_epoch"`
	State             string    `json:"assignment_state"`
	RuntimeSnapshotID string    `json:"runtime_snapshot_id,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

const AssignmentLegacy = "LEGACY"

// GetAssignmentRecord reads the non-authoritative v2 sidecar. It is intentionally
// NOT called by command dispatch or readiness paths until the v2 runtime
// protocol and transfer service can enforce the entire assignment contract.
func (r *Repository) GetAssignmentRecord(ctx context.Context, deviceID string) (AssignmentRecord, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return AssignmentRecord{}, ErrInvalidDevice
	}
	var record AssignmentRecord
	var project sql.NullString
	var updatedUS int64
	err := r.db.QueryRowContext(ctx, `
		SELECT device_id, project_id, assignment_epoch, assignment_state,
		       runtime_snapshot_id, updated_at_us
		FROM stage_device_assignments WHERE device_id = ?
	`, deviceID).Scan(&record.DeviceID, &project, &record.Epoch, &record.State,
		&record.RuntimeSnapshotID, &updatedUS)
	if err != nil {
		return AssignmentRecord{}, fmt.Errorf("read Stage Device assignment metadata: %w", err)
	}
	if project.Valid {
		record.ProjectID = project.String
	}
	record.UpdatedAt = time.UnixMicro(updatedUS).UTC()
	return record, nil
}
