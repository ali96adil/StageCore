package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const tabletSceneCueType = "TABLET_SCENE"

type tabletSceneActionView struct {
	ActionID      string          `json:"action_id"`
	OrderIndex    int             `json:"order_index"`
	DeviceID      string          `json:"device_id"`
	DisplayName   string          `json:"display_name"`
	CommandType   string          `json:"command_type"`
	CapabilityKey string          `json:"capability_key"`
	ExecutionMode string          `json:"execution_mode"`
	Priority      string          `json:"priority"`
	Parameters    json.RawMessage `json:"parameters"`
	Enabled       bool            `json:"enabled"`
}

type tabletSceneView struct {
	CueID        string                  `json:"cue_id"`
	RevisionID   string                  `json:"revision_id"`
	DisplayLabel string                  `json:"display_label"`
	Name         string                  `json:"name"`
	OrderIndex   int                     `json:"order_index"`
	Enabled      bool                    `json:"enabled"`
	Actions      []tabletSceneActionView `json:"actions"`
}

type tabletAliasBinding struct {
	DeviceID      string
	CapabilityKey string
}

// WithOperatorTabletAuthoring exposes a read-only, operator-friendly projection
// of Tablet Scenes. The source of truth remains the normal revision-backed Cue
// and Action tables; mutations continue through the canonical Cue endpoints.
// This projection only resolves internal target aliases back to device IDs so
// the graphical editor never needs to expose target_ref values or raw config.
func WithOperatorTabletAuthoring(auth *userauth.Service, devices *deviceexperience.Repository, stageStore *store.Store) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil || stageStore == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/tablet-controller/scenes", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
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
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "TABLET_SCENES_UNAVAILABLE"})
				return
			}
			aliases, err := stageStore.ListAliases(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "TABLET_ALIASES_UNAVAILABLE"})
				return
			}
			allDevices, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "TABLET_LIST_FAILED"})
				return
			}

			bindings := tabletAliasBindings(aliases)
			displayNames := make(map[string]string)
			for _, device := range tabletDevices(allDevices) {
				displayNames[device.ID] = device.DisplayName
			}
			scenes := make([]tabletSceneView, 0)
			for _, cue := range cues {
				scene, ok := makeTabletSceneView(cue, bindings, displayNames)
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
			writeJSON(w, http.StatusOK, map[string]any{
				"revision": makeRevisionView(revision),
				"scenes":   scenes,
			})
		}))
	}
}

func tabletAliasBindings(aliases []domain.ProjectDeviceAlias) map[string]tabletAliasBinding {
	bindings := make(map[string]tabletAliasBinding)
	for _, alias := range aliases {
		var config struct {
			DeviceID      string `json:"device_id"`
			CapabilityKey string `json:"capability_key"`
		}
		if json.Unmarshal(alias.ProjectConfig, &config) != nil {
			continue
		}
		config.DeviceID = strings.TrimSpace(config.DeviceID)
		config.CapabilityKey = strings.TrimSpace(config.CapabilityKey)
		if config.DeviceID == "" || !strings.HasPrefix(config.CapabilityKey, "tablet.media.") {
			continue
		}
		bindings[alias.LogicalName] = tabletAliasBinding{DeviceID: config.DeviceID, CapabilityKey: config.CapabilityKey}
	}
	return bindings
}

func makeTabletSceneView(cue domain.Cue, bindings map[string]tabletAliasBinding, displayNames map[string]string) (tabletSceneView, bool) {
	actions := make([]tabletSceneActionView, 0, len(cue.Actions))
	for _, action := range cue.Actions {
		if !strings.HasPrefix(action.CapabilityKey, "tablet.media.") {
			// A mixed Cue is deliberately left to the general Cue editor. Editing
			// it as a Tablet Scene could otherwise discard non-tablet actions.
			if len(cue.Actions) != 0 {
				return tabletSceneView{}, false
			}
			continue
		}
		binding, ok := bindings[action.TargetRef]
		if !ok || binding.CapabilityKey != action.CapabilityKey {
			return tabletSceneView{}, false
		}
		commandType := tabletCommandForCapability(action.CapabilityKey)
		if commandType == "" {
			return tabletSceneView{}, false
		}
		actions = append(actions, tabletSceneActionView{
			ActionID: action.ID, OrderIndex: action.OrderIndex,
			DeviceID: binding.DeviceID, DisplayName: displayNames[binding.DeviceID],
			CommandType: commandType, CapabilityKey: action.CapabilityKey,
			ExecutionMode: action.ExecutionMode, Priority: string(action.PriorityClass),
			Parameters: normalizeRawObject(action.Parameters), Enabled: action.Enabled,
		})
	}
	if cue.CueType != tabletSceneCueType && len(actions) == 0 {
		return tabletSceneView{}, false
	}
	return tabletSceneView{
		CueID: cue.ID, RevisionID: cue.RevisionID, DisplayLabel: cue.DisplayLabel,
		Name: cue.Name, OrderIndex: cue.OrderIndex, Enabled: cue.Enabled, Actions: actions,
	}, true
}

func tabletCommandForCapability(capability string) string {
	switch strings.TrimSpace(capability) {
	case deviceexperience.CapabilityTabletPrepare:
		return deviceexperience.CommandTabletPrepare
	case deviceexperience.CapabilityTabletPlay:
		return deviceexperience.CommandTabletPlay
	case deviceexperience.CapabilityTabletPause:
		return deviceexperience.CommandTabletPause
	case deviceexperience.CapabilityTabletStop:
		return deviceexperience.CommandTabletStop
	case deviceexperience.CapabilityTabletBlackout:
		return deviceexperience.CommandTabletBlackout
	case deviceexperience.CapabilityTabletBlackoutClear:
		return deviceexperience.CommandTabletBlackoutClear
	case deviceexperience.CapabilityTabletSelectMedia:
		return deviceexperience.CommandTabletSelectMedia
	case deviceexperience.CapabilityTabletOverlayPlay:
		return deviceexperience.CommandTabletOverlayPlay
	case deviceexperience.CapabilityTabletOverlayClear:
		return deviceexperience.CommandTabletOverlayClear
	case deviceexperience.CapabilityTabletLiveShow:
		return deviceexperience.CommandTabletLiveShow
	case deviceexperience.CapabilityTabletLiveHide:
		return deviceexperience.CommandTabletLiveHide
	default:
		return ""
	}
}
