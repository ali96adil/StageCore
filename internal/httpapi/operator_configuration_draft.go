package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func WithOperatorConfigurationDraft(auth *userauth.Service, stageStore *store.Store) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/configuration/lock", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			lock, err := stageStore.ShowConfigurationLockState(r.Context(), projectID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"show_configuration_lock": lock})
		}))
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/configuration/draft", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			lock, err := stageStore.ShowConfigurationLockState(r.Context(), projectID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			if lock.Locked {
				writeJSON(w, http.StatusLocked, map[string]any{"error_code": "SHOW_CONFIGURATION_LOCKED", "show_configuration_lock": lock})
				return
			}
			revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Operator routing edit")
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"draft_revision": makeRevisionView(revision)})
		}))
		s.mux.HandleFunc("DELETE /api/v1/projects/{project_id}/configuration/draft", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			lock, err := stageStore.ShowConfigurationLockState(r.Context(), projectID)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			if lock.Locked {
				writeJSON(w, http.StatusLocked, map[string]any{"error_code": "SHOW_CONFIGURATION_LOCKED", "show_configuration_lock": lock})
				return
			}
			var input struct {
				Reason string `json:"reason"`
			}
			if r.Body != nil {
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err != io.EOF {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_REQUEST"})
					return
				}
			}
			restored, discarded, err := stageStore.DiscardProjectDraft(r.Context(), projectID, session.User.ID, input.Reason)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"discarded":        discarded,
				"current_revision": makeRevisionView(restored),
			})
		}))
	}
}
