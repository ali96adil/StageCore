package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type operatorExtensionInstaller struct {
	users     *userauth.Service
	installer *extension.Installer
	audit     *securityaudit.Service
}

func WithOperatorExtensionInstaller(users *userauth.Service, installer *extension.Installer, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || installer == nil {
			return
		}
		h := &operatorExtensionInstaller{users: users, installer: installer, audit: audit}
		s.mux.HandleFunc("GET /api/v1/extensions/installations", withPermission(users, userauth.PermissionProjectRead, h.list))
		s.mux.HandleFunc("GET /api/v1/extensions/installations/{installation_id}", withPermission(users, userauth.PermissionProjectRead, h.get))
		s.mux.HandleFunc("GET /api/v1/extensions/packages/{package_id}/install-plan", withPermission(users, userauth.PermissionProjectRead, h.plan))
		s.mux.HandleFunc("POST /api/v1/extensions/packages/{package_id}/install", withPermission(users, userauth.PermissionPluginManage, h.install))
	}
}

func (h *operatorExtensionInstaller) list(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	items, err := h.installer.List(r.Context(), strings.TrimSpace(r.URL.Query().Get("extension_id")))
	if errors.Is(err, extension.ErrInstalledPayloadIntegrity) {
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXTENSION_INSTALL_INTEGRITY_FAILED"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_INSTALLATIONS_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installations": items})
}

func (h *operatorExtensionInstaller) get(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	item, err := h.installer.Get(r.Context(), strings.TrimSpace(r.PathValue("installation_id")))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXTENSION_INSTALLATION_NOT_FOUND"})
	case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXTENSION_INSTALL_INTEGRITY_FAILED"})
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_INSTALLATIONS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusOK, item)
	}
}

func (h *operatorExtensionInstaller) plan(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	plan, err := h.installer.PlanInstall(r.Context(), strings.TrimSpace(r.PathValue("package_id")))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXTENSION_PACKAGE_NOT_FOUND"})
	case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXTENSION_INSTALL_INTEGRITY_FAILED"})
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_INSTALL_PLAN_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusOK, plan)
	}
}

func (h *operatorExtensionInstaller) install(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	packageID := strings.TrimSpace(r.PathValue("package_id"))
	item, err := h.installer.InstallPlanned(r.Context(), packageID, session.User.Username)
	if err != nil {
		status := http.StatusServiceUnavailable
		code := "EXTENSION_INSTALL_FAILED"
		response := map[string]any{}
		switch {
		case errors.Is(err, domain.ErrShowConfigurationLocked):
			status = http.StatusConflict
			code = "SHOW_CONFIGURATION_LOCKED"
		case errors.Is(err, domain.ErrNotFound):
			status = http.StatusNotFound
			code = "EXTENSION_PACKAGE_NOT_FOUND"
		case errors.Is(err, extension.ErrDifferentPackageInstalled):
			status = http.StatusConflict
			code = "EXTENSION_VERSION_ALREADY_INSTALLED"
		case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
			status = http.StatusConflict
			code = "EXTENSION_INSTALL_INTEGRITY_FAILED"
		case errors.Is(err, storagehealth.ErrRuntimeReserve):
			status = http.StatusInsufficientStorage
			code = "EXTENSION_INSTALL_STORAGE_RESERVE"
		case errors.Is(err, extension.ErrDependenciesRequired):
			status = http.StatusConflict
			code = "EXTENSION_DEPENDENCIES_REQUIRED"
		case errors.Is(err, extension.ErrDependencyPlanBlocked):
			status = http.StatusConflict
			code = "EXTENSION_DEPENDENCY_PLAN_BLOCKED"
		}
		response["error_code"] = code
		var planErr *extension.InstallPlanError
		if errors.As(err, &planErr) {
			response["plan"] = planErr.Plan
			h.record(r, session, packageID, securityaudit.ResultRejected, code, map[string]any{
				"plan_status": planErr.Plan.Status,
				"install_steps": len(planErr.Plan.Steps),
				"blockers": len(planErr.Plan.Blockers),
			})
		} else {
			h.record(r, session, packageID, securityaudit.ResultRejected, code, nil)
		}
		writeJSON(w, status, response)
		return
	}
	h.record(r, session, item.PackageID, securityaudit.ResultSuccess, "", map[string]any{
		"installation_id": item.InstallationID,
		"extension_id": item.ExtensionID,
		"version": item.Version,
		"lifecycle_state": item.LifecycleState,
	})
	writeJSON(w, http.StatusCreated, item)
}

func (h *operatorExtensionInstaller) record(r *http.Request, session userauth.Session, resourceID, result, reason string, metadata any) {
	if h.audit == nil {
		return
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType: "extension.package.install",
		ActorUserID: session.User.ID,
		ActorUsername: session.User.Username,
		Source: "operator-web",
		ResourceType: "extension_package",
		ResourceID: strings.TrimSpace(resourceID),
		Result: result,
		Reason: reason,
		Metadata: metadata,
	})
}
