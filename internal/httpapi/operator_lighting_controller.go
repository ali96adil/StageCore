package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const lightingSceneCueType = "LIGHTING_SCENE"

var lightingAliasSlugRE = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type lightingChannelView struct {
	Alias         string                   `json:"alias"`
	DeviceID      string                   `json:"device_id"`
	DisplayName   string                   `json:"display_name"`
	ChannelKey    string                   `json:"channel_key"`
	ChannelNumber int                      `json:"channel_number"`
	Kind          lightingnode.ChannelKind `json:"kind"`
	PhysicalZone  string                   `json:"physical_zone,omitempty"`
	MinimumLevel  float64                  `json:"minimum_level"`
	MaximumLevel  float64                  `json:"maximum_level"`
	Inverted      bool                     `json:"inverted"`
}

type lightingNodeView struct {
	DeviceID    string                `json:"device_id"`
	DisplayName string                `json:"display_name"`
	Enabled     bool                  `json:"enabled"`
	Connection  string                `json:"connection,omitempty"`
	Readiness   string                `json:"readiness,omitempty"`
	Channels    []lightingChannelView `json:"channels"`
}

type lightingCueActionRequest struct {
	CommandType   string             `json:"command_type"`
	Levels        map[string]float64 `json:"levels,omitempty"`
	DeviceIDs     []string           `json:"device_ids,omitempty"`
	FadeMS        int64              `json:"fade_ms,omitempty"`
	ExecutionMode string             `json:"execution_mode,omitempty"`
	Priority      string             `json:"priority,omitempty"`
}

type lightingCueActionDescriptor struct {
	DeviceID      string          `json:"device_id"`
	DisplayName   string          `json:"display_name"`
	TargetRef     string          `json:"target_ref"`
	CapabilityKey string          `json:"capability_key"`
	ExecutionMode string          `json:"execution_mode"`
	Priority      string          `json:"priority"`
	Parameters    json.RawMessage `json:"parameters"`
	TimeoutPolicy json.RawMessage `json:"timeout_policy"`
}

type lightingSceneView struct {
	CueID        string             `json:"cue_id"`
	RevisionID   string             `json:"revision_id"`
	DisplayLabel string             `json:"display_label"`
	Name         string             `json:"name"`
	OrderIndex   int                `json:"order_index"`
	Enabled      bool               `json:"enabled"`
	CommandType  string             `json:"command_type"`
	FadeMS       int64              `json:"fade_ms,omitempty"`
	Levels       map[string]float64 `json:"levels,omitempty"`
	DeviceIDs    []string           `json:"device_ids,omitempty"`
}

type lightingTargetBinding struct {
	DeviceID      string
	CapabilityKey string
}

