package httpapi

import (
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func registerOutputDraftHistoryRoutes(mux *http.ServeMux, auth *userauth.Service, st *store.Store, hooks operatorConfigurationHooks) {
	handle := func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		if _, err := currentProjectRouteGrant(r); !writeProjectQualificationError(w, err) {
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet {
			ctx := store.WithOutputDraftHistoryActor(r.Context(), session.User.ID)
			status, err := st.OutputDraftHistory(ctx, projectID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectEdit) {
				return
			}
			writeJSON(w, http.StatusOK, status)
			return
		}

		var body routeDraftHistoryRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		writeCtx := store.WithOutputDraftHistoryActor(projectMutationContext(
			r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.ProjectEdit,
		), session.User.ID)
		var status store.OutputDraftHistoryStatus
		var err error
		switch strings.ToUpper(strings.TrimSpace(body.Direction)) {
		case "UNDO":
			status, err = st.UndoOutputDraft(writeCtx, projectID)
		case "REDO":
			status, err = st.RedoOutputDraft(writeCtx, projectID)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "INVALID_OUTPUT_DRAFT_HISTORY_DIRECTION"})
			return
		}
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		if hooks.BeforeResponse != nil {
			hooks.BeforeResponse()
		}
		if !recheckCommittedProjectMutation(w, r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.ProjectEdit) {
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
	mux.HandleFunc("GET /api/v1/projects/{project_id}/output-draft-history", withPermission(auth, userauth.PermissionProjectEdit, handle))
	mux.HandleFunc("POST /api/v1/projects/{project_id}/output-draft-history", withPermission(auth, userauth.PermissionProjectEdit, handle))
}
