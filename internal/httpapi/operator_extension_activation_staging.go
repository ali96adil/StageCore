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

type operatorExtensionActivationStaging struct {
	users  *userauth.Service
	stager *extension.ActivationStager
	audit  *securityaudit.Service
}

func WithOperatorExtensionActivationStaging(users *userauth.Service, stager *extension.ActivationStager, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || stager == nil {
			return
		}
		h := &operatorExtensionActivationStaging{users: users, stager: stager, audit: audit}
		s.mux.HandleFunc("POST /api/v1/extensions/installations/{installation_id}/activation-staging-check", withPermission(users, userauth.PermissionPluginManage, h.check))
	}
}

func (h *operatorExtensionActivationStaging) check(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	installationID := strings.TrimSpace(r.PathValue("installation_id"))
	result, err := h.stager.Check(r.Context(), installationID)
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "EXTENSION_ACTIVATION_STAGING_UNAVAILABLE"
		response := map[string]any{}
		switch {
		case errors.Is(err, domain.ErrShowConfigurationLocked):
			status = http.StatusConflict
			code = "SHOW_CONFIGURATION_LOCKED"
		case errors.Is(err, domain.ErrNotFound):
			status = http.StatusNotFound
			code = "EXTENSION_INSTALLATION_NOT_FOUND"
		case errors.Is(err, extension.ErrActivationNotReady):
			status = http.StatusConflict
			code = "EXTENSION_ACTIVATION_NOT_READY"
			var notReady *extension.ActivationNotReadyError
			if errors.As(err, &notReady) {
				response["assessment"] = notReady.Assessment
			}
		case errors.Is(err, extension.ErrInstalledPayloadIntegrity), errors.Is(err, extension.ErrPermissionReviewIntegrity), errors.Is(err, extension.ErrActivationStagingIntegrity):
			status = http.StatusConflict
			code = "EXTENSION_ACTIVATION_STAGING_INTEGRITY_FAILED"
		}
		response["error_code"] = code
		h.record(r, session, installationID, securityaudit.ResultRejected, code, nil)
		writeJSON(w, status, response)
		return
	}
	h.record(r, session, installationID, securityaudit.ResultSuccess, "", map[string]any{
		"extension_id": result.ExtensionID,
		"version": result.Version,
		"status": result.Status,
		"runtime_permissions": result.RuntimePermissions,
		"execution_authorized": result.ExecutionAuthorized,
		"execution_blocker": result.ExecutionBlocker,
	})
	writeJSON(w, http.StatusOK, result)
}

func (h *operatorExtensionActivationStaging) record(r *http.Request, session userauth.Session, installationID, result, reason string, metadata any) {
	if h.audit == nil {
		return
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType: "extension.activation.staging_check",
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
