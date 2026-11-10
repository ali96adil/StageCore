package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func registerCueActionPresets(mux *http.ServeMux, auth *userauth.Service, st *store.Store) {
	handle := func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		if _, err := currentProjectRouteGrant(r); !writeProjectQualificationError(w, err) {
			return
		}
		ctx, projectID, userID := r.Context(), r.PathValue("project_id"), session.User.ID
		if _, err := st.GetProject(ctx, projectID); err != nil {
			writeProjectStoreError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		switch r.Method {
		case http.MethodGet:
			presets, err := st.ListCueActionPresets(ctx, projectID, userID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			if !allowCuePrivateRead(w, r, auth, session) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"presets": presets})
		case http.MethodPost:
			var request struct {
				Name   string             `json:"name"`
				Action actionWriteRequest `json:"action"`
			}
			if !decodeBoundedJSON(w, r, &request) {
				return
			}
			request.Name = strings.TrimSpace(request.Name)
			if request.Name == "" || utf8.RuneCountInString(request.Name) > 64 ||
				strings.ContainsFunc(request.Name, unicode.IsControl) ||
				strings.TrimSpace(request.Action.CapabilityKey) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "INVALID_CUE_ACTION_PRESET"})
				return
			}
			request.Action.ActionID = ""
			request.Action.OrderIndex = 0
			document, err := json.Marshal(request.Action)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "INVALID_CUE_ACTION_PRESET"})
				return
			}
			preset, err := st.CreateCueActionPreset(projectMutationContext(r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit), projectID, userID, request.Name, document)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, preset)
		case http.MethodDelete:
			if err := st.DeleteCueActionPreset(projectMutationContext(r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.CueEdit), projectID, userID, r.PathValue("preset_id")); err != nil {
				writeProjectStoreError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.HandleFunc("GET /api/v1/projects/{project_id}/cue-action-presets", withPermission(auth, userauth.PermissionProjectEdit, handle))
	mux.HandleFunc("POST /api/v1/projects/{project_id}/cue-action-presets", withPermission(auth, userauth.PermissionProjectEdit, handle))
	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/cue-action-presets/{preset_id}", withPermission(auth, userauth.PermissionProjectEdit, handle))
}
