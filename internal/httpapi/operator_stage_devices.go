package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type runtimeSnapshotDeviceSyncResult struct {
	DeviceID          string `json:"device_id"`
	DisplayName       string `json:"display_name,omitempty"`
	DeviceKind        string `json:"device_kind,omitempty"`
	ProfileID         string `json:"profile_id,omitempty"`
	FromSnapshotID    string `json:"from_runtime_snapshot_id,omitempty"`
	ToSnapshotID      string `json:"to_runtime_snapshot_id"`
	AssignmentEpoch   int64  `json:"assignment_epoch,omitempty"`
	Status            string `json:"status"`
	Detail            string `json:"detail,omitempty"`
}

// WithOperatorStageDevices exposes the Phase 4 Operator surface for connected
// tablets/displays/render nodes, live-video source configuration and the
// network cockpit. Runtime commands are authorized separately from project
// configuration edits, preserving the existing StageCore RBAC boundary.
func WithOperatorStageDevices(
	auth *userauth.Service,
	devices *deviceexperience.Repository,
	runtime *devicechannel.Runtime,
	stageStore *store.Store,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || devices == nil || runtime == nil || stageStore == nil {
			return
		}

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/stage-devices", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if projectID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_PROJECT_REQUIRED"})
				return
			}
			items, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "STAGE_DEVICE_LIST_FAILED", "detail": err.Error()})
				return
			}
			active := make([]deviceexperience.Device, 0, len(items))
			for _, item := range items {
				if item.Enabled {
					active = append(active, item)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{"devices": active})
		}))


		s.mux.HandleFunc("PUT /api/v1/projects/{project_id}/stage-devices/{device_id}/show-requirement", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			if projectID == "" || deviceID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_SHOW_REQUIREMENT_SCOPE_REQUIRED"})
				return
			}
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			var input struct {
				RequiredForShow *bool `json:"required_for_show"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			if input.RequiredForShow == nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_SHOW_REQUIREMENT_REQUIRED"})
				return
			}
			assignment, err := devices.SetRequiredForShow(r.Context(), deviceID, projectID, *input.RequiredForShow)
			if err != nil {
				status := http.StatusConflict
				if errors.Is(err, deviceexperience.ErrInvalidDevice) {
					status = http.StatusBadRequest
				}
				writeJSON(w, status, map[string]any{"error": "STAGE_DEVICE_SHOW_REQUIREMENT_UPDATE_FAILED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"assignment": assignment})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/stage-devices/sync-runtime-snapshot", withPermission(auth, userauth.PermissionSnapshotPublish, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if userauth.Authorize(session.User.Role, userauth.PermissionProjectEdit) != nil ||
				userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SYNC_PERMISSION_REQUIRED"})
				return
			}
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if projectID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SYNC_PROJECT_REQUIRED"})
				return
			}
			if active, err := stageStore.ActiveSessionForProject(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SYNC_SESSION_CHECK_FAILED"})
				return
			} else if active != nil {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "STAGE_DEVICE_SNAPSHOT_SYNC_SESSION_ACTIVE",
					"detail": "End the active Session before synchronizing Stage Devices to a new Runtime Snapshot.",
				})
				return
			}
			var input struct {
				RuntimeSnapshotID string `json:"runtime_snapshot_id"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			input.RuntimeSnapshotID = strings.TrimSpace(input.RuntimeSnapshotID)
			snapshot, err := stageStore.GetRuntimeSnapshot(r.Context(), input.RuntimeSnapshotID)
			if err != nil || snapshot.ProjectID != projectID || snapshot.Status != domain.SnapshotPublished {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SYNC_TARGET_INVALID"})
				return
			}
			var manifest struct {
				LightingNodes []struct {
					DeviceID string `json:"device_id"`
				} `json:"lighting_nodes"`
				Targets []struct {
					LogicalType string          `json:"logical_type"`
					Configuration json.RawMessage `json:"configuration"`
				} `json:"targets"`
			}
			if err := json.Unmarshal(snapshot.Manifest, &manifest); err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SYNC_MANIFEST_INVALID"})
				return
			}
			targetLighting := make(map[string]bool, len(manifest.LightingNodes))
			for _, binding := range manifest.LightingNodes {
				if deviceID := strings.TrimSpace(binding.DeviceID); deviceID != "" {
					targetLighting[deviceID] = true
				}
			}
			targetStageLasers := make(map[string]bool)
			for _, target := range manifest.Targets {
				if !strings.EqualFold(strings.TrimSpace(target.LogicalType), stagelaser.LogicalTargetType) {
					continue
				}
				var config struct {
					DeviceID string `json:"device_id"`
				}
				if json.Unmarshal(target.Configuration, &config) != nil {
					continue
				}
				if deviceID := strings.TrimSpace(config.DeviceID); deviceID != "" {
					targetStageLasers[deviceID] = true
				}
			}

			token, ok := browserSessionToken(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
				return
			}
			csrf := r.Header.Get(csrfHeader)
			actor := session.User.ID
			reauthorize := func(ctx context.Context) error {
				next, err := auth.ValidateCSRF(ctx, token, csrf)
				if err != nil {
					return err
				}
				if next.User.ID != actor {
					return userauth.ErrForbidden
				}
				if err := userauth.Authorize(next.User.Role, userauth.PermissionSnapshotPublish); err != nil {
					return err
				}
				if err := userauth.Authorize(next.User.Role, userauth.PermissionProjectEdit); err != nil {
					return err
				}
				return userauth.Authorize(next.User.Role, userauth.PermissionCompanionPair)
			}

			items, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SYNC_INVENTORY_FAILED"})
				return
			}
			// A failed Lighting rebind can intentionally leave the node UNASSIGNED.
			// Include target Lighting identities from global Hub inventory so a
			// retry can resume from that fail-closed state.
			seen := make(map[string]bool, len(items))
			for _, device := range items {
				seen[device.ID] = true
			}
			if len(targetLighting) != 0 {
				global, globalErr := devices.ListDevices(r.Context(), "")
				if globalErr != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SYNC_GLOBAL_INVENTORY_FAILED"})
					return
				}
				for _, device := range global {
					if !seen[device.ID] && targetLighting[device.ID] {
						items = append(items, device)
						seen[device.ID] = true
					}
				}
			}
			results := make([]runtimeSnapshotDeviceSyncResult, 0)
			complete := true
			for _, device := range items {
				if !device.Enabled || device.ProtocolVersion != deviceexperience.ProtocolVersion2 ||
					device.Assignment == nil {
					continue
				}
				assignment := *device.Assignment
				isTablet := device.Kind == deviceexperience.DeviceTabletPlayer &&
					device.ProfileID == deviceexperience.TabletPlayerProfileID
				isTargetLighting := device.ProfileID == lightingnode.ProfileID && targetLighting[device.ID]
				isTargetStageLaser := device.Kind == deviceexperience.DeviceGeneric &&
					device.ProfileID == stagelaser.ProfileID && targetStageLasers[device.ID]
				if !isTablet && !isTargetLighting && !isTargetStageLaser {
					continue
				}
				if (isTablet || isTargetStageLaser) && assignment.ProjectID != projectID {
					continue
				}
				result := runtimeSnapshotDeviceSyncResult{
					DeviceID: device.ID, DisplayName: device.DisplayName,
					DeviceKind: string(device.Kind), ProfileID: device.ProfileID,
					FromSnapshotID: assignment.RuntimeSnapshotID,
					ToSnapshotID: input.RuntimeSnapshotID,
					AssignmentEpoch: assignment.Epoch,
					Status: "FAILED",
				}

				scope, scopeReady := runtime.CurrentV2Scope(device.ID)
				if assignment.State == "ACTIVE" &&
					assignment.RuntimeSnapshotID == input.RuntimeSnapshotID &&
					scopeReady && scope.ProjectID == projectID &&
					scope.RuntimeSnapshotID == input.RuntimeSnapshotID &&
					scope.AssignmentEpoch == assignment.Epoch && scope.CommandsEnabled {
					result.Status = "ALREADY_SYNCED"
					results = append(results, result)
					continue
				}
				if isTablet && assignment.State == "ACTIVE" &&
					assignment.RuntimeSnapshotID == input.RuntimeSnapshotID {
					if waitForStageDeviceScope(r.Context(), runtime, device.ID, projectID, input.RuntimeSnapshotID, assignment.Epoch, 12*time.Second) {
						result.Status = "SYNCED"
						results = append(results, result)
						continue
					}
					result.Detail = "Tablet authority is committed to the Published Runtime Snapshot but its fresh reconnect is not READY."
					complete = false
					results = append(results, result)
					continue
				}
				if isTargetStageLaser && assignment.State == "ACTIVE" &&
					assignment.RuntimeSnapshotID == input.RuntimeSnapshotID {
					if waitForStageDeviceScope(r.Context(), runtime, device.ID, projectID, input.RuntimeSnapshotID, assignment.Epoch, 12*time.Second) {
						result.Status = "SYNCED"
						results = append(results, result)
						continue
					}
					result.Detail = "StageLaser authority is committed to the Published Runtime Snapshot but its fresh reconnect is not READY."
					complete = false
					results = append(results, result)
					continue
				}

				switch {
				case device.Kind == deviceexperience.DeviceTabletPlayer &&
					device.ProfileID == deviceexperience.TabletPlayerProfileID:
					if assignment.State != "ACTIVE" || assignment.RuntimeSnapshotID == "" {
						result.Detail = "Tablet is not ACTIVE on an authoritative source Runtime Snapshot."
						complete = false
						results = append(results, result)
						continue
					}
					record, assignErr := runtime.ExecuteTabletAssignmentAuthorized(
						r.Context(),
						deviceexperience.TabletAssignmentInput{
							DeviceID: device.ID,
							ExpectedProjectID: projectID,
							ExpectedRuntimeSnapshotID: assignment.RuntimeSnapshotID,
							TargetProjectID: projectID,
							TargetRuntimeSnapshotID: input.RuntimeSnapshotID,
							ExpectedEpoch: assignment.Epoch,
						},
						actor,
						reauthorize,
					)
					if assignErr != nil {
						result.Detail = assignErr.Error()
						complete = false
						results = append(results, result)
						continue
					}
					result.AssignmentEpoch = record.ToEpoch
					if !waitForStageDeviceScope(r.Context(), runtime, device.ID, projectID, input.RuntimeSnapshotID, record.ToEpoch, 12*time.Second) {
						result.Detail = "Tablet assignment committed but fresh Runtime Snapshot reconnect did not become READY in time."
						complete = false
						results = append(results, result)
						continue
					}
					result.Status = "SYNCED"
					results = append(results, result)

				case isTargetStageLaser:
					if assignment.State != "ACTIVE" || assignment.ProjectID != projectID ||
						assignment.RuntimeSnapshotID == "" {
						result.Detail = "StageLaser is not ACTIVE on an authoritative source Runtime Snapshot. Assign it from Devices first."
						complete = false
						results = append(results, result)
						continue
					}
					record, assignErr := runtime.ExecuteStageLaserAssignmentAuthorized(
						r.Context(),
						deviceexperience.StageLaserAssignmentInput{
							DeviceID: device.ID,
							ExpectedProjectID: projectID,
							ExpectedRuntimeSnapshotID: assignment.RuntimeSnapshotID,
							TargetProjectID: projectID,
							TargetRuntimeSnapshotID: input.RuntimeSnapshotID,
							ExpectedEpoch: assignment.Epoch,
						},
						actor,
						reauthorize,
					)
					if assignErr != nil {
						result.Detail = assignErr.Error()
						complete = false
						results = append(results, result)
						continue
					}
					result.AssignmentEpoch = record.ToEpoch
					if !waitForStageDeviceScope(r.Context(), runtime, device.ID, projectID, input.RuntimeSnapshotID, record.ToEpoch, 12*time.Second) {
						result.Detail = "StageLaser safe rebind committed but fresh Runtime Snapshot reconnect did not become READY in time."
						complete = false
						results = append(results, result)
						continue
					}
					result.Status = "SYNCED"
					results = append(results, result)

				case isTargetLighting:
					if assignment.ProjectID != "" && assignment.ProjectID != projectID {
						result.Detail = "Lighting Node is assigned to another Project and will not be moved automatically."
						complete = false
						results = append(results, result)
						continue
					}
					rebind, rebindErr := runtime.RebindLightingSnapshotAuthorized(
						r.Context(), device.ID, projectID, input.RuntimeSnapshotID, actor, reauthorize,
					)
					if rebindErr != nil {
						result.Detail = rebindErr.Error()
						complete = false
						results = append(results, result)
						continue
					}
					if rebind.Reassign != nil {
						result.AssignmentEpoch = rebind.Reassign.ToEpoch
					} else if rebind.Activation != nil {
						result.AssignmentEpoch = rebind.Activation.AssignmentEpoch
					}
					result.Status = "SYNCED"
					results = append(results, result)
				}
			}

			writeJSON(w, http.StatusOK, map[string]any{
				"runtime_snapshot_id": input.RuntimeSnapshotID,
				"complete": complete,
				"results": results,
			})
		}))

		// Device provisioning inventory is not scoped to any Project yet.
		// Only an operator authorized for pairing may discover unassigned v2
		// identities. This endpoint grants NO Assign/Transfer authority.
		// Global v2 physical-device inventory. This is intentionally not scoped
		// to the opened Project: an operator with pairing authority can see the
		// reusable device identity, its Hub-owned assignment, and the current
		// authenticated runtime scope. Read-only; it grants no transfer or
		// command authority.
		s.mux.HandleFunc("GET /api/v1/stage-devices/inventory", withPermission(auth, userauth.PermissionCompanionPair, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			items, err := devices.ListDevices(r.Context(), "")
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_INVENTORY_UNAVAILABLE"})
				return
			}
			type inventoryDevice struct {
				DeviceID      string                           `json:"device_id"`
				DisplayName   string                           `json:"display_name"`
				DeviceKind     deviceexperience.DeviceKind      `json:"device_kind"`
				ProfileID      string                           `json:"profile_id,omitempty"`
				ProtocolVersion string                          `json:"protocol_version"`
				ClientVersion  string                           `json:"client_version,omitempty"`
				Capabilities  []string                         `json:"capabilities,omitempty"`
				Assignment    deviceexperience.AssignmentRecord `json:"assignment"`
				Connection    deviceexperience.ConnectionState `json:"connection_state,omitempty"`
				Readiness     deviceexperience.Readiness       `json:"readiness,omitempty"`
				LiveScope     *devicechannel.V2RuntimeScope    `json:"live_scope,omitempty"`
				Runtime       *deviceexperience.RuntimeState   `json:"runtime,omitempty"`
				Enabled       bool                             `json:"enabled"`
			}
			out := make([]inventoryDevice, 0)
			for _, item := range items {
				if item.ProtocolVersion != deviceexperience.ProtocolVersion2 || item.ProjectID != "" {
					continue
				}
				assignment, err := devices.GetAssignmentRecord(r.Context(), item.ID)
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_ASSIGNMENT_UNAVAILABLE"})
					return
				}
				view := inventoryDevice{
					DeviceID: item.ID, DisplayName: item.DisplayName,
					DeviceKind: item.Kind, ProfileID: item.ProfileID,
					ProtocolVersion: item.ProtocolVersion, ClientVersion: item.ClientVersion,
					Capabilities: append([]string(nil), item.Capabilities...),
					Assignment: assignment,
					Enabled: item.Enabled,
				}
				if item.Runtime != nil {
					view.Connection = item.Runtime.Connection
					view.Readiness = item.Runtime.Readiness
					runtimeCopy := *item.Runtime
					runtimeCopy.ObservedState = append(json.RawMessage(nil), item.Runtime.ObservedState...)
					runtimeCopy.NetworkState = append(json.RawMessage(nil), item.Runtime.NetworkState...)
					view.Runtime = &runtimeCopy
				}
				if scope, ok := runtime.CurrentV2Scope(item.ID); ok {
					copy := scope
					view.LiveScope = &copy
				}
				out = append(out, view)
			}
			writeJSON(w, http.StatusOK, map[string]any{"devices": out})
		}))

		s.mux.HandleFunc("GET /api/v1/stage-devices/unassigned", withPermission(auth, userauth.PermissionCompanionPair, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			items, err := devices.ListDevices(r.Context(), "")
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_INVENTORY_UNAVAILABLE"})
				return
			}
			type unassignedDevice struct {
				DeviceID        string                           `json:"device_id"`
				DisplayName     string                           `json:"display_name"`
				DeviceKind      deviceexperience.DeviceKind      `json:"device_kind"`
				ProfileID       string                           `json:"profile_id,omitempty"`
				AssignmentEpoch int64                            `json:"assignment_epoch"`
				Connection      deviceexperience.ConnectionState `json:"connection_state,omitempty"`
				Readiness       deviceexperience.Readiness       `json:"readiness,omitempty"`
			}
			out := make([]unassignedDevice, 0)
			for _, item := range items {
				if !item.Enabled || item.ProtocolVersion != deviceexperience.ProtocolVersion2 || item.ProjectID != "" {
					continue
				}
				assignment, err := devices.GetAssignmentRecord(r.Context(), item.ID)
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_ASSIGNMENT_UNAVAILABLE"})
					return
				}
				if assignment.State != "UNASSIGNED" || assignment.ProjectID != "" {
					continue
				}
				view := unassignedDevice{
					DeviceID: item.ID, DisplayName: item.DisplayName,
					DeviceKind: item.Kind, ProfileID: item.ProfileID,
					AssignmentEpoch: assignment.Epoch,
				}
				if item.Runtime != nil {
					view.Connection = item.Runtime.Connection
					view.Readiness = item.Runtime.Readiness
				}
				out = append(out, view)
			}
			writeJSON(w, http.StatusOK, map[string]any{"devices": out})
		}))

		s.mux.HandleFunc("POST /api/v1/stage-devices/{device_id}/decommission", withPermission(auth, userauth.PermissionCompanionRevoke, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if session.User.Role != userauth.RoleOwner {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "OWNER_REQUIRED"})
				return
			}
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			var input struct {
				Confirm string `json:"confirm"`
				Reason  string `json:"reason"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			if strings.TrimSpace(input.Confirm) != "DECOMMISSION_OFFLINE_TABLET" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "TABLET_DECOMMISSION_CONFIRMATION_REQUIRED"})
				return
			}
			record, err := devices.DecommissionOfflineTablet(r.Context(), deviceID, session.User.ID, input.Reason)
			if err != nil {
				status := http.StatusConflict
				if errors.Is(err, sql.ErrNoRows) {
					status = http.StatusNotFound
				} else if errors.Is(err, deviceexperience.ErrInvalidDevice) {
					status = http.StatusBadRequest
				}
				writeJSON(w, status, map[string]any{"error": "TABLET_DECOMMISSION_BLOCKED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"decommission": record,
				"active_inventory": false,
				"history_preserved": true,
			})
		}))

		s.mux.HandleFunc("GET /api/v1/stage-devices/{device_id}", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			item, err := devices.GetDevice(r.Context(), strings.TrimSpace(r.PathValue("device_id")))
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_NOT_FOUND"})
				return
			}
			// Unassigned identities are pairing inventory, not members of
			// whichever Project the requesting user happens to read.
			if item.ProjectID == "" && userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			writeJSON(w, http.StatusOK, item)
		}))

		// Read-only legacy/v2 assignment metadata for inventory diagnostics.
		// This does not assign or transfer a device, and LEGACY is never a
		// statement that the new authenticated v2 handshake has completed.
		s.mux.HandleFunc("GET /api/v1/stage-devices/{device_id}/assignment", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			record, err := devices.GetAssignmentRecord(r.Context(), deviceID)
			if err != nil {
				if errors.Is(err, deviceexperience.ErrInvalidDevice) {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_ID_INVALID"})
					return
				}
				if errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_ASSIGNMENT_NOT_FOUND"})
					return
				}
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_ASSIGNMENT_UNAVAILABLE"})
				return
			}
			if record.ProjectID == "" && userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			writeJSON(w, http.StatusOK, record)
		}))

		// Read-only Operator status for the current Hub-owned v2 assignment.
		// An ACK stored on an earlier socket is historical, not live proof.
		// This endpoint never reports independent physical DMX verification.
		s.mux.HandleFunc("GET /api/v1/stage-devices/{device_id}/assignment/transfer-status", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			assignment, err := devices.GetAssignmentRecord(r.Context(), deviceID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_ASSIGNMENT_NOT_FOUND"})
					return
				}
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_TRANSFER_STATUS_UNAVAILABLE"})
				return
			}
			scope, online := runtime.CurrentV2Scope(deviceID)
			generation := scope.Generation
			reported := false
			var ack *deviceexperience.BlockedEpochAck
			if assignment.State == "BLOCKED" {
				record, err := devices.GetBlockedEpochAck(r.Context(), deviceID, assignment.Epoch)
				if err == nil && record.ProjectID == assignment.ProjectID {
					ack = &record
					reported = online && generation == record.ConnectionGeneration
				} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_EPOCH_ACK_UNAVAILABLE"})
					return
				}
			}
			liveActive := assignment.State == "ACTIVE" && online &&
				scope.ProjectID == assignment.ProjectID &&
				scope.RuntimeSnapshotID == assignment.RuntimeSnapshotID &&
				scope.AssignmentEpoch == assignment.Epoch &&
				scope.CommandsEnabled
			status := "NOT_ELIGIBLE_FOR_V2_TRANSFER"
			switch assignment.State {
			case "UNASSIGNED":
				status = "UNASSIGNED"
			case "BLOCKED":
				status = "AWAITING_CURRENT_SOFTWARE_EPOCH_ACK"
				if reported {
					status = "CURRENT_SOFTWARE_ZERO_REPORTED_BLOCKED"
				}
			case "ACTIVE":
				status = "ACTIVE_AWAITING_CURRENT_SCOPE_ACK"
				if liveActive {
					status = "ACTIVE_CURRENT_SCOPE_READY"
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"assignment": assignment,
				"epoch_ack": ack,
				"connection_online": online,
				"software_zero_report_current_connection": reported,
				"live_scope": func() any {
					if !online {
						return nil
					}
					return scope
				}(),
				"status": status,
				"commands_enabled": liveActive,
				"snapshot_active": assignment.State == "ACTIVE" && assignment.RuntimeSnapshotID != "",
				"physical_blackout_verified": false,
				"note": "DEVICE_REPORT_ONLY_NO_INDEPENDENT_PHYSICAL_DMX_PROOF",
			})
		}))

		// One-way migration for a known project-linked v1 ESP32 lighting node.
		// This only fences legacy authority and enters BLOCKED v2 state. It does
		// not activate a Runtime Snapshot, enable commands or claim physical zero.
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/stage-devices/{device_id}/migrate-v2-lighting", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if session.User.Role != userauth.RoleOwner {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "OWNER_REQUIRED"})
				return
			}
			if userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			if projectID == "" || deviceID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LEGACY_V2_MIGRATION_SCOPE_REQUIRED"})
				return
			}
			var input struct {
				Confirm string `json:"confirm"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			const confirmation = "MIGRATE_LEGACY_LIGHTING_TO_V2_BLOCKED"
			if strings.TrimSpace(input.Confirm) != confirmation {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LEGACY_V2_MIGRATION_CONFIRMATION_REQUIRED"})
				return
			}
			record, err := devices.MigrateLegacyLightingToV2Blocked(
				r.Context(), deviceID, projectID, session.User.ID)
			if err != nil {
				switch {
				case errors.Is(err, sql.ErrNoRows):
					writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_NOT_FOUND"})
				case errors.Is(err, deviceexperience.ErrInvalidState):
					writeJSON(w, http.StatusConflict, map[string]any{"error": "LEGACY_V2_MIGRATION_BLOCKED", "detail": err.Error()})
				default:
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "LEGACY_V2_MIGRATION_FAILED", "detail": err.Error()})
				}
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"migration": record,
				"state": "BLOCKED",
				"commands_enabled": false,
				"snapshot_active": false,
				"physical_blackout_verified": false,
				"note": "LEGACY_AUTHORITY_FENCED_V2_BLOCKED_REQUIRES_FRESH_RUNTIME_ACK",
			})
		}))

		// An explicit read-only transfer preflight. The requester needs both
		// project.edit and companion.pair before seeing device identity or
		// project affiliation. The returned result does not mutate storage
		// and is NEVER sufficient to authorize a later transfer commit.
		s.mux.HandleFunc("POST /api/v1/stage-devices/{device_id}/assignment/preflight", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			var input struct {
				ExpectedProjectID string `json:"expected_project_id"`
				TargetProjectID   string `json:"target_project_id"`
				ExpectedEpoch     int64  `json:"expected_assignment_epoch"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			preflight, err := devices.PreflightTransfer(r.Context(), deviceexperience.TransferPreflightInput{
				DeviceID: deviceID, ExpectedProjectID: input.ExpectedProjectID,
				TargetProjectID: input.TargetProjectID, ExpectedEpoch: input.ExpectedEpoch,
			})
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_NOT_FOUND"})
				return
			}
			if err != nil {
				if errors.Is(err, deviceexperience.ErrInvalidState) {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_TRANSFER_PREFLIGHT_BLOCKED"})
					return
				}
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_TRANSFER_PREFLIGHT_UNAVAILABLE"})
				return
			}
			if !runtime.IsConnected(deviceID) {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_OFFLINE"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"preflight": preflight,
				"note": "CHECK_ONLY_NOT_A_TRANSFER_OR_PHYSICAL_BLACKOUT_PROOF",
			})
		}))

		// EXPERIMENTAL / disabled by default. This route only commits a
		// BLOCKED/UNASSIGNED epoch after a device-reported SOFTWARE blackout.
		// It never activates a snapshot or new Project commands and is not
		// evidence that a physical DMX fixture is electrically dark.
		s.mux.HandleFunc("POST /api/v1/stage-devices/{device_id}/assignment/software-transfer", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if os.Getenv("STAGECORE_EXPERIMENTAL_V2_SOFTWARE_TRANSFER") != "1" {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_TRANSFER_DISABLED"})
				return
			}
			if userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			var input struct {
				ExpectedProjectID string `json:"expected_project_id"`
				TargetProjectID   string `json:"target_project_id"`
				ExpectedEpoch     int64  `json:"expected_assignment_epoch"`
				Confirm          string `json:"confirm"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			// Explicit acknowledgement cannot be inherited from Preflight or
			// inferred from a Project choice; the action is blackout-first.
			const confirmation = "BLOCK_OUTPUTS_AND_CHANGE_PROJECT_SOFTWARE_ONLY"
			if input.Confirm != confirmation {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_TRANSFER_CONFIRMATION_REQUIRED"})
				return
			}
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			intent := deviceexperience.TransferPreflightInput{
				DeviceID: deviceID,
				ExpectedProjectID: input.ExpectedProjectID,
				TargetProjectID: input.TargetProjectID,
				ExpectedEpoch: input.ExpectedEpoch,
			}
			if _, err := devices.PreflightTransfer(r.Context(), intent); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_NOT_FOUND"})
					return
				}
				writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_TRANSFER_PREFLIGHT_BLOCKED"})
				return
			}
			token, ok := browserSessionToken(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
				return
			}
			csrf := r.Header.Get(csrfHeader)
			actor := session.User.ID
			reauthorize := func(ctx context.Context) error {
				next, err := auth.ValidateCSRF(ctx, token, csrf)
				if err != nil {
					return err
				}
				if next.User.ID != actor {
					return userauth.ErrForbidden
				}
				if err := userauth.Authorize(next.User.Role, userauth.PermissionProjectEdit); err != nil {
					return err
				}
				return userauth.Authorize(next.User.Role, userauth.PermissionCompanionPair)
			}
			record, err := runtime.ExecuteReservedSoftwareTransferAuthorized(
				r.Context(), intent, actor, reauthorize)
			if err != nil {
				// Never interpret an error as a successful transfer. Client
				// must refetch Hub-owned assignment, not retry a stale ACK.
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "STAGE_DEVICE_SOFTWARE_TRANSFER_NOT_COMMITTED",
					"note": "VERIFY_HUB_ASSIGNMENT_BEFORE_RETRY",
				})
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"transfer": record,
				"state": record.NextState,
				"commands_enabled": false,
				"physical_blackout_verified": false,
				"epoch_ack_required": record.NextState == "BLOCKED",
				"note": "SOFTWARE_ACK_ONLY_NOT_PHYSICAL_DMX_QUALIFICATION",
			})
		}))

		// EXPERIMENTAL / disabled by default. Activation is a second step after
		// an attended BLOCKED transfer + current-generation software-zero ACK.
		// It applies the exact Published Runtime Snapshot configuration while
		// blackout remains asserted, then requires a fresh reconnect/scope ACK.
		// StageLaser first assignment is fail-closed: the exact authenticated
		// socket must report DISARMED + OFF with known state before the Hub
		// commits ACTIVE Project/Snapshot authority. The committed socket is
		// closed and commands remain disabled until a fresh scope ACK.
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/stage-devices/{device_id}/stagelaser-assignment", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			if projectID == "" || deviceID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGELASER_ASSIGNMENT_SCOPE_REQUIRED"})
				return
			}
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			var input struct {
				RuntimeSnapshotID string `json:"runtime_snapshot_id"`
				ExpectedEpoch     int64  `json:"expected_assignment_epoch"`
				Confirm           string `json:"confirm"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			const confirmation = "VERIFY_SAFE_OFF_AND_ASSIGN_STAGELASER"
			if input.Confirm != confirmation {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGELASER_ASSIGNMENT_CONFIRMATION_REQUIRED"})
				return
			}
			token, ok := browserSessionToken(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
				return
			}
			csrf := r.Header.Get(csrfHeader)
			actor := session.User.ID
			reauthorize := func(ctx context.Context) error {
				next, err := auth.ValidateCSRF(ctx, token, csrf)
				if err != nil {
					return err
				}
				if next.User.ID != actor {
					return userauth.ErrForbidden
				}
				if err := userauth.Authorize(next.User.Role, userauth.PermissionProjectEdit); err != nil {
					return err
				}
				return userauth.Authorize(next.User.Role, userauth.PermissionCompanionPair)
			}
			record, err := runtime.ExecuteStageLaserAssignmentAuthorized(
				r.Context(),
				deviceexperience.StageLaserAssignmentInput{
					DeviceID: deviceID,
					TargetProjectID: projectID,
					TargetRuntimeSnapshotID: strings.TrimSpace(input.RuntimeSnapshotID),
					ExpectedEpoch: input.ExpectedEpoch,
				},
				actor,
				reauthorize,
			)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "STAGELASER_ASSIGNMENT_NOT_COMMITTED",
					"detail": err.Error(),
					"note": "REFETCH_ASSIGNMENT_AND_CURRENT_DEVICE_STATE_BEFORE_RETRY",
				})
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"assignment": record,
				"state": "ACTIVE",
				"commands_enabled": false,
				"reconnect_required": true,
				"state_quality": record.StateQuality,
				"physical_state_confirmed": record.StateQuality == "CONFIRMED",
				"note": "TRACKED_STATE_IS_SOFTWARE_STATE_UNLESS_PHYSICAL_FEEDBACK_EXISTS",
			})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/stage-devices/{device_id}/lighting-activation", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if os.Getenv("STAGECORE_EXPERIMENTAL_V2_LIGHTING_ACTIVATION") != "1" {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "LIGHTING_ACTIVATION_DISABLED"})
				return
			}
			if userauth.Authorize(session.User.Role, userauth.PermissionCompanionPair) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "STAGE_DEVICE_PAIRING_PERMISSION_REQUIRED"})
				return
			}
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			if projectID == "" || deviceID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_ACTIVATION_SCOPE_REQUIRED"})
				return
			}
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			var input struct {
				RuntimeSnapshotID string `json:"runtime_snapshot_id"`
				ExpectedEpoch     int64  `json:"expected_assignment_epoch"`
				Confirm           string `json:"confirm"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			const confirmation = "APPLY_PUBLISHED_CONFIG_AND_ACTIVATE_LIGHTING_SOFTWARE_ONLY"
			if input.Confirm != confirmation {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "LIGHTING_ACTIVATION_CONFIRMATION_REQUIRED"})
				return
			}
			token, ok := browserSessionToken(r)
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "AUTH_REQUIRED"})
				return
			}
			csrf := r.Header.Get(csrfHeader)
			actor := session.User.ID
			reauthorize := func(ctx context.Context) error {
				next, err := auth.ValidateCSRF(ctx, token, csrf)
				if err != nil {
					return err
				}
				if next.User.ID != actor {
					return userauth.ErrForbidden
				}
				if err := userauth.Authorize(next.User.Role, userauth.PermissionProjectEdit); err != nil {
					return err
				}
				return userauth.Authorize(next.User.Role, userauth.PermissionCompanionPair)
			}
			record, err := runtime.ExecuteLightingActivationAuthorized(
				r.Context(),
				deviceexperience.LightingActivationInput{
					DeviceID: deviceID,
					ProjectID: projectID,
					RuntimeSnapshotID: strings.TrimSpace(input.RuntimeSnapshotID),
					ExpectedEpoch: input.ExpectedEpoch,
				},
				actor,
				reauthorize,
			)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "LIGHTING_ACTIVATION_NOT_COMMITTED",
					"detail": err.Error(),
					"note": "REFETCH_ASSIGNMENT_AND_CURRENT_SOFTWARE_ZERO_BEFORE_RETRY",
				})
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"activation": record,
				"state": "ACTIVE",
				"commands_enabled": false,
				"reconnect_required": true,
				"physical_blackout_verified": false,
				"note": "SOFTWARE_ZERO_AND_CONFIG_HASH_VERIFIED_NOT_PHYSICAL_DMX_QUALIFICATION",
			})
		}))

		s.mux.HandleFunc("POST /api/v1/stage-devices/{device_id}/commands", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			device, err := devices.GetDevice(r.Context(), deviceID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_NOT_FOUND"})
				return
			}
			var input struct {
				CommandType       string          `json:"command_type"`
				SessionID         string          `json:"session_id"`
				RuntimeSnapshotID string          `json:"runtime_snapshot_id"`
				CorrelationID     string          `json:"correlation_id"`
				CausationID       string          `json:"causation_id"`
				Priority          string          `json:"priority"`
				IdempotencyKey    string          `json:"idempotency_key"`
				Payload           json.RawMessage `json:"payload"`
				DeadlineAt        *time.Time      `json:"deadline_at"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			commandProjectID := device.ProjectID
			commandSnapshotID := strings.TrimSpace(input.RuntimeSnapshotID)
			if device.ProtocolVersion == deviceexperience.ProtocolVersion2 {
				profileAuthorized := (device.Kind == deviceexperience.DeviceTabletPlayer &&
					device.ProfileID == deviceexperience.TabletPlayerProfileID) ||
					device.ProfileID == lightingnode.ProfileID
				if !profileAuthorized ||
					device.Assignment == nil || device.Assignment.State != "ACTIVE" ||
					device.Assignment.ProjectID == "" || device.Assignment.RuntimeSnapshotID == "" {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_PROJECT_UNBOUND"})
					return
				}
				commandProjectID = device.Assignment.ProjectID
				if commandSnapshotID != "" && commandSnapshotID != device.Assignment.RuntimeSnapshotID {
					writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_SNAPSHOT_SCOPE_MISMATCH"})
					return
				}
				commandSnapshotID = device.Assignment.RuntimeSnapshotID
			} else if commandProjectID == "" {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_PROJECT_UNBOUND"})
				return
			}
			command, err := runtime.Dispatch(r.Context(), deviceexperience.CreateCommandInput{
				ProjectID:         commandProjectID,
				SessionID:         input.SessionID,
				DeviceID:          device.ID,
				CommandType:       input.CommandType,
				Issuer:            session.User.ID,
				CorrelationID:     input.CorrelationID,
				CausationID:       input.CausationID,
				RuntimeSnapshotID: commandSnapshotID,
				Priority:          input.Priority,
				IdempotencyKey:    input.IdempotencyKey,
				Payload:           input.Payload,
				DeadlineAt:        input.DeadlineAt,
			})
			if err != nil {
				writeStageDeviceCommandError(w, err)
				return
			}
			status := http.StatusAccepted
			if command.CompletedAt != nil {
				status = http.StatusOK
			}
			writeJSON(w, status, command)
		}))

		// Broad sends are expanded server-side to independent command identities
		// while preserving one correlation ID. Group and location selectors are
		// canonical Stage Device metadata, not browser-only filtering.
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/stage-device-commands", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if projectID == "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_PROJECT_REQUIRED"})
				return
			}
			var input struct {
				All               bool            `json:"all"`
				GroupName         string          `json:"group_name"`
				LocationName      string          `json:"location_name"`
				DeviceIDs         []string        `json:"device_ids"`
				CommandType       string          `json:"command_type"`
				SessionID         string          `json:"session_id"`
				RuntimeSnapshotID string          `json:"runtime_snapshot_id"`
				CorrelationID     string          `json:"correlation_id"`
				CausationID       string          `json:"causation_id"`
				Priority          string          `json:"priority"`
				IdempotencyKey    string          `json:"idempotency_key"`
				Payload           json.RawMessage `json:"payload"`
				DeadlineAt        *time.Time      `json:"deadline_at"`
			}
			if !decodeBoundedJSON(w, r, &input) {
				return
			}
			allDevices, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "STAGE_DEVICE_LIST_FAILED", "detail": err.Error()})
				return
			}
			targets, ok := selectStageDeviceTargets(allDevices, input.All, input.GroupName, input.LocationName, input.DeviceIDs)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "STAGE_DEVICE_TARGET_INVALID", "detail": "choose exactly one of all, group_name, location_name or device_ids"})
				return
			}
			if len(targets) == 0 {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_TARGET_EMPTY"})
				return
			}

			correlationID := strings.TrimSpace(input.CorrelationID)
			if correlationID == "" {
				correlationID, err = stageid.New()
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "CORRELATION_ID_FAILED", "detail": err.Error()})
					return
				}
			}
			baseKey := strings.TrimSpace(input.IdempotencyKey)
			if baseKey == "" {
				baseKey = correlationID + ":" + strings.TrimSpace(input.CommandType)
			}
			type batchResult struct {
				DeviceID string                          `json:"device_id"`
				Command  *deviceexperience.DeviceCommand `json:"command,omitempty"`
				Error    string                          `json:"error,omitempty"`
			}
			results := make([]batchResult, 0, len(targets))
			for _, device := range targets {
				command, dispatchErr := runtime.Dispatch(r.Context(), deviceexperience.CreateCommandInput{
					ProjectID:         projectID,
					SessionID:         input.SessionID,
					DeviceID:          device.ID,
					CommandType:       input.CommandType,
					Issuer:            session.User.ID,
					CorrelationID:     correlationID,
					CausationID:       input.CausationID,
					RuntimeSnapshotID: input.RuntimeSnapshotID,
					Priority:          input.Priority,
					IdempotencyKey:    baseKey + ":" + device.ID,
					Payload:           input.Payload,
					DeadlineAt:        input.DeadlineAt,
				})
				if dispatchErr != nil {
					results = append(results, batchResult{DeviceID: device.ID, Error: dispatchErr.Error()})
					continue
				}
				copy := command
				results = append(results, batchResult{DeviceID: device.ID, Command: &copy})
			}
			writeJSON(w, http.StatusOK, map[string]any{"correlation_id": correlationID, "results": results})
		}))

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/live-video-sources", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if _, err := stageStore.GetProject(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "PROJECT_NOT_FOUND"})
				return
			}
			sources, err := devices.ListLiveSources(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "LIVE_SOURCE_LIST_FAILED", "detail": err.Error()})
				return
			}
			if !tabletHasConfiguredRelaySource(sources) {
				if source, ok := tabletAutoRelaySource(projectID, tabletAutoRelayAdvertiseHost(r), time.Now()); ok {
					sources = append(sources, source)
				}
			}
			cameraStatus := probeTabletCameraRelay(r.Context(), tabletRelaySource(sources))
			writeJSON(w, http.StatusOK, map[string]any{"sources": sources, "camera_status": cameraStatus})
		}))

		s.mux.HandleFunc("PUT /api/v1/projects/{project_id}/live-video-sources/{source_id}", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			if err := stageStore.RequireProjectConfigurationMutable(r.Context(), projectID); err != nil {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "detail": err.Error()})
				return
			}
			var source deviceexperience.LiveSource
			if !decodeBoundedJSON(w, r, &source) {
				return
			}
			source.ID = strings.TrimSpace(r.PathValue("source_id"))
			source.ProjectID = projectID
			updated, err := devices.UpsertLiveSource(r.Context(), source)
			if err != nil {
				status := http.StatusBadRequest
				if errors.Is(err, deviceexperience.ErrCapabilityMissing) {
					status = http.StatusConflict
				}
				writeJSON(w, status, map[string]any{"error": "LIVE_SOURCE_UPDATE_FAILED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, updated)
		}))

		s.mux.HandleFunc("GET /api/v1/network/cockpit", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			items, err := devices.Cockpit(r.Context(), 15*time.Second)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "NETWORK_COCKPIT_FAILED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"targets": items, "stale_after_ms": 15000})
		}))
	}
}


func waitForStageDeviceScope(
	ctx context.Context,
	runtime *devicechannel.Runtime,
	deviceID, projectID, runtimeSnapshotID string,
	assignmentEpoch int64,
	timeout time.Duration,
) bool {
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if scope, ok := runtime.CurrentV2Scope(deviceID); ok &&
			scope.ProjectID == projectID &&
			scope.RuntimeSnapshotID == runtimeSnapshotID &&
			scope.AssignmentEpoch == assignmentEpoch &&
			scope.CommandsEnabled {
			return true
		}
		select {
		case <-wait.Done():
			return false
		case <-ticker.C:
		}
	}
}


func writeStageDeviceCommandError(w http.ResponseWriter, err error) {
	status := http.StatusConflict
	switch {
	case errors.Is(err, deviceexperience.ErrCommandExpired):
		status = http.StatusGone
	case errors.Is(err, deviceexperience.ErrInvalidState), errors.Is(err, deviceexperience.ErrInvalidDevice):
		status = http.StatusBadRequest
	case errors.Is(err, deviceexperience.ErrCapabilityMissing):
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]any{"error": "STAGE_DEVICE_COMMAND_REJECTED", "detail": err.Error()})
}

func selectStageDeviceTargets(devices []deviceexperience.Device, all bool, groupName, locationName string, deviceIDs []string) ([]deviceexperience.Device, bool) {
	groupName = strings.TrimSpace(groupName)
	locationName = strings.TrimSpace(locationName)
	wantedIDs := make(map[string]struct{}, len(deviceIDs))
	for _, id := range deviceIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			wantedIDs[id] = struct{}{}
		}
	}
	selectors := 0
	if all {
		selectors++
	}
	if groupName != "" {
		selectors++
	}
	if locationName != "" {
		selectors++
	}
	if len(wantedIDs) > 0 {
		selectors++
	}
	if selectors != 1 {
		return nil, false
	}
	out := make([]deviceexperience.Device, 0)
	for _, device := range devices {
		switch {
		case all:
			out = append(out, device)
		case groupName != "" && device.GroupName == groupName:
			out = append(out, device)
		case locationName != "" && device.LocationName == locationName:
			out = append(out, device)
		default:
			if _, ok := wantedIDs[device.ID]; ok {
				out = append(out, device)
			}
		}
	}
	return out, true
}
