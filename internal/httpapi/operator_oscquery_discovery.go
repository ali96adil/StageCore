package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// Discovery is read-only. The browser cannot choose arbitrary network endpoints
// or another project's Companion: both are resolved by the Hub.
type oscQueryDiscoveryRuntime interface {
	DiscoverOSCQuery(context.Context, string, string) companionchannel.OSCQueryDiscoveryResult
}

func registerOperatorOSCQueryDiscoveryRoute(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, runtime oscQueryDiscoveryRuntime) {
	path := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/oscquery/discover"
	mux.HandleFunc("POST "+path, withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		environment, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), strings.TrimSpace(r.PathValue("execution_environment_id")))
		if err != nil || environment.RevisionID != revision.ID || environment.Manifest.AdapterKey != "stagecore.adapter.vdmx" {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "VDMX_ENVIRONMENT_NOT_FOUND"})
			return
		}
		if environment.MachineRoleID == nil || strings.TrimSpace(*environment.MachineRoleID) == "" {
			writeJSON(w, http.StatusConflict, map[string]any{"error_code": "VDMX_MACHINE_ROLE_NOT_BOUND"})
			return
		}
		role, err := stageStore.GetMachineRole(r.Context(), *environment.MachineRoleID)
		if err != nil || role.ProjectID != strings.TrimSpace(r.PathValue("project_id")) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MACHINE_ROLE_NOT_FOUND"})
			return
		}
		assignment, err := stageStore.GetActiveRoleAssignment(r.Context(), role.ID)
		if err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, domain.ErrNotFound) {
				status = http.StatusConflict
			}
			writeJSON(w, status, map[string]any{"error_code": "VDMX_COMPANION_NOT_ASSIGNED"})
			return
		}
		// The OSCQuery endpoint is explicitly local to the VDMX Companion.
		// Never proxy the browser-supplied URL to a workstation.
		if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectEdit) {
			return
		}
		ctx := capability.WithDispatchCheck(r.Context(), func(ctx context.Context) error {
			request := r.WithContext(ctx)
			if err := validateLiveProjectBrowser(request, auth, session, userauth.PermissionProjectEdit); err != nil {
				return err
			}
			if _, err := currentProjectRouteGrant(request); err != nil {
				return err
			}
			active, err := stageStore.GetActiveRoleAssignment(ctx, role.ID)
			if err != nil || active.ID != assignment.ID || active.CompanionID != assignment.CompanionID {
				return capability.ErrDispatchAuthorityRevoked
			}
			return nil
		})
		result := runtime.DiscoverOSCQuery(ctx, assignment.CompanionID, "http://127.0.0.1:2345/")
		if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectEdit) {
			return
		}
		if result.Status != "COMPLETED" {
			writeJSON(w, http.StatusServiceUnavailable, result)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
}