func WithOperatorLightingController(auth *userauth.Service, devices *deviceexperience.Repository, stageStore *store.Store) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil || stageStore == nil {
			return
		}

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/lighting-controller", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			project, err := stageStore.GetProject(r.Context(), projectID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			revision, err := stageStore.GetRevision(r.Context(), project.CurrentRevisionID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			bindings, err := stageStore.ListLightingNodeBindings(r.Context(), revision.ID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_BINDINGS_UNAVAILABLE", "detail": err.Error()})
				return
			}
			allDevices, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_DEVICES_UNAVAILABLE", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"revision": makeRevisionView(revision),
				"nodes":    lightingNodeViews(bindings, allDevices),
			})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/lighting-controller/cue-actions", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			project, err := stageStore.GetProject(r.Context(), projectID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			var input lightingCueActionRequest
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			input.CommandType = strings.TrimSpace(input.CommandType)
			capability := deviceexperience.RequiredCapability(input.CommandType)
			if capability == "" || deviceexperience.CommandTypeForCueCapability(capability) != input.CommandType || !strings.HasPrefix(capability, "lighting.") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_COMMAND_UNSUPPORTED"})
				return
			}

			bindings, err := stageStore.ListLightingNodeBindings(r.Context(), project.CurrentRevisionID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_BINDINGS_UNAVAILABLE", "detail": err.Error()})
				return
			}
			allDevices, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_DEVICES_UNAVAILABLE", "detail": err.Error()})
				return
			}
			deviceMap := make(map[string]deviceexperience.Device)
			for _, device := range allDevices {
				if device.ProfileID == lightingnode.ProfileID && device.Enabled {
					deviceMap[device.ID] = device
				}
			}

			groups, err := lightingCueGroups(bindings, input)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_CUE_INVALID", "detail": err.Error()})
				return
			}
			aliases, err := stageStore.ListAliases(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_TARGETS_UNAVAILABLE", "detail": err.Error()})
				return
			}

			deviceIDs := make([]string, 0, len(groups))
			for deviceID := range groups {
				deviceIDs = append(deviceIDs, deviceID)
			}
			sort.Strings(deviceIDs)
			actions := make([]lightingCueActionDescriptor, 0, len(deviceIDs))
			for _, deviceID := range deviceIDs {
				device, ok := deviceMap[deviceID]
				if !ok {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_DEVICE_UNAVAILABLE", "device_id": deviceID})
					return
				}
				if !containsString(device.Capabilities, capability) {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_CAPABILITY_UNAVAILABLE", "device_id": deviceID, "capability": capability})
					return
				}
				parameters, err := lightingCueParameters(input.CommandType, input.FadeMS, groups[deviceID])
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_CUE_INVALID", "detail": err.Error()})
					return
				}
				if _, err := lightingnode.ResolveCueCommandPayload(bindings, deviceID, input.CommandType, parameters); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_ALIAS_RESOLUTION_FAILED", "detail": err.Error()})
					return
				}
				targetRef, err := ensureLightingTargetAlias(r, stageStore, aliases, projectID, device, capability)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_TARGET_FAILED", "detail": err.Error()})
					return
				}
				actions = append(actions, lightingCueActionDescriptor{
					DeviceID: device.ID, DisplayName: device.DisplayName,
					TargetRef: targetRef, CapabilityKey: capability,
					ExecutionMode: defaultLightingExecutionMode(input.ExecutionMode),
					Priority: defaultLightingPriority(input.Priority, input.CommandType),
					Parameters: parameters,
					TimeoutPolicy: lightingTimeoutPolicy(input.CommandType, input.FadeMS),
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"actions": actions})
		}))

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/lighting-controller/cues", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			project, err := stageStore.GetProject(r.Context(), projectID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			revision, err := stageStore.GetRevision(r.Context(), project.CurrentRevisionID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			cues, err := stageStore.ListCues(r.Context(), revision.ID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_CUES_UNAVAILABLE"})
				return
			}
			aliases, err := stageStore.ListAliases(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_TARGETS_UNAVAILABLE"})
				return
			}
			targets := lightingTargetBindings(aliases)
			scenes := make([]lightingSceneView, 0)
			for _, cue := range cues {
				scene, ok := makeLightingSceneView(cue, targets)
				if ok {
					scenes = append(scenes, scene)
				}
			}
			sort.Slice(scenes, func(i, j int) bool {
				if scenes[i].OrderIndex == scenes[j].OrderIndex {
					return scenes[i].CueID < scenes[j].CueID
				}
				return scenes[i].OrderIndex < scenes[j].OrderIndex
			})
			writeJSON(w, http.StatusOK, map[string]any{"revision": makeRevisionView(revision), "cues": scenes})
		}))
	}
}

