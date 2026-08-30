package httpapi

import (
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
	}
}
