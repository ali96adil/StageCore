package sessionsafety

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/store"
)

// NewStageDeviceSnapshotGate verifies the target Published Runtime Snapshot
// before Session start. A cross-Project Snapshot is a hard invariant failure,
// but managed v2 Tablet/Lighting readiness is advisory: a live show must be
// able to continue in degraded mode when one physical device is unavailable.
//
// This is intentionally read-only. Publish/Sync owns authority transitions;
// Session start reports incomplete device transitions without mutating them.
func NewStageDeviceSnapshotGate(
	stageStore *store.Store,
	devices *deviceexperience.Repository,
	runtime *devicechannel.Runtime,
) func(context.Context, string, string) (bool, string, error) {
	return func(ctx context.Context, projectID, runtimeSnapshotID string) (bool, string, error) {
		projectID = strings.TrimSpace(projectID)
		runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
		if stageStore == nil || devices == nil || runtime == nil ||
			projectID == "" || runtimeSnapshotID == "" {
			return false, "", fmt.Errorf("managed Stage Device snapshot gate is unavailable")
		}

		snapshot, err := stageStore.GetRuntimeSnapshot(ctx, runtimeSnapshotID)
		if err != nil {
			return false, "", err
		}
		if snapshot.ProjectID != projectID {
			return false, "Published Runtime Snapshot does not belong to this Project.", nil
		}

		// Every enabled v2 Tablet currently assigned to this Project is managed
		// output. Snapshot/readiness mismatches are reported as degraded-start
		// warnings, not Session blockers.
		items, err := devices.ListDevices(ctx, projectID)
		if err != nil {
			return false, "", err
		}
		warnings := make([]string, 0)
		for _, device := range items {
			if !device.Enabled ||
				device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
				device.Kind != deviceexperience.DeviceTabletPlayer ||
				device.ProfileID != deviceexperience.TabletPlayerProfileID {
				continue
			}
			assignment := device.Assignment
			if assignment == nil ||
				assignment.State != "ACTIVE" ||
				assignment.ProjectID != projectID ||
				assignment.RuntimeSnapshotID != runtimeSnapshotID {
				warnings = append(warnings, fmt.Sprintf(
					"Tablet %s is not synchronized to the latest Published Runtime Snapshot.",
					stageDeviceGateName(device),
				))
				continue
			}
			scope, ok := runtime.CurrentV2Scope(device.ID)
			if !ok ||
				scope.ProjectID != projectID ||
				scope.RuntimeSnapshotID != runtimeSnapshotID ||
				scope.AssignmentEpoch != assignment.Epoch ||
				!scope.CommandsEnabled {
				warnings = append(warnings, fmt.Sprintf(
					"Tablet %s has not completed its fresh Runtime Snapshot reconnect.",
					stageDeviceGateName(device),
				))
			}
		}

		// Lighting membership is authored into the immutable Snapshot, so only
		// bindings required by this exact Snapshot are start-gating.
		var manifest struct {
			LightingNodes []struct {
				DeviceID string `json:"device_id"`
			} `json:"lighting_nodes"`
		}
		if err := json.Unmarshal(snapshot.Manifest, &manifest); err != nil {
			return false, "", fmt.Errorf("decode Runtime Snapshot lighting bindings: %w", err)
		}
		for _, binding := range manifest.LightingNodes {
			deviceID := strings.TrimSpace(binding.DeviceID)
			if deviceID == "" {
				continue
			}
			device, err := devices.GetDevice(ctx, deviceID)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf(
					"Lighting Node %s required by the Published Runtime Snapshot is unavailable.",
					deviceID,
				))
				continue
			}
			if !device.Enabled ||
				device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
				device.ProfileID != lightingnode.ProfileID ||
				device.Assignment == nil ||
				device.Assignment.State != "ACTIVE" ||
				device.Assignment.ProjectID != projectID ||
				device.Assignment.RuntimeSnapshotID != runtimeSnapshotID {
				warnings = append(warnings, fmt.Sprintf(
					"Lighting Node %s is not synchronized to the latest Published Runtime Snapshot.",
					stageDeviceGateName(device),
				))
				continue
			}
			scope, ok := runtime.CurrentV2Scope(device.ID)
			if !ok ||
				scope.ProjectID != projectID ||
				scope.RuntimeSnapshotID != runtimeSnapshotID ||
				scope.AssignmentEpoch != device.Assignment.Epoch ||
				!scope.CommandsEnabled {
				warnings = append(warnings, fmt.Sprintf(
					"Lighting Node %s has not completed its fresh Runtime Snapshot reconnect.",
					stageDeviceGateName(device),
				))
			}
		}
		return true, strings.Join(warnings, " "), nil
	}
}

func stageDeviceGateName(device deviceexperience.Device) string {
	if name := strings.TrimSpace(device.DisplayName); name != "" {
		return name
	}
	return device.ID
}
