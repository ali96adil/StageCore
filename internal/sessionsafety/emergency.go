package sessionsafety

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

const emergencyWait = 5 * time.Second

type emergencyDeviceRepository interface {
	ListDevices(context.Context, string) ([]deviceexperience.Device, error)
	GetCommand(context.Context, string) (deviceexperience.DeviceCommand, error)
}

type EmergencyDomainReport struct {
	Domain    string   `json:"domain"`
	Status    string   `json:"status"`
	Attempted int      `json:"attempted"`
	Completed int      `json:"completed"`
	Details   []string `json:"details,omitempty"`
}

type EmergencyReport struct {
	Enabled          bool                  `json:"enabled"`
	Lighting         EmergencyDomainReport `json:"lighting"`
	Tablets          EmergencyDomainReport `json:"tablets"`
	NativeVisual     EmergencyDomainReport `json:"native_visual"`
	Audio            EmergencyDomainReport `json:"audio"`
	ExternalAdapters EmergencyDomainReport `json:"external_adapters"`
}

func NewManagedOutputBlackout(
	stageStore *store.Store,
	devices emergencyDeviceRepository,
	dispatcher lightingDispatcher,
	executor capability.Executor,
) func(context.Context, domain.Session, contracts.CommandEnvelope, bool) (json.RawMessage, error) {
	return func(ctx context.Context, session domain.Session, command contracts.CommandEnvelope, enabled bool) (json.RawMessage, error) {
		report, err := SetManagedOutputBlackout(ctx, stageStore, devices, dispatcher, executor, session, command, enabled)
		payload, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode Emergency Blackout report: %w", marshalErr)
		}
		return payload, err
	}
}

func SetManagedOutputBlackout(
	ctx context.Context,
	stageStore *store.Store,
	devices emergencyDeviceRepository,
	dispatcher lightingDispatcher,
	executor capability.Executor,
	session domain.Session,
	command contracts.CommandEnvelope,
	enabled bool,
) (EmergencyReport, error) {
	report := EmergencyReport{
		Enabled: enabled,
		Lighting: EmergencyDomainReport{Domain: "LIGHTING", Status: "NOT_CONFIGURED"},
		Tablets: EmergencyDomainReport{Domain: "TABLET", Status: "NOT_CONFIGURED"},
		NativeVisual: EmergencyDomainReport{Domain: "NATIVE_VISUAL", Status: "NOT_CONFIGURED"},
		Audio: EmergencyDomainReport{
			Domain: "AUDIO", Status: "UNCHANGED_BY_DESIGN",
			Details: []string{"Emergency Blackout never stops or changes audio."},
		},
		ExternalAdapters: EmergencyDomainReport{
			Domain: "EXTERNAL_ADAPTERS", Status: "UNCHANGED_BY_DESIGN",
			Details: []string{"External VDMX/OSC adapters are not changed unless they expose a managed StageCore safe-state capability."},
		},
	}
	if stageStore == nil || devices == nil || dispatcher == nil || executor == nil {
		return report, fmt.Errorf("managed-output emergency safety is unavailable")
	}
	runtimeSnapshot, err := stageStore.GetRuntimeSnapshot(ctx, session.RuntimeSnapshotID)
	if err != nil {
		return report, fmt.Errorf("load Session Runtime Snapshot: %w", err)
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return report, fmt.Errorf("decode Session Runtime Snapshot: %w", err)
	}

	var failures []string
	if enabled {
		report.Lighting.Attempted = uniqueLightingNodeCount(manifest)
		if report.Lighting.Attempted > 0 {
			if err := BlackoutLighting(ctx, stageStore, devices, dispatcher, session, command); err != nil {
				report.Lighting.Status = "FAILED"
				report.Lighting.Details = []string{err.Error()}
				failures = append(failures, "lighting: "+err.Error())
			} else {
				report.Lighting.Status = "COMPLETED"
				report.Lighting.Completed = report.Lighting.Attempted
			}
		}
	} else if uniqueLightingNodeCount(manifest) > 0 {
		report.Lighting.Status = "MANUAL_RECOVERY_REQUIRED"
		report.Lighting.Attempted = uniqueLightingNodeCount(manifest)
		report.Lighting.Details = []string{"Lighting remains at blackout; restore it only with an explicit Lighting Cue or operator action."}
	}

	tabletReport, tabletErr := setTabletBlackout(ctx, devices, dispatcher, session, command, enabled)
	report.Tablets = tabletReport
	if tabletErr != nil {
		failures = append(failures, "tablets: "+tabletErr.Error())
	}

	visualReport, visualErr := setNativeVisualBlackout(ctx, stageStore, executor, manifest, session, command, enabled)
	report.NativeVisual = visualReport
	if visualErr != nil {
		failures = append(failures, "native visual: "+visualErr.Error())
	}

	if len(failures) > 0 {
		return report, fmt.Errorf("emergency safe-state partial failure: %s", strings.Join(failures, "; "))
	}
	return report, nil
}

