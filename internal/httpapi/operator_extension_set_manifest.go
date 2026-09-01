package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type operatorExtensionSetManifest struct {
	users   *userauth.Service
	service *extension.ExtensionSetService
	audit   *securityaudit.Service
}

func WithOperatorExtensionSetManifest(users *userauth.Service, service *extension.ExtensionSetService, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || service == nil {
			return
		}
		h := &operatorExtensionSetManifest{users: users, service: service, audit: audit}
		s.mux.HandleFunc("GET /api/v1/extensions/set-manifest", withPermission(users, userauth.PermissionProjectRead, h.export))
		s.mux.HandleFunc("POST /api/v1/extensions/set-manifest/restore-plan", withPermission(users, userauth.PermissionPluginManage, h.planRestore))
		s.mux.HandleFunc("POST /api/v1/extensions/set-manifest/restore", withPermission(users, userauth.PermissionPluginManage, h.restore))
	}
}

func (h *operatorExtensionSetManifest) export(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	manifest, raw, err := h.service.Export(r.Context())
	if err != nil {
		h.record(r, session, "extension.set_manifest.export", securityaudit.ResultRejected, "EXTENSION_SET_EXPORT_FAILED", nil)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_SET_EXPORT_FAILED"})
		return
	}
	h.record(r, session, "extension.set_manifest.export", securityaudit.ResultSuccess, "", map[string]any{"extension_count": len(manifest.Extensions)})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="stagecore-extension-set.json"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (h *operatorExtensionSetManifest) planRestore(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	raw, ok := readExtensionSetManifest(w, r)
	if !ok {
		return
	}
	plan, err := h.service.PlanRestore(r.Context(), raw)
	if err != nil {
		if errors.Is(err, extension.ErrExtensionSetInvalid) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXTENSION_SET_INVALID"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_SET_PLAN_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (h *operatorExtensionSetManifest) restore(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	raw, ok := readExtensionSetManifest(w, r)
	if !ok {
		return
	}
	result, err := h.service.Restore(r.Context(), raw, session.User.Username)
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "EXTENSION_SET_RESTORE_FAILED"
		response := map[string]any{}
		switch {
		case errors.Is(err, extension.ErrExtensionSetInvalid):
			status = http.StatusBadRequest
			code = "EXTENSION_SET_INVALID"
		case errors.Is(err, extension.ErrExtensionSetRestoreBlocked):
			status = http.StatusConflict
			code = "EXTENSION_SET_RESTORE_BLOCKED"
			response["plan"] = result.Plan
		case errors.Is(err, domain.ErrShowConfigurationLocked):
			status = http.StatusConflict
			code = "SHOW_CONFIGURATION_LOCKED"
		case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
			status = http.StatusConflict
			code = "EXTENSION_INSTALL_INTEGRITY_FAILED"
		}
		response["error_code"] = code
		h.record(r, session, "extension.set_manifest.restore", securityaudit.ResultRejected, code, map[string]any{
			"plan_status": result.Plan.Status,
			"blockers": len(result.Plan.Blockers),
			"installed_before_failure": len(result.Installed),
		})
		writeJSON(w, status, response)
		return
	}
	h.record(r, session, "extension.set_manifest.restore", securityaudit.ResultSuccess, "", map[string]any{
		"plan_status": result.Plan.Status,
		"installed": len(result.Installed),
		"permission_reviews_restored": result.Plan.PermissionReviewsRestored,
		"runtime_intent_restored": result.Plan.RuntimeIntentRestored,
	})
	writeJSON(w, http.StatusOK, result)
}

func readExtensionSetManifest(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	limited := http.MaxBytesReader(w, r.Body, extension.MaxExtensionSetManifestSize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error_code": "EXTENSION_SET_TOO_LARGE"})
			return nil, false
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXTENSION_SET_READ_FAILED"})
		return nil, false
	}
	if len(raw) > extension.MaxExtensionSetManifestSize {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error_code": "EXTENSION_SET_TOO_LARGE"})
		return nil, false
	}
	return raw, true
}

func (h *operatorExtensionSetManifest) record(r *http.Request, session userauth.Session, eventType, result, reason string, metadata any) {
	if h.audit == nil {
		return
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType: eventType,
		ActorUserID: session.User.ID,
		ActorUsername: session.User.Username,
		Source: "operator-web",
		ResourceType: "extension_set_manifest",
		ResourceID: "installed-extensions",
		Result: result,
		Reason: reason,
		Metadata: metadata,
	})
}
