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

// NewStageDeviceSnapshotGate prevents REHEARSAL and SHOW from starting while
// managed v2 Tablets or Lighting required by the target Published Runtime
// Snapshot still hold authority for an older Snapshot.
//
// This is intentionally read-only. Publish/Sync owns authority transitions;
// Session start only verifies that those transitions completed.
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
		// output and must have exact current Snapshot authority before a Session
		// can start.
		items, err := devices.ListDevices(ctx, projectID)
		if err != nil {
			return false, "", err
		}
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
				return false, fmt.Sprintf(
					"Tablet %s is not synchronized to the latest Published Runtime Snapshot.",
					stageDeviceGateName(device),
				), nil
			}
			scope, ok := runtime.CurrentV2Scope(device.ID)
			if !ok ||
				scope.ProjectID != projectID ||
				scope.RuntimeSnapshotID != runtimeSnapshotID ||
				scope.AssignmentEpoch != assignment.Epoch ||
				!scope.CommandsEnabled {
				return false, fmt.Sprintf(
					"Tablet %s has not completed its fresh Runtime Snapshot reconnect.",
					stageDeviceGateName(device),
				), nil
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
				return false, fmt.Sprintf(
					"Lighting Node %s required by the Published Runtime Snapshot is unavailable.",
					deviceID,
				), nil
			}
			if !device.Enabled ||
				device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
				device.ProfileID != lightingnode.ProfileID ||
				device.Assignment == nil ||
				device.Assignment.State != "ACTIVE" ||
				device.Assignment.ProjectID != projectID ||
				device.Assignment.RuntimeSnapshotID != runtimeSnapshotID {
				return false, fmt.Sprintf(
					"Lighting Node %s is not synchronized to the latest Published Runtime Snapshot.",
					stageDeviceGateName(device),
				), nil
			}
			scope, ok := runtime.CurrentV2Scope(device.ID)
			if !ok ||
				scope.ProjectID != projectID ||
				scope.RuntimeSnapshotID != runtimeSnapshotID ||
				scope.AssignmentEpoch != device.Assignment.Epoch ||
				!scope.CommandsEnabled {
				return false, fmt.Sprintf(
					"Lighting Node %s has not completed its fresh Runtime Snapshot reconnect.",
					stageDeviceGateName(device),
				), nil
			}
		}
		return true, "", nil
	}
}

func stageDeviceGateName(device deviceexperience.Device) string {
	if name := strings.TrimSpace(device.DisplayName); name != "" {
		return name
	}
	return device.ID
}
