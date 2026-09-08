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
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/configuration/lock", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			lock, err := stageStore.ShowConfigurationLockState(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "CONFIGURATION_LOCK_READ_FAILED"})
				return
			}
			writeJSON(w, http.StatusOK, lock)
		}))
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/configuration/draft", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			lock, err := stageStore.ShowConfigurationLockState(r.Context(), projectID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "CONFIGURATION_LOCK_READ_FAILED"})
				return
			}
			if lock.Locked {
				writeJSON(w, http.StatusLocked, map[string]any{"error": "SHOW_CONFIGURATION_LOCKED", "lock": lock})
				return
			}
			revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Operator routing edit")
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "DRAFT_CREATE_FAILED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"draft_revision": makeRevisionView(revision)})
		}))
		s.mux.HandleFunc("DELETE /api/v1/projects/{project_id}/configuration/draft", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			var input struct {
				Reason string `json:"reason"`
			}
			if r.Body != nil {
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil && err != io.EOF {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": "INVALID_REQUEST"})
					return
				}
			}
			restored, discarded, err := stageStore.DiscardProjectDraft(r.Context(), projectID, session.User.ID, input.Reason)
			if err != nil {
				status := http.StatusConflict
				if lock, lockErr := stageStore.ShowConfigurationLockState(r.Context(), projectID); lockErr == nil && lock.Locked {
					status = http.StatusLocked
				}
				writeJSON(w, status, map[string]any{"error": "DRAFT_DISCARD_FAILED", "detail": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"discarded":        discarded,
				"current_revision": makeRevisionView(restored),
			})
		}))
	}
}
