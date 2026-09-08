package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func WithOperatorConfigurationDraft(auth *userauth.Service, stageStore *store.Store, audits ...*securityaudit.Service) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil {
			return
		}
		var audit *securityaudit.Service
		if len(audits) > 0 {
			audit = audits[0]
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
			if session.User.Role != userauth.RoleOwner {
				if audit != nil {
					_, _ = audit.Append(r.Context(), securityaudit.Event{
						EventType: "project.draft.discard", ActorUserID: session.User.ID, ActorUsername: session.User.Username,
						Source: "operator_web", ResourceType: "project", ResourceID: projectID,
						Result: securityaudit.ResultRejected, Reason: "OWNER role required",
					})
				}
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "OWNER_REQUIRED"})
				return
			}
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
				if audit != nil {
					_, _ = audit.Append(r.Context(), securityaudit.Event{
						EventType: "project.draft.discard", ActorUserID: session.User.ID, ActorUsername: session.User.Username,
						Source: "operator_web", ResourceType: "project", ResourceID: projectID,
						Result: securityaudit.ResultFailed, Reason: err.Error(),
					})
				}
				writeProjectStoreError(w, err)
				return
			}
			if audit != nil {
				if _, err := audit.Append(r.Context(), securityaudit.Event{
					EventType: "project.draft.discard", ActorUserID: session.User.ID, ActorUsername: session.User.Username,
					Source: "operator_web", ResourceType: "project", ResourceID: projectID,
					Result: securityaudit.ResultSuccess, Reason: strings.TrimSpace(input.Reason),
					Metadata: map[string]any{"discarded": discarded, "restored_revision_id": restored.ID},
				}); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]any{"error_code": "AUDIT_RECORD_FAILED"})
					return
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"discarded":        discarded,
				"current_revision": makeRevisionView(restored),
			})
		}))
	}
}
