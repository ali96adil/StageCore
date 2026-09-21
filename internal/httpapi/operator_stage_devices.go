package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

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
			writeJSON(w, http.StatusOK, map[string]any{"devices": items})
		}))

		// Device provisioning inventory is not scoped to any Project yet.
		// Only an operator authorized for pairing may discover unassigned v2
		// identities. This endpoint grants NO Assign/Transfer authority.
		s.mux.HandleFunc("GET /api/v1/stage-devices/unassigned", withPermission(auth, userauth.PermissionCompanionPair, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			items, err := devices.ListDevices(r.Context(), "")
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "STAGE_DEVICE_INVENTORY_UNAVAILABLE"})
				return
			}
			type unassignedDevice struct {
				DeviceID        string                       `json:"device_id"`
				DisplayName     string                       `json:"display_name"`
				DeviceKind      deviceexperience.DeviceKind  `json:"device_kind"`
				AssignmentEpoch int64                        `json:"assignment_epoch"`
				Connection      deviceexperience.ConnectionState `json:"connection_state,omitempty"`
				Readiness       deviceexperience.Readiness       `json:"readiness,omitempty"`
			}
			out := make([]unassignedDevice, 0)
			for _, item := range items {
				if item.ProtocolVersion != deviceexperience.ProtocolVersion2 || item.ProjectID != "" {
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
					DeviceKind: item.Kind, AssignmentEpoch: assignment.Epoch,
				}
				if item.Runtime != nil {
					view.Connection = item.Runtime.Connection
					view.Readiness = item.Runtime.Readiness
				}
				out = append(out, view)
			}
			writeJSON(w, http.StatusOK, map[string]any{"devices": out})
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

		s.mux.HandleFunc("POST /api/v1/stage-devices/{device_id}/commands", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			deviceID := strings.TrimSpace(r.PathValue("device_id"))
			device, err := devices.GetDevice(r.Context(), deviceID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_NOT_FOUND"})
				return
			}
			if device.ProjectID == "" {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "STAGE_DEVICE_PROJECT_UNBOUND"})
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
			command, err := runtime.Dispatch(r.Context(), deviceexperience.CreateCommandInput{
				ProjectID:         device.ProjectID,
				SessionID:         input.SessionID,
				DeviceID:          device.ID,
				CommandType:       input.CommandType,
				Issuer:            session.User.ID,
				CorrelationID:     input.CorrelationID,
				CausationID:       input.CausationID,
				RuntimeSnapshotID: input.RuntimeSnapshotID,
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

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/live-video-sources", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			items, err := devices.ListLiveSources(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "LIVE_SOURCE_LIST_FAILED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"sources": items})
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
