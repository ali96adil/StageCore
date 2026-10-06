package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type stageLaserCueActionRequest struct {
	DeviceID       string  `json:"device_id"`
	CommandType    string  `json:"command_type"`
	FrequencyHz    float64 `json:"frequency_hz,omitempty"`
	DurationMS     int64   `json:"duration_ms,omitempty"`
	ExecutionMode  string  `json:"execution_mode,omitempty"`
	Priority       string  `json:"priority,omitempty"`
}

type stageLaserCueActionDescriptor struct {
	DeviceID      string          `json:"device_id"`
	DisplayName   string          `json:"display_name"`
	TargetRef     string          `json:"target_ref"`
	CapabilityKey string          `json:"capability_key"`
	ExecutionMode string          `json:"execution_mode"`
	Priority      string          `json:"priority"`
	Parameters    json.RawMessage `json:"parameters"`
	TimeoutPolicy json.RawMessage `json:"timeout_policy"`
}

func WithOperatorStageLaserController(
	auth *userauth.Service,
	devices *deviceexperience.Repository,
	stageStore *store.Store,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil || stageStore == nil {
			return
		}

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/stagelaser-controller", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if _, err := stageStore.GetProject(r.Context(), projectID); err != nil {
				writeProjectStoreError(w, err)
				return
			}
			items, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGELASER_DEVICES_UNAVAILABLE"})
				return
			}
			out := make([]deviceexperience.Device, 0)
			for _, device := range items {
				if stageLaserAuthoringDevice(device, projectID) {
					out = append(out, device)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"devices": out})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/stagelaser-controller/cue-actions", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			var input stageLaserCueActionRequest
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			input.DeviceID = strings.TrimSpace(input.DeviceID)
			input.CommandType = strings.TrimSpace(input.CommandType)
			if input.DeviceID == "" || input.CommandType == "" ||
				!stagelaser.CueSafeCommand(input.CommandType) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGELASER_CUE_COMMAND_UNSUPPORTED"})
				return
			}

			device, err := devices.GetDevice(r.Context(), input.DeviceID)
			if err != nil || !stageLaserAuthoringDevice(device, projectID) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGELASER_DEVICE_UNAVAILABLE"})
				return
			}
			capability := stagelaser.CommandCapability(input.CommandType)
			if capability == "" || !containsString(device.Capabilities, capability) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "STAGELASER_CAPABILITY_UNAVAILABLE",
					"capability": capability,
				})
				return
			}

			parameters, err := stageLaserCueParameters(input)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGELASER_CUE_INVALID", "detail": err.Error()})
				return
			}
			aliases, err := stageStore.ListAliases(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGELASER_TARGETS_UNAVAILABLE"})
				return
			}
			targetRef, err := ensureStageLaserTargetAlias(r, stageStore, aliases, projectID, device)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGELASER_TARGET_FAILED", "detail": err.Error()})
				return
			}
			action := stageLaserCueActionDescriptor{
				DeviceID: device.ID,
				DisplayName: device.DisplayName,
				TargetRef: targetRef,
				CapabilityKey: capability,
				ExecutionMode: defaultStageLaserExecutionMode(input.ExecutionMode),
				Priority: defaultStageLaserPriority(input.Priority, input.CommandType),
				Parameters: parameters,
				TimeoutPolicy: json.RawMessage(`{}`),
			}
			writeJSON(w, http.StatusOK, map[string]any{"actions": []stageLaserCueActionDescriptor{action}})
		}))
	}
}

func stageLaserAuthoringDevice(device deviceexperience.Device, projectID string) bool {
	return device.Enabled &&
		device.ProtocolVersion == deviceexperience.ProtocolVersion2 &&
		device.Kind == deviceexperience.DeviceGeneric &&
		device.ProfileID == stagelaser.ProfileID &&
		device.ProjectID == "" &&
		device.Assignment != nil &&
		device.Assignment.State == "ACTIVE" &&
		device.Assignment.ProjectID == strings.TrimSpace(projectID) &&
		device.Assignment.RuntimeSnapshotID != ""
}

func stageLaserCueParameters(input stageLaserCueActionRequest) (json.RawMessage, error) {
	switch input.CommandType {
	case stagelaser.CommandSetOn:
		return json.Marshal(map[string]string{"state": string(stagelaser.StateOn)})
	case stagelaser.CommandSetOff:
		return json.Marshal(map[string]string{"state": string(stagelaser.StateOff)})
	case stagelaser.CommandFlashStart:
		payload := stagelaser.FlashStartPayload{
			FrequencyHz: input.FrequencyHz,
			DurationMS: input.DurationMS,
		}
		if err := stagelaser.ValidateFlashStartPayload(payload, stagelaser.DefaultMechanicalLimits()); err != nil {
			return nil, err
		}
		return json.Marshal(payload)
	case stagelaser.CommandArm, stagelaser.CommandDisarm,
		stagelaser.CommandFlashStop, stagelaser.CommandSafeOff:
		return stagelaser.CanonicalEmptyPayload(), nil
	default:
		return nil, fmt.Errorf("command %s is not Cue-safe", input.CommandType)
	}
}

func ensureStageLaserTargetAlias(
	r *http.Request,
	stageStore *store.Store,
	aliases []domain.ProjectDeviceAlias,
	projectID string,
	device deviceexperience.Device,
) (string, error) {
	for _, alias := range aliases {
		if alias.LogicalType != devicechannel.StageDeviceLogicalType {
			continue
		}
		var cfg struct {
			DeviceID      string `json:"device_id"`
			CapabilityKey string `json:"capability_key,omitempty"`
		}
		if json.Unmarshal(alias.ProjectConfig, &cfg) == nil &&
			strings.TrimSpace(cfg.DeviceID) == device.ID &&
			strings.TrimSpace(cfg.CapabilityKey) == "" {
			return alias.LogicalName, nil
		}
	}
	config, err := json.Marshal(map[string]string{"device_id": device.ID})
	if err != nil {
		return "", err
	}
	created, err := stageStore.CreateAlias(r.Context(), domain.ProjectDeviceAlias{
		ProjectID: projectID,
		LogicalName: "stagelaser." + lightingAliasSlug(device.ID),
		LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef: device.ID,
		GroupName: device.GroupName,
		ProjectConfig: config,
	})
	if err != nil {
		return "", err
	}
	return created.LogicalName, nil
}

func defaultStageLaserExecutionMode(value string) string {
	switch strings.TrimSpace(value) {
	case "SEQUENTIAL", "PARALLEL", "PARALLEL_BARRIER":
		return strings.TrimSpace(value)
	default:
		return "PARALLEL_BARRIER"
	}
}

func defaultStageLaserPriority(value, commandType string) string {
	switch strings.TrimSpace(value) {
	case "P0", "P1", "P2", "P3":
		return strings.TrimSpace(value)
	}
	switch commandType {
	case stagelaser.CommandDisarm, stagelaser.CommandFlashStop, stagelaser.CommandSafeOff:
		return "P0"
	default:
		return "P1"
	}
}
