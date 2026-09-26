package deviceexperience

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/lightingnode"
)

// TransferPreflightInput is operator intent. It is not authorization to
// mutate assignment, and the caller must independently validate the operator
// session, CSRF and project-edit + device-pairing permissions.
type TransferPreflightInput struct {
	DeviceID          string
	ExpectedProjectID string
	TargetProjectID   string
	ExpectedEpoch     int64
}

type TransferPreflight struct {
	DeviceID        string `json:"device_id"`
	FromProjectID   string `json:"from_project_id,omitempty"`
	ToProjectID     string `json:"to_project_id,omitempty"`
	AssignmentEpoch int64  `json:"assignment_epoch"`
	AssignmentState string `json:"assignment_state"`
	NextState       string `json:"next_state"`
	RequiredAction  string `json:"required_action"`
}

// PreflightTransfer is an immutable snapshot of server-side checks. A later
// transfer MUST re-check every condition inside its commit transaction, bind
// the current authenticated transport generation and verify a fresh device
// zero-output acknowledgment. Passing here is never physical blackout proof.
func (r *Repository) PreflightTransfer(ctx context.Context, input TransferPreflightInput) (TransferPreflight, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.ExpectedProjectID = strings.TrimSpace(input.ExpectedProjectID)
	input.TargetProjectID = strings.TrimSpace(input.TargetProjectID)
	if input.DeviceID == "" || input.ExpectedEpoch < 1 ||
		input.ExpectedProjectID == input.TargetProjectID {
		return TransferPreflight{}, fmt.Errorf("%w: device, epoch and distinct target required", ErrInvalidState)
	}
	device, err := r.GetDevice(ctx, input.DeviceID)
	if err != nil {
		return TransferPreflight{}, err
	}
	if device.ProtocolVersion != ProtocolVersion2 || !device.Enabled || device.ProfileID != lightingnode.ProfileID {
		return TransferPreflight{}, fmt.Errorf("%w: only enabled v2 devices may be transferred", ErrInvalidState)
	}
	record, err := r.GetAssignmentRecord(ctx, input.DeviceID)
	if err != nil {
		return TransferPreflight{}, err
	}
	validState := (record.State == "UNASSIGNED" && record.ProjectID == "" && record.RuntimeSnapshotID == "") ||
		(record.State == "BLOCKED" && record.ProjectID != "" && record.RuntimeSnapshotID == "") ||
		(record.State == "ACTIVE" && record.ProjectID != "" && record.RuntimeSnapshotID != "")
	if record.ProjectID != input.ExpectedProjectID || record.Epoch != input.ExpectedEpoch || !validState {
		return TransferPreflight{}, fmt.Errorf("%w: stale or unsupported assignment", ErrInvalidState)
	}
	if record.State == "ACTIVE" {
		var audited int
		if err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM stage_device_lighting_activation_audit
			WHERE device_id=? AND project_id=? AND runtime_snapshot_id=?
			      AND assignment_epoch=?
		`, input.DeviceID, record.ProjectID, record.RuntimeSnapshotID, record.Epoch).Scan(&audited); err != nil {
			return TransferPreflight{}, fmt.Errorf("verify active lighting activation audit: %w", err)
		}
		if audited != 1 {
			return TransferPreflight{}, fmt.Errorf("%w: ACTIVE lighting scope has no canonical activation audit", ErrInvalidState)
		}
	}
	// v2 bootstrap does not alter a legacy stage_devices project row.
	// Keep this preflight restricted to freshly unassigned v2 identities;
	// migration of previously project-linked devices requires separate audit.
	if device.ProjectID != "" {
		return TransferPreflight{}, fmt.Errorf("%w: legacy project migration not supported", ErrInvalidState)
	}
	for _, projectID := range []string{input.ExpectedProjectID, input.TargetProjectID} {
		if projectID == "" {
			continue
		}
		var count int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projects WHERE project_id = ?", projectID).Scan(&count); err != nil {
			return TransferPreflight{}, fmt.Errorf("validate transfer project: %w", err)
		}
		if count != 1 {
			return TransferPreflight{}, fmt.Errorf("%w: project not found", ErrInvalidState)
		}
		if err := r.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sessions
			WHERE project_id = ? AND session_type = 'SHOW' AND status = 'ACTIVE'
		`, projectID).Scan(&count); err != nil {
			return TransferPreflight{}, fmt.Errorf("inspect project SHOW lock: %w", err)
		}
		if count != 0 {
			return TransferPreflight{}, fmt.Errorf("%w: active SHOW locks project assignment", ErrInvalidState)
		}
	}
	var pending int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_device_commands
		WHERE device_id = ? AND status = 'ACCEPTED'
	`, input.DeviceID).Scan(&pending); err != nil {
		return TransferPreflight{}, fmt.Errorf("inspect outstanding commands: %w", err)
	}
	if pending != 0 {
		return TransferPreflight{}, fmt.Errorf("%w: device has outstanding commands", ErrInvalidState)
	}
	result := TransferPreflight{
		DeviceID: input.DeviceID, FromProjectID: record.ProjectID,
		ToProjectID: input.TargetProjectID, AssignmentEpoch: record.Epoch,
		AssignmentState: record.State, NextState: "BLOCKED",
		RequiredAction: "AUTHENTICATED_BLACKOUT_ACK",
	}
	if input.TargetProjectID == "" {
		result.NextState = "UNASSIGNED"
	}
	return result, nil
}

