package devicechannel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

const lightingSnapshotRebindWait = 12 * time.Second

type LightingSnapshotRebindResult struct {
	DeviceID          string                                     `json:"device_id"`
	ProjectID         string                                     `json:"project_id"`
	RuntimeSnapshotID string                                     `json:"runtime_snapshot_id"`
	Unassign          *deviceexperience.TransferCommitRecord     `json:"unassign,omitempty"`
	Reassign          *deviceexperience.TransferCommitRecord     `json:"reassign,omitempty"`
	Activation        *deviceexperience.LightingActivationCommit `json:"activation,omitempty"`
	CommandsEnabled   bool                                       `json:"commands_enabled"`
}

// RebindLightingSnapshotAuthorized safely converges one v2 Lighting Node onto
// a Published Runtime Snapshot for a Project. It is deliberately resumable:
// a previous attempt may have stopped at UNASSIGNED or BLOCKED and a retry
// continues from that fail-closed state.
//
// Normal path:
// ACTIVE(old snapshot) -> authenticated software blackout -> UNASSIGNED
// -> authenticated software blackout -> BLOCKED(new epoch)
// -> fresh BLOCKED software-zero ACK -> activate published config
// -> fresh authenticated reconnect -> ACTIVE(new snapshot).
//
// Any failure leaves the node fail-closed (UNASSIGNED/BLOCKED or commands off).
func (r *Runtime) RebindLightingSnapshotAuthorized(
	ctx context.Context,
	deviceID, projectID, runtimeSnapshotID, actorID string,
	authorize func(context.Context) error,
) (LightingSnapshotRebindResult, error) {
	var out LightingSnapshotRebindResult
	if r == nil || r.repository == nil {
		return out, fmt.Errorf("lighting snapshot rebind runtime unavailable")
	}
	deviceID = strings.TrimSpace(deviceID)
	projectID = strings.TrimSpace(projectID)
	runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
	actorID = strings.TrimSpace(actorID)
	if deviceID == "" || projectID == "" || runtimeSnapshotID == "" || actorID == "" {
		return out, fmt.Errorf("lighting snapshot rebind scope is incomplete")
	}
	out.DeviceID = deviceID
	out.ProjectID = projectID
	out.RuntimeSnapshotID = runtimeSnapshotID

	// Validate the target Published Runtime Snapshot and authoritative lighting
	// binding before mutating the current assignment.
	if _, err := r.repository.ResolveLightingScope(ctx, deviceID, projectID, runtimeSnapshotID); err != nil {
		return out, err
	}

	record, err := r.repository.GetAssignmentRecord(ctx, deviceID)
	if err != nil {
		return out, err
	}

	if record.State == "ACTIVE" && record.ProjectID == projectID &&
		record.RuntimeSnapshotID == runtimeSnapshotID {
		if err := r.waitForActiveV2Scope(
			ctx, deviceID, projectID, runtimeSnapshotID, record.Epoch, lightingSnapshotRebindWait,
		); err != nil {
			return out, fmt.Errorf("%w: target lighting assignment exists but current authenticated scope is not ready: %v", deviceexperience.ErrInvalidState, err)
		}
		out.CommandsEnabled = true
		return out, nil
	}

	// First converge any stale ACTIVE scope to UNASSIGNED. This is the same
	// authenticated software-blackout transfer used for explicit Project moves.
	if record.State == "ACTIVE" {
		if record.ProjectID != projectID || strings.TrimSpace(record.RuntimeSnapshotID) == "" {
			return out, fmt.Errorf("%w: lighting node ACTIVE scope is not the expected Project", deviceexperience.ErrInvalidState)
		}
		startGeneration, ok := r.CurrentV2Generation(deviceID)
		if !ok {
			return out, fmt.Errorf("%w: authenticated v2 lighting socket unavailable", deviceexperience.ErrInvalidState)
		}
		unassign, err := r.ExecuteReservedSoftwareTransferAuthorized(
			ctx,
			deviceexperience.TransferPreflightInput{
				DeviceID:          deviceID,
				ExpectedProjectID: projectID,
				TargetProjectID:   "",
				ExpectedEpoch:     record.Epoch,
			},
			actorID,
			authorize,
		)
		if err != nil {
			return out, fmt.Errorf("blackout/unassign stale lighting scope: %w", err)
		}
		out.Unassign = &unassign
		if err := r.waitForNewV2Generation(ctx, deviceID, startGeneration, lightingSnapshotRebindWait); err != nil {
			return out, fmt.Errorf("wait for lighting reconnect after unassign: %w", err)
		}
		record, err = r.repository.GetAssignmentRecord(ctx, deviceID)
		if err != nil {
			return out, err
		}
	}

	// A retry may begin here after a previous attempt safely stopped at
	// UNASSIGNED. Reattach the physical node to the same Project as BLOCKED.
	if record.State == "UNASSIGNED" {
		if record.ProjectID != "" || record.RuntimeSnapshotID != "" {
			return out, fmt.Errorf("%w: unassigned lighting node carries stale scope", deviceexperience.ErrInvalidState)
		}
		unassignedGeneration, ok := r.CurrentV2Generation(deviceID)
		if !ok {
			return out, fmt.Errorf("%w: unassigned lighting node is offline", deviceexperience.ErrInvalidState)
		}
		reassign, err := r.ExecuteReservedSoftwareTransferAuthorized(
			ctx,
			deviceexperience.TransferPreflightInput{
				DeviceID:          deviceID,
				ExpectedProjectID: "",
				TargetProjectID:   projectID,
				ExpectedEpoch:     record.Epoch,
			},
			actorID,
			authorize,
		)
		if err != nil {
			return out, fmt.Errorf("blackout/reassign lighting to Project: %w", err)
		}
		out.Reassign = &reassign
		if err := r.waitForBlockedEpochAck(
			ctx, deviceID, projectID, reassign.ToEpoch, unassignedGeneration, lightingSnapshotRebindWait,
		); err != nil {
			return out, fmt.Errorf("wait for current BLOCKED lighting zero ACK: %w", err)
		}
		record, err = r.repository.GetAssignmentRecord(ctx, deviceID)
		if err != nil {
			return out, err
		}
	}

	// A retry may also begin here if assignment to the Project committed but
	// activation did not. Require the current authenticated BLOCKED zero ACK.
	if record.State != "BLOCKED" || record.ProjectID != projectID || record.RuntimeSnapshotID != "" {
		return out, fmt.Errorf("%w: lighting node did not converge to BLOCKED Project scope", deviceexperience.ErrInvalidState)
	}
	if err := r.waitForBlockedEpochAck(
		ctx, deviceID, projectID, record.Epoch, 0, lightingSnapshotRebindWait,
	); err != nil {
		return out, fmt.Errorf("wait for current BLOCKED lighting zero ACK: %w", err)
	}

	activation, err := r.ExecuteLightingActivationAuthorized(
		ctx,
		deviceexperience.LightingActivationInput{
			DeviceID:          deviceID,
			ProjectID:         projectID,
			RuntimeSnapshotID: runtimeSnapshotID,
			ExpectedEpoch:     record.Epoch,
		},
		actorID,
		authorize,
	)
	if err != nil {
		return out, fmt.Errorf("activate lighting on published Runtime Snapshot: %w", err)
	}
	out.Activation = &activation

	if err := r.waitForActiveV2Scope(
		ctx, deviceID, projectID, runtimeSnapshotID, record.Epoch, lightingSnapshotRebindWait,
	); err != nil {
		return out, fmt.Errorf("wait for active lighting Runtime Snapshot scope: %w", err)
	}
	out.CommandsEnabled = true
	return out, nil
}

func (r *Runtime) waitForNewV2Generation(
	ctx context.Context, deviceID string, previous int64, timeout time.Duration,
) error {
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if generation, ok := r.CurrentV2Generation(deviceID); ok && generation != previous {
			return nil
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runtime) waitForBlockedEpochAck(
	ctx context.Context,
	deviceID, projectID string,
	epoch, previousGeneration int64,
	timeout time.Duration,
) error {
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		generation, online := r.CurrentV2Generation(deviceID)
		if online && (previousGeneration == 0 || generation != previousGeneration) {
			if ack, err := r.repository.GetBlockedEpochAck(wait, deviceID, epoch); err == nil &&
				ack.ProjectID == projectID && ack.ConnectionGeneration == generation {
				return nil
			}
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runtime) waitForActiveV2Scope(
	ctx context.Context,
	deviceID, projectID, runtimeSnapshotID string,
	epoch int64,
	timeout time.Duration,
) error {
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if scope, ok := r.CurrentV2Scope(deviceID); ok &&
			scope.ProjectID == projectID &&
			scope.RuntimeSnapshotID == runtimeSnapshotID &&
			scope.AssignmentEpoch == epoch &&
			scope.CommandsEnabled {
			return nil
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-ticker.C:
		}
	}
}
