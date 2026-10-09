package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	snapshotpkg "github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func WithOperatorLightingCommissioning(
	auth *userauth.Service,
	devices *deviceexperience.Repository,
	runtime *devicechannel.Runtime,
	stageStore *store.Store,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil || runtime == nil || stageStore == nil {
			return
		}

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/lighting-controller/nodes/{device_id}/identify", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			device, err := devices.GetDevice(r.Context(), deviceID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_DEVICE_NOT_FOUND"})
				return
			}
			snapshotID, authorized := lightingCommissioningSnapshot(device, projectID)
			if !authorized {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_DEVICE_NOT_FOUND"})
				return
			}
			if !lightingCommissioningScopeReady(runtime, device, projectID, snapshotID) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_DEVICE_SCOPE_NOT_READY"})
				return
			}
			var input struct {
				Alias      string  `json:"alias"`
				Level      float64 `json:"level"`
				DurationMS int64   `json:"duration_ms"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			var bindings []lightingnode.ProjectBinding
			if device.ProtocolVersion == deviceexperience.ProtocolVersion2 {
				// v2 identity is projectless. Resolve aliases only from the exact
				// Published Snapshot bound to its ACTIVE Hub-owned assignment,
				// never from a newer/unpublished Project draft.
				assigned, readErr := stageStore.GetRuntimeSnapshot(r.Context(), snapshotID)
				if readErr != nil || assigned.ProjectID != projectID || assigned.Status != domain.SnapshotPublished {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_ASSIGNED_SNAPSHOT_INVALID"})
					return
				}
				manifest, decodeErr := snapshotpkg.Decode(assigned.Manifest)
				if decodeErr != nil {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_ASSIGNED_SNAPSHOT_INVALID"})
					return
				}
				bindings = manifest.LightingNodes
			} else {
				project, readErr := stageStore.GetProject(r.Context(), projectID)
				if readErr != nil {
					writeProjectStoreError(w, readErr)
					return
				}
				bindings, err = stageStore.ListLightingNodeBindings(r.Context(), project.CurrentRevisionID)
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_BINDINGS_UNAVAILABLE"})
					return
				}
			}
			resolved, err := lightingnode.ResolveAlias(bindings, strings.TrimSpace(input.Alias))
			if err != nil || resolved.DeviceID != deviceID {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_ALIAS_INVALID"})
				return
			}
			if input.DurationMS == 0 {
				input.DurationMS = 1200
			}
			payload, _ := json.Marshal(lightingnode.IdentifyPayload{
				ChannelKey: resolved.ChannelKey,
				Level:      input.Level,
				DurationMS: input.DurationMS,
			})
			correlationID, _ := stageid.New()
			deadline := time.Now().UTC().Add(10 * time.Second)
			command, err := runtime.Dispatch(r.Context(), deviceexperience.CreateCommandInput{
				ProjectID:         projectID,
				RuntimeSnapshotID: snapshotID,
				DeviceID:          deviceID,
				CommandType:       lightingnode.CommandIdentify,
				Issuer:         session.User.ID,
				CorrelationID:  correlationID,
				IdempotencyKey: "lighting-identify:" + correlationID,
				Priority:       "P2",
				Payload:        payload,
				DeadlineAt:     &deadline,
			})
			if err != nil {
				writeStageDeviceCommandError(w, err)
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"command": command, "alias": input.Alias})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/lighting-controller/nodes/{device_id}/apply-published-config", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			device, err := devices.GetDevice(r.Context(), deviceID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_DEVICE_NOT_FOUND"})
				return
			}
			snapshotID, authorized := lightingCommissioningSnapshot(device, projectID)
			if !authorized {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_DEVICE_NOT_FOUND"})
				return
			}
			if !lightingCommissioningScopeReady(runtime, device, projectID, snapshotID) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_DEVICE_SCOPE_NOT_READY"})
				return
			}
			published, err := stageStore.LatestPublishedRuntimeSnapshotForProject(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LIGHTING_SNAPSHOT_UNAVAILABLE"})
				return
			}
			if published == nil || published.Status != domain.SnapshotPublished {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_PUBLISHED_SNAPSHOT_REQUIRED"})
				return
			}
			if device.ProtocolVersion == deviceexperience.ProtocolVersion2 && published.ID != snapshotID {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_SNAPSHOT_REBIND_REQUIRED"})
				return
			}
			manifest, err := snapshotpkg.Decode(published.Manifest)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_SNAPSHOT_INVALID"})
				return
			}
			var binding *lightingnode.ProjectBinding
			for i := range manifest.LightingNodes {
				if manifest.LightingNodes[i].DeviceID == deviceID {
					copy := manifest.LightingNodes[i]
					binding = &copy
					break
				}
			}
			if binding == nil {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_NODE_NOT_PUBLISHED"})
				return
			}
			payload, _ := json.Marshal(lightingnode.ConfigApplyPayload{Configuration: binding.Configuration})
			correlationID, _ := stageid.New()
			deadline := time.Now().UTC().Add(15 * time.Second)
			command, err := runtime.Dispatch(r.Context(), deviceexperience.CreateCommandInput{
				ProjectID:         projectID,
				DeviceID:          deviceID,
				CommandType:       lightingnode.CommandConfigApply,
				Issuer:            session.User.ID,
				CorrelationID:     correlationID,
				RuntimeSnapshotID: published.ID,
				IdempotencyKey:    "lighting-config-apply:" + published.ID + ":" + deviceID + ":" + correlationID,
				Priority:          "P1",
				Payload:           payload,
				DeadlineAt:        &deadline,
			})
			if err != nil {
				if errors.Is(err, deviceexperience.ErrInvalidState) {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "LIGHTING_CONFIG_APPLY_REJECTED", "detail": err.Error()})
					return
				}
				writeStageDeviceCommandError(w, err)
				return
			}
			hash, _ := lightingnode.ConfigurationHash(binding.Configuration)
			writeJSON(w, http.StatusAccepted, map[string]any{
				"command": command,
				"runtime_snapshot_id": published.ID,
				"expected_configuration_hash": hash,
			})
		}))
	}
}

 
// v1 has a legacy project row; v2 Lighting is project-independent and must
// derive Project/Snapshot ownership from the Hub-owned ACTIVE assignment.
func lightingCommissioningSnapshot(device deviceexperience.Device, projectID string) (string, bool) {
	if !device.Enabled || device.ProfileID != lightingnode.ProfileID || projectID == "" {
		return "", false
	}
	switch device.ProtocolVersion {
	case deviceexperience.ProtocolVersion1:
		return "", device.ProjectID == projectID
	case deviceexperience.ProtocolVersion2:
		a := device.Assignment
		if device.ProjectID != "" || a == nil || a.State != "ACTIVE" ||
			a.ProjectID != projectID || a.RuntimeSnapshotID == "" {
			return "", false
		}
		return a.RuntimeSnapshotID, true
	default:
		return "", false
	}
}

// The current authenticated socket must agree with the committed v2 scope
// before an Operator Identify/config command can reach the physical output.
func lightingCommissioningScopeReady(runtime *devicechannel.Runtime, device deviceexperience.Device, projectID, snapshotID string) bool {
	if device.ProtocolVersion == deviceexperience.ProtocolVersion1 {
		return true
	}
	if runtime == nil || device.Assignment == nil {
		return false
	}
	scope, online := runtime.CurrentV2Scope(device.ID)
	return online && scope.CommandsEnabled &&
		scope.ProjectID == projectID &&
		scope.RuntimeSnapshotID == snapshotID &&
		scope.AssignmentEpoch == device.Assignment.Epoch
}