func lightingNodeViews(bindings []lightingnode.ProjectBinding, devices []deviceexperience.Device) []lightingNodeView {
	deviceMap := make(map[string]deviceexperience.Device, len(devices))
	for _, device := range devices {
		deviceMap[device.ID] = device
	}
	out := make([]lightingNodeView, 0, len(bindings))
	for _, binding := range bindings {
		device := deviceMap[binding.DeviceID]
		reverse := make(map[string]string, len(binding.Aliases))
		for alias, channelKey := range binding.Aliases {
			reverse[channelKey] = alias
		}
		channels := append([]lightingnode.ChannelConfig(nil), binding.Configuration.Channels...)
		sort.Slice(channels, func(i, j int) bool { return channels[i].ChannelNumber < channels[j].ChannelNumber })
		view := lightingNodeView{DeviceID: binding.DeviceID, DisplayName: device.DisplayName, Enabled: device.Enabled, Channels: make([]lightingChannelView, 0, len(channels))}
		if view.DisplayName == "" {
			view.DisplayName = binding.DeviceID
		}
		if device.Runtime != nil {
			view.Connection = string(device.Runtime.Connection)
			view.Readiness = string(device.Runtime.Readiness)
		}
		for _, channel := range channels {
			if !channel.Enabled || channel.Kind == lightingnode.ChannelUnused {
				continue
			}
			view.Channels = append(view.Channels, lightingChannelView{
				Alias: reverse[channel.ChannelKey], DeviceID: binding.DeviceID, DisplayName: channel.DisplayName,
				ChannelKey: channel.ChannelKey, ChannelNumber: channel.ChannelNumber, Kind: channel.Kind,
				PhysicalZone: channel.PhysicalZone, MinimumLevel: channel.MinimumLevel, MaximumLevel: channel.MaximumLevel,
				Inverted: channel.Inverted,
			})
		}
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DisplayName == out[j].DisplayName {
			return out[i].DeviceID < out[j].DeviceID
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out
}

func lightingCueGroups(bindings []lightingnode.ProjectBinding, input lightingCueActionRequest) (map[string]map[string]float64, error) {
	groups := make(map[string]map[string]float64)
	switch input.CommandType {
	case lightingnode.CommandChannelsSet, lightingnode.CommandChannelsFade:
		if len(input.Levels) == 0 {
			return nil, fmt.Errorf("at least one logical lighting alias is required")
		}
		if input.CommandType == lightingnode.CommandChannelsFade && (input.FadeMS <= 0 || input.FadeMS > 600000) {
			return nil, fmt.Errorf("fade_ms must be within 1..600000")
		}
		aliases := make([]string, 0, len(input.Levels))
		for alias := range input.Levels {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, rawAlias := range aliases {
			alias := strings.TrimSpace(rawAlias)
			level := input.Levels[rawAlias]
			if level < 0 || level > 100 {
				return nil, fmt.Errorf("lighting alias %q level must be within 0..100", alias)
			}
			resolved, err := lightingnode.ResolveAlias(bindings, alias)
			if err != nil {
				return nil, err
			}
			if groups[resolved.DeviceID] == nil {
				groups[resolved.DeviceID] = make(map[string]float64)
			}
			if _, exists := groups[resolved.DeviceID][alias]; exists {
				return nil, fmt.Errorf("duplicate logical lighting alias %q", alias)
			}
			groups[resolved.DeviceID][alias] = level
		}
	case lightingnode.CommandBlackout:
		if input.FadeMS < 0 || input.FadeMS > 600000 {
			return nil, fmt.Errorf("fade_ms must be within 0..600000")
		}
		wanted := uniqueStrings(input.DeviceIDs)
		if len(wanted) == 0 {
			return nil, fmt.Errorf("at least one lighting node is required for blackout")
		}
		known := make(map[string]bool, len(bindings))
		for _, binding := range bindings {
			known[binding.DeviceID] = true
		}
		for _, deviceID := range wanted {
			if !known[deviceID] {
				return nil, fmt.Errorf("lighting node %q is not configured in the current revision", deviceID)
			}
			groups[deviceID] = map[string]float64{}
		}
	default:
		return nil, fmt.Errorf("unsupported lighting Cue command %q", input.CommandType)
	}
	return groups, nil
}

func lightingCueParameters(commandType string, fadeMS int64, levels map[string]float64) (json.RawMessage, error) {
	var value any
	switch commandType {
	case lightingnode.CommandChannelsSet:
		value = lightingnode.CueSetPayload{Aliases: levels}
	case lightingnode.CommandChannelsFade:
		value = lightingnode.CueFadePayload{FadeMS: fadeMS, Aliases: levels}
	case lightingnode.CommandBlackout:
		value = lightingnode.BlackoutPayload{FadeMS: fadeMS}
	default:
		return nil, fmt.Errorf("unsupported lighting Cue command %q", commandType)
	}
	return json.Marshal(value)
}

func lightingTimeoutPolicy(commandType string, fadeMS int64) json.RawMessage {
	timeoutMS := int64(5000)
	if (commandType == lightingnode.CommandChannelsFade || commandType == lightingnode.CommandBlackout) && fadeMS > 0 {
		timeoutMS = fadeMS + 3000
	}
	raw, _ := json.Marshal(map[string]int64{"timeout_ms": timeoutMS})
	return raw
}

func ensureLightingTargetAlias(r *http.Request, stageStore *store.Store, aliases []domain.ProjectDeviceAlias, projectID string, device deviceexperience.Device, capability string) (string, error) {
	for _, alias := range aliases {
		if alias.LogicalType != devicechannel.StageDeviceLogicalType {
			continue
		}
		var cfg struct {
			DeviceID      string `json:"device_id"`
			CapabilityKey string `json:"capability_key"`
		}
		if json.Unmarshal(alias.ProjectConfig, &cfg) == nil && strings.TrimSpace(cfg.DeviceID) == device.ID && strings.TrimSpace(cfg.CapabilityKey) == capability {
			return alias.LogicalName, nil
		}
	}
	config, err := json.Marshal(map[string]string{"device_id": device.ID, "capability_key": capability})
	if err != nil {
		return "", err
	}
	logicalName := "lighting." + lightingAliasSlug(device.ID) + "." + lightingAliasSlug(strings.TrimPrefix(capability, "lighting."))
	created, err := stageStore.CreateAlias(r.Context(), domain.ProjectDeviceAlias{
		ProjectID: projectID, LogicalName: logicalName, LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef: device.ID, GroupName: device.GroupName, ProjectConfig: config,
	})
	if err != nil {
		return "", err
	}
	return created.LogicalName, nil
}

func lightingAliasSlug(value string) string {
	value = lightingAliasSlugRE.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, "-._")
	if value == "" {
		return "device"
	}
	return value
}

func defaultLightingExecutionMode(value string) string {
	switch strings.TrimSpace(value) {
	case "SEQUENTIAL", "PARALLEL", "PARALLEL_BARRIER":
		return strings.TrimSpace(value)
	default:
		return "PARALLEL_BARRIER"
	}
}

func defaultLightingPriority(value, commandType string) string {
	switch strings.TrimSpace(value) {
	case "P0", "P1", "P2", "P3":
		return strings.TrimSpace(value)
	}
	if commandType == lightingnode.CommandBlackout {
		return "P0"
	}
	return "P1"
}

func lightingTargetBindings(aliases []domain.ProjectDeviceAlias) map[string]lightingTargetBinding {
	out := make(map[string]lightingTargetBinding)
	for _, alias := range aliases {
		if alias.LogicalType != devicechannel.StageDeviceLogicalType {
			continue
		}
		var cfg struct {
			DeviceID      string `json:"device_id"`
			CapabilityKey string `json:"capability_key"`
		}
		if json.Unmarshal(alias.ProjectConfig, &cfg) != nil {
			continue
		}
		cfg.DeviceID = strings.TrimSpace(cfg.DeviceID)
		cfg.CapabilityKey = strings.TrimSpace(cfg.CapabilityKey)
		if cfg.DeviceID == "" || !strings.HasPrefix(cfg.CapabilityKey, "lighting.") {
			continue
		}
		out[alias.LogicalName] = lightingTargetBinding{DeviceID: cfg.DeviceID, CapabilityKey: cfg.CapabilityKey}
	}
	return out
}

func makeLightingSceneView(cue domain.Cue, targets map[string]lightingTargetBinding) (lightingSceneView, bool) {
	view := lightingSceneView{
		CueID: cue.ID, RevisionID: cue.RevisionID, DisplayLabel: cue.DisplayLabel,
		Name: cue.Name, OrderIndex: cue.OrderIndex, Enabled: cue.Enabled,
		Levels: map[string]float64{},
	}
	commandType := ""
	fadeDefined := false
	devices := make(map[string]bool)
	for _, action := range cue.Actions {
		currentCommand := deviceexperience.CommandTypeForCueCapability(action.CapabilityKey)
		if currentCommand == "" || !strings.HasPrefix(action.CapabilityKey, "lighting.") {
			if len(cue.Actions) != 0 {
				return lightingSceneView{}, false
			}
			continue
		}
		target, ok := targets[action.TargetRef]
		if !ok || target.CapabilityKey != action.CapabilityKey {
			return lightingSceneView{}, false
		}
		if commandType == "" {
			commandType = currentCommand
		} else if commandType != currentCommand {
			return lightingSceneView{}, false
		}
		devices[target.DeviceID] = true
		switch currentCommand {
		case lightingnode.CommandChannelsSet:
			var payload lightingnode.CueSetPayload
			if json.Unmarshal(action.Parameters, &payload) != nil {
				return lightingSceneView{}, false
			}
			for alias, level := range payload.Aliases {
				if _, exists := view.Levels[alias]; exists {
					return lightingSceneView{}, false
				}
				view.Levels[alias] = level
			}
		case lightingnode.CommandChannelsFade:
			var payload lightingnode.CueFadePayload
			if json.Unmarshal(action.Parameters, &payload) != nil || payload.FadeMS <= 0 {
				return lightingSceneView{}, false
			}
			if fadeDefined && view.FadeMS != payload.FadeMS {
				return lightingSceneView{}, false
			}
			view.FadeMS = payload.FadeMS
			fadeDefined = true
			for alias, level := range payload.Aliases {
				if _, exists := view.Levels[alias]; exists {
					return lightingSceneView{}, false
				}
				view.Levels[alias] = level
			}
		case lightingnode.CommandBlackout:
			var payload lightingnode.BlackoutPayload
			if json.Unmarshal(action.Parameters, &payload) != nil || payload.FadeMS < 0 {
				return lightingSceneView{}, false
			}
			if fadeDefined && view.FadeMS != payload.FadeMS {
				return lightingSceneView{}, false
			}
			view.FadeMS = payload.FadeMS
			fadeDefined = true
		default:
			return lightingSceneView{}, false
		}
	}
	if commandType == "" {
		return lightingSceneView{}, false
	}
	view.CommandType = commandType
	for deviceID := range devices {
		view.DeviceIDs = append(view.DeviceIDs, deviceID)
	}
	sort.Strings(view.DeviceIDs)
	if len(view.Levels) == 0 {
		view.Levels = nil
	}
	return view, true
}
