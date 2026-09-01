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

type extensionUpdateRequest struct {
	TargetPackageID string `json:"target_package_id"`
}

func registerOperatorExtensionMaintenanceRoutes(mux *http.ServeMux, users *userauth.Service, supervisor *extension.RuntimeSupervisor, audit *securityaudit.Service) {
	if mux == nil || users == nil || supervisor == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/extensions/installations/{installation_id}/update-plan", withPermission(users, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		installationID := strings.TrimSpace(r.PathValue("installation_id"))
		targetPackageID := strings.TrimSpace(r.URL.Query().Get("target_package_id"))
		if targetPackageID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXTENSION_UPDATE_TARGET_REQUIRED"})
			return
		}
		plan, err := supervisor.PlanUpdate(r.Context(), installationID, targetPackageID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXTENSION_UPDATE_RESOURCE_NOT_FOUND"})
		case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
			writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXTENSION_INSTALL_INTEGRITY_FAILED"})
		case err != nil:
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_UPDATE_PLAN_UNAVAILABLE"})
		default:
			writeJSON(w, http.StatusOK, plan)
		}
	}))

	mux.HandleFunc("POST /api/v1/extensions/installations/{installation_id}/update", withPermission(users, userauth.PermissionPluginManage, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		installationID := strings.TrimSpace(r.PathValue("installation_id"))
		var body extensionUpdateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		body.TargetPackageID = strings.TrimSpace(body.TargetPackageID)
		if body.TargetPackageID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXTENSION_UPDATE_TARGET_REQUIRED"})
			return
		}

		item, err := supervisor.UpdateInstallation(r.Context(), installationID, body.TargetPackageID, session.User.Username)
		if err != nil {
			status := http.StatusServiceUnavailable
			code := "EXTENSION_UPDATE_FAILED"
			response := map[string]any{}
			eventType := "extension.package.update"
			var planErr *extension.UpdatePlanError
			if errors.As(err, &planErr) {
				response["plan"] = planErr.Plan
				if planErr.Plan.Direction == extension.UpdateDirectionRollback {
					eventType = "extension.package.rollback"
				}
			}
			switch {
			case errors.Is(err, domain.ErrShowConfigurationLocked):
				status = http.StatusConflict
				code = "SHOW_CONFIGURATION_LOCKED"
			case errors.Is(err, domain.ErrNotFound):
				status = http.StatusNotFound
				code = "EXTENSION_UPDATE_RESOURCE_NOT_FOUND"
			case errors.Is(err, extension.ErrExtensionRuntimeMustBeDisabled):
				status = http.StatusConflict
				code = "EXTENSION_RUNTIME_MUST_BE_DISABLED"
			case errors.Is(err, extension.ErrUpdateDependenciesRequired):
				status = http.StatusConflict
				code = "EXTENSION_UPDATE_DEPENDENCIES_REQUIRED"
			case errors.Is(err, extension.ErrUpdatePlanBlocked):
				status = http.StatusConflict
				code = "EXTENSION_UPDATE_PLAN_BLOCKED"
			case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
				status = http.StatusConflict
				code = "EXTENSION_INSTALL_INTEGRITY_FAILED"
			case errors.Is(err, storagehealth.ErrRuntimeReserve):
				status = http.StatusInsufficientStorage
				code = "EXTENSION_UPDATE_STORAGE_RESERVE"
			}
			response["error_code"] = code
			metadata := map[string]any{"target_package_id": body.TargetPackageID}
			if planErr != nil {
				metadata["direction"] = planErr.Plan.Direction
				metadata["plan_status"] = planErr.Plan.Status
				metadata["install_steps"] = len(planErr.Plan.Steps)
				metadata["blockers"] = len(planErr.Plan.Blockers)
			}
			recordExtensionMaintenance(r, audit, session, eventType, installationID, securityaudit.ResultRejected, code, metadata)
			writeJSON(w, status, response)
			return
		}

		eventType := "extension.package.update"
		if item.Direction == extension.UpdateDirectionRollback {
			eventType = "extension.package.rollback"
		}
		recordExtensionMaintenance(r, audit, session, eventType, installationID, securityaudit.ResultSuccess, "", map[string]any{
			"extension_id": item.ExtensionID,
			"direction": item.Direction,
			"from_package_id": item.FromPackageID,
			"from_version": item.FromVersion,
			"to_package_id": item.ToPackageID,
			"to_version": item.ToVersion,
			"cleanup_warning": item.CleanupWarning,
		})
		writeJSON(w, http.StatusOK, item)
	}))

	mux.HandleFunc("POST /api/v1/extensions/installations/{installation_id}/repair", withPermission(users, userauth.PermissionPluginManage, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		installationID := strings.TrimSpace(r.PathValue("installation_id"))
		item, err := supervisor.RepairInstallation(r.Context(), installationID, session.User.Username)
		if err != nil {
			status := http.StatusServiceUnavailable
			code := "EXTENSION_REPAIR_FAILED"
			switch {
			case errors.Is(err, domain.ErrShowConfigurationLocked):
				status = http.StatusConflict
				code = "SHOW_CONFIGURATION_LOCKED"
			case errors.Is(err, domain.ErrNotFound):
				status = http.StatusNotFound
				code = "EXTENSION_INSTALLATION_NOT_FOUND"
			case errors.Is(err, extension.ErrExtensionRuntimeMustBeDisabled):
				status = http.StatusConflict
				code = "EXTENSION_RUNTIME_MUST_BE_DISABLED"
			case errors.Is(err, extension.ErrInstalledPayloadIntegrity):
				status = http.StatusConflict
				code = "EXTENSION_REPAIR_INTEGRITY_FAILED"
			case errors.Is(err, storagehealth.ErrRuntimeReserve):
				status = http.StatusInsufficientStorage
				code = "EXTENSION_REPAIR_STORAGE_RESERVE"
			}
			recordExtensionMaintenance(r, audit, session, "extension.package.repair", installationID, securityaudit.ResultRejected, code, nil)
			writeJSON(w, status, map[string]any{"error_code": code})
			return
		}
		recordExtensionMaintenance(r, audit, session, "extension.package.repair", installationID, securityaudit.ResultSuccess, "", map[string]any{
			"package_id": item.PackageID,
			"extension_id": item.ExtensionID,
			"version": item.Version,
			"already_healthy": item.AlreadyHealthy,
			"payload_repaired": item.PayloadRepaired,
		})
		writeJSON(w, http.StatusOK, item)
	}))
}

func recordExtensionMaintenance(r *http.Request, audit *securityaudit.Service, session userauth.Session, eventType, installationID, result, reason string, metadata any) {
	if audit == nil {
		return
	}
	_, _ = audit.Append(r.Context(), securityaudit.Event{
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
