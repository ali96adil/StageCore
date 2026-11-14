package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func registerCueEditorCheckpoints(mux *http.ServeMux, auth *userauth.Service, st *store.Store, beforeMutationResponse func()) {
	handle := func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		if _, err := currentProjectRouteGrant(r); !writeProjectQualificationError(w, err) {
			return
		}
		projectID, cueID, userID := r.PathValue("project_id"), r.PathValue("cue_id"), session.User.ID
		project, err := st.GetProject(r.Context(), projectID)
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		cue, err := st.GetCue(r.Context(), cueID)
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		revision, err := st.GetRevision(r.Context(), cue.RevisionID)
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		if revision.ProjectID != project.ID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error_code": "NOT_FOUND"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		switch r.Method {
		case http.MethodGet:
			checkpoints, err := st.ListCueEditorCheckpoints(r.Context(), projectID, userID, cueID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectEdit) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"checkpoints": checkpoints})
		case http.MethodPost:
			var request struct {
				Name     string          `json:"name"`
				Document json.RawMessage `json:"document"`
			}
			if !decodeBoundedJSON(w, r, &request) {
				return
			}
			checkpoint, err := st.CreateCueEditorCheckpoint(projectMutationContext(
				r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit,
			), projectID, userID, cueID, request.Name, request.Document)
			if errors.Is(err, domain.ErrInvalidInput) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "INVALID_CUE_EDITOR_CHECKPOINT"})
				return
			}
			if err != nil {
				if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectEdit) {
					return
				}
				writeProjectStoreError(w, err)
				return
			}
			if beforeMutationResponse != nil {
				beforeMutationResponse()
			}
			if !recheckCommittedProjectMutation(w, r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit) {
				return
			}
			writeJSON(w, http.StatusCreated, checkpoint)
		case http.MethodDelete:
			err := st.DeleteCueEditorCheckpoint(projectMutationContext(
				r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit,
			), projectID, userID, cueID, r.PathValue("checkpoint_id"))
			if err != nil {
				if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectEdit) {
					return
				}
				writeProjectStoreError(w, err)
				return
			}
			if beforeMutationResponse != nil {
				beforeMutationResponse()
			}
			if !recheckCommittedProjectMutation(w, r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit) {
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.HandleFunc("GET /api/v1/projects/{project_id}/cues/{cue_id}/editor-checkpoints", withPermission(auth, userauth.PermissionProjectEdit, handle))
	mux.HandleFunc("POST /api/v1/projects/{project_id}/cues/{cue_id}/editor-checkpoints", withPermission(auth, userauth.PermissionProjectEdit, handle))
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/cues/{cue_id}/editor-checkpoints/{checkpoint_id}", withPermission(auth, userauth.PermissionProjectEdit, handle))
}