func uniqueLightingNodeCount(manifest snapshot.Manifest) int {
	seen := map[string]struct{}{}
	for _, binding := range manifest.LightingNodes {
		if id := strings.TrimSpace(binding.DeviceID); id != "" {
			seen[id] = struct{}{}
		}
	}
	return len(seen)
}

func setTabletBlackout(
	ctx context.Context,
	devices emergencyDeviceRepository,
	dispatcher lightingDispatcher,
	session domain.Session,
	command contracts.CommandEnvelope,
	enabled bool,
) (EmergencyDomainReport, error) {
	report := EmergencyDomainReport{Domain: "TABLET", Status: "NOT_CONFIGURED"}
	all, err := devices.ListDevices(ctx, session.ProjectID)
	if err != nil {
		report.Status = "FAILED"
		report.Details = []string{err.Error()}
		return report, err
	}
	commandType := deviceexperience.CommandTabletBlackout
	if !enabled {
		commandType = deviceexperience.CommandTabletBlackoutClear
	}
	capabilityKey := deviceexperience.RequiredCapability(commandType)
	targets := make([]deviceexperience.Device, 0)
	for _, device := range all {
		if !device.Enabled || device.Kind != deviceexperience.DeviceTabletPlayer {
			continue
		}
		if device.ProtocolVersion == deviceexperience.ProtocolVersion2 {
			if device.Assignment == nil || device.Assignment.State != "ACTIVE" ||
				device.Assignment.ProjectID != session.ProjectID ||
				device.Assignment.RuntimeSnapshotID != session.RuntimeSnapshotID {
				continue
			}
		} else if device.ProjectID != session.ProjectID {
			continue
		}
		report.Attempted++
		if !hasCapability(device.Capabilities, capabilityKey) {
			report.Details = append(report.Details, device.ID+": required "+capabilityKey+" capability is unavailable")
			continue
		}
		targets = append(targets, device)
	}
	if report.Attempted == 0 {
		return report, nil
	}
	if len(report.Details) > 0 {
		report.Status = "FAILED"
		return report, fmt.Errorf("%s", strings.Join(report.Details, "; "))
	}

	deadline := time.Now().UTC().Add(emergencyWait)
	pending := map[string]string{}
	for _, device := range targets {
		dispatched, err := dispatcher.Dispatch(ctx, deviceexperience.CreateCommandInput{
			ProjectID: session.ProjectID,
			SessionID: session.ID,
			DeviceID: device.ID,
			CommandType: commandType,
			Issuer: "hub.runtime_control",
			CorrelationID: command.CorrelationID,
			CausationID: command.CommandID,
			RuntimeSnapshotID: session.RuntimeSnapshotID,
			Priority: "P0",
			IdempotencyKey: "emergency-blackout:" + command.CommandID + ":" + commandType + ":" + device.ID,
			Payload: json.RawMessage(`{}`),
			DeadlineAt: &deadline,
		})
		if err != nil {
			report.Status = "FAILED"
			report.Details = append(report.Details, device.ID+": "+err.Error())
			continue
		}
		pending[dispatched.Envelope.CommandID] = device.ID
	}
	if len(report.Details) > 0 {
		return report, fmt.Errorf("%s", strings.Join(report.Details, "; "))
	}
	if err := waitEmergencyDeviceCommands(ctx, devices, pending, deadline, &report); err != nil {
		report.Status = "FAILED"
		return report, err
	}
	report.Status = "COMPLETED"
	return report, nil
}

