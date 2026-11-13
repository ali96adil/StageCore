package httpapi

import (
	"errors"
	"net/http"

	"github.com/ali96adil/StageCore/internal/domain"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func registerCueEditorRecovery(mux *http.ServeMux, auth *userauth.Service, st *store.Store, beforeMutationResponse func()) {
	handle := func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		if _, err := currentProjectRouteGrant(r); !writeProjectQualificationError(w, err) {
			return
		}
		projectID := r.PathValue("project_id")
		if _, err := st.GetProject(r.Context(), projectID); err != nil {
			writeProjectStoreError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		var recovery store.CueEditorRecovery
		var err error
		switch r.Method {
		case http.MethodGet:
			recovery, err = st.CueEditorRecovery(r.Context(), projectID, session.User.ID)
		case http.MethodPut:
			var request store.CueEditorRecovery
			if !decodeBoundedJSON(w, r, &request) {
				return
			}
			recovery, err = st.SaveCueEditorRecovery(projectMutationContext(r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit), projectID, session.User.ID, request.Generation, request.Document)
		case http.MethodDelete:
			err = st.DeleteCueEditorRecovery(projectMutationContext(r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit), projectID, session.User.ID, r.Header.Get("If-Match"))
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "INVALID_CUE_RECOVERY"})
			return
		}
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		if r.Method == http.MethodGet && !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectEdit) {
			return
		}
		if r.Method != http.MethodGet && beforeMutationResponse != nil {
			beforeMutationResponse()
		}
		if r.Method != http.MethodGet && !recheckCommittedProjectMutation(w, r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit) {
			return
		}
		writeJSON(w, http.StatusOK, recovery)
	}
	mux.HandleFunc("GET /api/v1/projects/{project_id}/cue-editor-recovery", withPermission(auth, userauth.PermissionProjectEdit, handle))
	mux.HandleFunc("PUT /api/v1/projects/{project_id}/cue-editor-recovery", withPermission(auth, userauth.PermissionProjectEdit, handle))
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/cue-editor-recovery", withPermission(auth, userauth.PermissionProjectEdit, handle))
}

// Recheck the exact browser session and current role independently of staged ACL.
func validateLiveProjectBrowser(r *http.Request, auth *userauth.Service, session userauth.Session, permission userauth.Permission) error {
	token, _ := browserSessionToken(r)
	current, err := auth.ValidatePassive(r.Context(), token)
	if err != nil || current.ID != session.ID || current.User.ID != session.User.ID {
		return userauth.ErrSessionInvalid
	}
	if err := userauth.Authorize(current.User.Role, permission); err != nil &&
		!scopedProjectRoleMayUsePermission(r, current.User.Role, permission) {
		return err
	}
	return nil
}

func allowLiveProjectRead(w http.ResponseWriter, r *http.Request, auth *userauth.Service, session userauth.Session, permission userauth.Permission) bool {
	w.Header().Set("Cache-Control", "no-store")
	switch err := validateLiveProjectBrowser(r, auth, session, permission); {
	case errors.Is(err, userauth.ErrSessionInvalid):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error_code": "SESSION_INVALID"})
		return false
	case err != nil:
		writeJSON(w, http.StatusForbidden, map[string]string{"error_code": "FORBIDDEN"})
		return false
	}
	_, err := currentProjectRouteGrant(r)
	return writeProjectQualificationError(w, err)
}
