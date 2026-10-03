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
	DeviceID          string                                      `json:"device_id"`
	ProjectID         string                                      `json:"project_id"`
	RuntimeSnapshotID string                                      `json:"runtime_snapshot_id"`
	Unassign          *deviceexperience.TransferCommitRecord      `json:"unassign,omitempty"`
	Reassign          *deviceexperience.TransferCommitRecord      `json:"reassign,omitempty"`
	Activation        *deviceexperience.LightingActivationCommit  `json:"activation,omitempty"`
	CommandsEnabled   bool                                        `json:"commands_enabled"`
}

// RebindLightingSnapshotAuthorized safely moves one ACTIVE v2 Lighting Node
// from an older Runtime Snapshot to a newly Published Runtime Snapshot for the
// same Project. The existing transfer/activation safety handshakes remain the
// only authority-changing operations:
//
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
	if record.State != "ACTIVE" || record.ProjectID != projectID ||
		strings.TrimSpace(record.RuntimeSnapshotID) == "" {
		return out, fmt.Errorf("%w: lighting node is not ACTIVE for the expected Project", deviceexperience.ErrInvalidState)
	}
	if record.RuntimeSnapshotID == runtimeSnapshotID {
		if scope, ok := r.CurrentV2Scope(deviceID); ok &&
			scope.ProjectID == projectID &&
			scope.RuntimeSnapshotID == runtimeSnapshotID &&
			scope.AssignmentEpoch == record.Epoch &&
			scope.CommandsEnabled {
			out.CommandsEnabled = true
			return out, nil
		}
		return out, fmt.Errorf("%w: target lighting assignment exists but current authenticated scope is not ready", deviceexperience.ErrInvalidState)
	}

	startGeneration, _ := r.CurrentV2Generation(deviceID)
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

	unassignedGeneration, _ := r.CurrentV2Generation(deviceID)
	reassign, err := r.ExecuteReservedSoftwareTransferAuthorized(
		ctx,
		deviceexperience.TransferPreflightInput{
			DeviceID:          deviceID,
			ExpectedProjectID: "",
			TargetProjectID:   projectID,
			ExpectedEpoch:     unassign.ToEpoch,
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

	activation, err := r.ExecuteLightingActivationAuthorized(
		ctx,
		deviceexperience.LightingActivationInput{
			DeviceID:          deviceID,
			ProjectID:         projectID,
			RuntimeSnapshotID: runtimeSnapshotID,
			ExpectedEpoch:     reassign.ToEpoch,
		},
		actorID,
		authorize,
	)
	if err != nil {
		return out, fmt.Errorf("activate lighting on published Runtime Snapshot: %w", err)
	}
	out.Activation = &activation

	if err := r.waitForActiveV2Scope(
		ctx, deviceID, projectID, runtimeSnapshotID, reassign.ToEpoch, lightingSnapshotRebindWait,
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
		if online && generation != previousGeneration {
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