func waitEmergencyDeviceCommands(
	ctx context.Context,
	commands emergencyDeviceRepository,
	pending map[string]string,
	deadline time.Time,
	report *EmergencyDomainReport,
) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for len(pending) > 0 {
		for commandID, deviceID := range pending {
			current, err := commands.GetCommand(ctx, commandID)
			if err != nil {
				report.Details = append(report.Details, deviceID+": "+err.Error())
				return err
			}
			switch current.Status {
			case contracts.CommandCompleted:
				report.Completed++
				delete(pending, commandID)
			case contracts.CommandAccepted:
			case contracts.CommandRejected, contracts.CommandFailed, contracts.CommandTimedOut, contracts.CommandCancelled:
				detail := fmt.Sprintf("%s ended with %s", deviceID, current.Status)
				report.Details = append(report.Details, detail)
				return fmt.Errorf("%s", detail)
			default:
				detail := fmt.Sprintf("%s has invalid status %s", deviceID, current.Status)
				report.Details = append(report.Details, detail)
				return fmt.Errorf(detail)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		if time.Now().UTC().After(deadline) {
			return fmt.Errorf("tablet blackout confirmation timed out")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return nil
}

func setNativeVisualBlackout(
	ctx context.Context,
	stageStore *store.Store,
	executor capability.Executor,
	manifest snapshot.Manifest,
	session domain.Session,
	command contracts.CommandEnvelope,
	enabled bool,
) (EmergencyDomainReport, error) {
	report := EmergencyDomainReport{Domain: "NATIVE_VISUAL", Status: "NOT_CONFIGURED"}
	for _, target := range manifest.Targets {
		if !strings.EqualFold(strings.TrimSpace(target.LogicalType), companion.MachineRoleLogicalType) {
			continue
		}
		var cfg struct {
			MachineRoleID string `json:"machine_role_id"`
		}
		if json.Unmarshal(target.Configuration, &cfg) != nil || strings.TrimSpace(cfg.MachineRoleID) == "" {
			continue
		}
		role, err := stageStore.GetMachineRole(ctx, strings.TrimSpace(cfg.MachineRoleID))
		if err != nil || !nativeVisualTargetManaged(manifest, target, role) {
			continue
		}
		report.Attempted++
		parameters, _ := json.Marshal(map[string]any{"contract_version": visualengine.ContractVersion1, "enabled": enabled})
		result := executor.Execute(ctx, capability.Request{
			ExecutionID: "emergency-blackout:" + command.CommandID + ":" + role.ID,
			ProjectID: session.ProjectID,
			SessionID: session.ID,
			RuntimeSnapshotID: session.RuntimeSnapshotID,
			Issuer: "hub.runtime_control",
			CausationID: command.CommandID,
			Capability: visualengine.CapabilityBlackout,
			Target: &capability.Target{
				AliasID: target.AliasID,
				Ref: target.TargetRef,
				LogicalType: target.LogicalType,
				Configuration: target.Configuration,
			},
			Parameters: parameters,
			Priority: "P0",
			TimeoutMS: int64(emergencyWait / time.Millisecond),
			CorrelationID: command.CorrelationID,
		})
		if result.Result != domain.ExecutionCompleted {
			detail := role.RoleKey + ": " + result.ResponseSummary
			if strings.TrimSpace(result.ErrorCode) != "" {
				detail = role.RoleKey + ": " + result.ErrorCode + " · " + result.ResponseSummary
			}
			report.Details = append(report.Details, detail)
			continue
		}
		report.Completed++
	}
	if report.Attempted == 0 {
		return report, nil
	}
	if len(report.Details) > 0 || report.Completed != report.Attempted {
		report.Status = "FAILED"
		return report, fmt.Errorf(strings.Join(report.Details, "; "))
	}
	report.Status = "COMPLETED"
	return report, nil
}

func nativeVisualTargetManaged(manifest snapshot.Manifest, target snapshot.Target, role domain.MachineRole) bool {
	for _, capabilityKey := range role.RequiredCapabilities {
		if visualengine.IsCapability(capabilityKey) {
			return true
		}
	}
	for _, cue := range manifest.Cues {
		if !cue.Enabled {
			continue
		}
		for _, action := range cue.Actions {
			if action.Enabled && action.TargetRef == target.TargetRef && visualengine.IsCapability(action.CapabilityKey) {
				return true
			}
		}
	}
	for _, output := range manifest.Outputs {
		if output.TargetRef == target.TargetRef && visualengine.IsCapability(output.CapabilityKey) {
			return true
		}
	}
	return false
}

func hasCapability(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

