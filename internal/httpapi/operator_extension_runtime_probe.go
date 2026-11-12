package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type operatorExtensionRuntimeProbe struct {
	users *userauth.Service
	probe extensionRuntimeProbeRunner
	audit *securityaudit.Service
	hooks extensionRuntimeProbeHooks
}

type extensionRuntimeProbeRunner interface {
	Probe(context.Context, string) (extension.RuntimeProbeResult, error)
}

type extensionRuntimeProbeHooks struct {
	BeforeResponse func()
}

func WithOperatorExtensionRuntimeProbe(users *userauth.Service, probe *extension.RuntimeProbe, audit *securityaudit.Service) Option {
	return withOperatorExtensionRuntimeProbe(users, probe, audit, extensionRuntimeProbeHooks{})
}

func withOperatorExtensionRuntimeProbe(users *userauth.Service, probe extensionRuntimeProbeRunner, audit *securityaudit.Service, hooks extensionRuntimeProbeHooks) Option {
	return func(s *Server) {
		if users == nil || probe == nil {
			return
		}
		h := &operatorExtensionRuntimeProbe{users: users, probe: probe, audit: audit, hooks: hooks}
		s.mux.HandleFunc("POST /api/v1/extensions/installations/{installation_id}/runtime-probe", withPermission(users, userauth.PermissionPluginManage, h.run))
	}
}

func (h *operatorExtensionRuntimeProbe) run(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	installationID := strings.TrimSpace(r.PathValue("installation_id"))
	result, err := h.probe.Probe(r.Context(), installationID)
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "EXTENSION_RUNTIME_PROBE_UNAVAILABLE"
		response := map[string]any{}
		switch {
		case errors.Is(err, domain.ErrShowConfigurationLocked):
			status = http.StatusConflict
			code = "SHOW_CONFIGURATION_LOCKED"
		case errors.Is(err, domain.ErrNotFound):
			status = http.StatusNotFound
			code = "EXTENSION_INSTALLATION_NOT_FOUND"
		case errors.Is(err, extension.ErrRuntimeProbeNotReady):
			status = http.StatusConflict
			code = "EXTENSION_RUNTIME_PROBE_NOT_READY"
			var notReady *extension.RuntimeProbeNotReadyError
			if errors.As(err, &notReady) {
				response["assessment"] = notReady.Assessment
			}
		case errors.Is(err, extension.ErrRuntimeProbeHandshake):
			status = http.StatusConflict
			code = "EXTENSION_RUNTIME_PROBE_HANDSHAKE_FAILED"
		case errors.Is(err, extension.ErrInstalledPayloadIntegrity), errors.Is(err, extension.ErrRuntimeArtifactInvalid), errors.Is(err, extension.ErrRuntimeProbeIntegrity), errors.Is(err, extension.ErrPermissionReviewIntegrity):
			status = http.StatusConflict
			code = "EXTENSION_RUNTIME_PROBE_INTEGRITY_FAILED"
		}
		response["error_code"] = code
		h.record(r, session, installationID, securityaudit.ResultRejected, code, nil)
		if h.beforeResponseRevoked(w, r, session) {
			return
		}
		writeJSON(w, status, response)
		return
	}
	h.record(r, session, installationID, securityaudit.ResultSuccess, "", map[string]any{
		"extension_id":                    result.ExtensionID,
		"version":                         result.Version,
		"status":                          result.Status,
		"engine":                          result.Engine,
		"network_mode":                    result.NetworkMode,
		"probe_execution_authorized":      result.ProbeExecutionAuthorized,
		"process_started":                 result.ProcessStarted,
		"process_stopped":                 result.ProcessStopped,
		"persistent_execution_authorized": result.PersistentExecutionAuthorized,
		"persistent_execution_blocker":    result.PersistentExecutionBlocker,
	})
	if h.beforeResponseRevoked(w, r, session) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *operatorExtensionRuntimeProbe) beforeResponseRevoked(w http.ResponseWriter, r *http.Request, session userauth.Session) bool {
	if h.hooks.BeforeResponse != nil {
		h.hooks.BeforeResponse()
	}
	if extensionManageResponseAuthorized(h.users, r, session) {
		return false
	}
	writeJSON(w, http.StatusForbidden, map[string]any{
		"error_code":                     capability.DispatchAuthorityRevokedCode,
		"detail":                         "Authorization ended during extension runtime probing; refresh runtime status before taking further action.",
		"probe_may_have_run":             true,
		"effect_may_have_been_committed": true,
		"retryable":                      false,
	})
	return true
}

func (h *operatorExtensionRuntimeProbe) record(r *http.Request, session userauth.Session, installationID, result, reason string, metadata any) {
	if h.audit == nil {
		return
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType:     "extension.runtime.probe",
		ActorUserID:   session.User.ID,
		ActorUsername: session.User.Username,
		Source:        "operator-web",
		ResourceType:  "extension_installation",
		ResourceID:    strings.TrimSpace(installationID),
		Result:        result,
		Reason:        reason,
		Metadata:      metadata,
	})
}
