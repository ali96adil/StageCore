package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type operatorExtensionRuntimeLifecycle struct {
	users      *userauth.Service
	supervisor *extension.RuntimeSupervisor
	audit      *securityaudit.Service
}

func WithOperatorExtensionRuntimeLifecycle(users *userauth.Service, supervisor *extension.RuntimeSupervisor, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || supervisor == nil {
			return
		}
		h := &operatorExtensionRuntimeLifecycle{users: users, supervisor: supervisor, audit: audit}
		s.mux.HandleFunc("GET /api/v1/extensions/installations/{installation_id}/runtime", withPermission(users, userauth.PermissionProjectRead, h.status))
		s.mux.HandleFunc("POST /api/v1/extensions/installations/{installation_id}/enable", withPermission(users, userauth.PermissionPluginManage, h.enable))
		s.mux.HandleFunc("POST /api/v1/extensions/installations/{installation_id}/disable", withPermission(users, userauth.PermissionPluginManage, h.disable))
	}
}

func (h *operatorExtensionRuntimeLifecycle) status(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	item, err := h.supervisor.Status(r.Context(), strings.TrimSpace(r.PathValue("installation_id")))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXTENSION_INSTALLATION_NOT_FOUND"})
	case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXTENSION_RUNTIME_INTEGRITY_FAILED"})
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_RUNTIME_STATUS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusOK, item)
	}
}

func (h *operatorExtensionRuntimeLifecycle) enable(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	h.transition(w, r, session, true)
}

func (h *operatorExtensionRuntimeLifecycle) disable(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	h.transition(w, r, session, false)
}

func (h *operatorExtensionRuntimeLifecycle) transition(w http.ResponseWriter, r *http.Request, session userauth.Session, enable bool) {
	installationID := strings.TrimSpace(r.PathValue("installation_id"))
	var item extension.RuntimeLifecycleStatus
	var err error
	eventType := "extension.runtime.disable"
	if enable {
		eventType = "extension.runtime.enable"
		item, err = h.supervisor.Enable(r.Context(), installationID, session.User.Username)
	} else {
		item, err = h.supervisor.Disable(r.Context(), installationID, session.User.Username)
	}
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "EXTENSION_RUNTIME_TRANSITION_FAILED"
		response := map[string]any{}
		if item.InstallationID != "" {
			response["runtime"] = item
		}
		switch {
		case errors.Is(err, domain.ErrShowConfigurationLocked):
			status = http.StatusConflict
			code = "SHOW_CONFIGURATION_LOCKED"
		case errors.Is(err, domain.ErrNotFound):
			status = http.StatusNotFound
			code = "EXTENSION_INSTALLATION_NOT_FOUND"
		case errors.Is(err, extension.ErrRuntimeProbeNotReady), errors.Is(err, extension.ErrRuntimeLifecycleNotReady):
			status = http.StatusConflict
			code = "EXTENSION_RUNTIME_NOT_READY"
			var probeNotReady *extension.RuntimeProbeNotReadyError
			if errors.As(err, &probeNotReady) {
				response["assessment"] = probeNotReady.Assessment
			}
			var lifecycleNotReady *extension.RuntimeLifecycleNotReadyError
			if errors.As(err, &lifecycleNotReady) {
				response["assessment"] = lifecycleNotReady.Assessment
			}
		case errors.Is(err, extension.ErrRuntimeProbeHandshake):
			status = http.StatusConflict
			code = "EXTENSION_RUNTIME_HANDSHAKE_FAILED"
		case errors.Is(err, extension.ErrInstalledPayloadIntegrity), errors.Is(err, extension.ErrRuntimeArtifactInvalid), errors.Is(err, extension.ErrRuntimeProbeIntegrity), errors.Is(err, extension.ErrPermissionReviewIntegrity):
			status = http.StatusConflict
			code = "EXTENSION_RUNTIME_INTEGRITY_FAILED"
		}
		response["error_code"] = code
		h.record(r, session, eventType, installationID, securityaudit.ResultRejected, code, map[string]any{
			"desired_state": item.DesiredState,
			"observed_state": item.ObservedState,
			"generation": item.Generation,
		})
		writeJSON(w, status, response)
		return
	}
	h.record(r, session, eventType, installationID, securityaudit.ResultSuccess, "", map[string]any{
		"extension_id": item.ExtensionID,
		"version": item.Version,
		"desired_state": item.DesiredState,
		"observed_state": item.ObservedState,
		"generation": item.Generation,
	})
	writeJSON(w, http.StatusOK, item)
}

func (h *operatorExtensionRuntimeLifecycle) record(r *http.Request, session userauth.Session, eventType, installationID, result, reason string, metadata any) {
	if h.audit == nil {
		return
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType: eventType,
		ActorUserID: session.User.ID,
		ActorUsername: session.User.Username,
		Source: "operator-web",
		ResourceType: "extension_installation",
		ResourceID: strings.TrimSpace(installationID),
		Result: result,
		Reason: reason,
		Metadata: metadata,
	})
}
