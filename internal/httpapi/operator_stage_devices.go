package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
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
			items, err := devices.ListDevices(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "STAGE_DEVICE_LIST_FAILED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"devices": items})
		}))

		s.mux.HandleFunc("GET /api/v1/stage-devices/{device_id}", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			item, err := devices.GetDevice(r.Context(), strings.TrimSpace(r.PathValue("device_id")))
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "STAGE_DEVICE_NOT_FOUND"})
				return
			}
			writeJSON(w, http.StatusOK, item)
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
				return
			}
			status := http.StatusAccepted
			if command.CompletedAt != nil {
				status = http.StatusOK
			}
			writeJSON(w, status, command)
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
