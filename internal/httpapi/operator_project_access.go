package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// WithOperatorProjectAccess adds ONLY Owner grant administration. This
// intentionally does not switch any existing read/Cue/GO/Vault routes to the
// prospective Project ACL; that requires a comprehensive security cutover.
// The API declares this explicitly on every successful response.
func WithOperatorProjectAccess(auth *userauth.Service, access *projectaccess.Service,
	projects *store.Store, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || access == nil ||
			projects == nil || audit == nil {
			return
		}
		registerProjectAccessRoutes(s.mux, auth, access, projects, audit)
	}
}

func registerProjectAccessRoutes(mux *http.ServeMux, auth *userauth.Service,
	access *projectaccess.Service, projects *store.Store, audit *securityaudit.Service) {
	const pattern = "/api/v1/projects/{project_id}/access/users/{user_id}"

	mux.HandleFunc("GET "+pattern, withPermission(auth, userauth.PermissionUserManage,
		func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			userID := strings.TrimSpace(r.PathValue("user_id"))
			grants, err := access.PermissionsForUser(r.Context(), session.User.ID, userID, projectID)
			if err != nil {
				writeProjectAccessError(w, err)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, map[string]any{
				"project_id": projectID, "user_id": userID, "permissions": grants,
				"enforced": false, "phase": "PREPARED_NOT_ENFORCED",
			})
		}))

	mux.HandleFunc("PUT "+pattern, withPermission(auth, userauth.PermissionUserManage,
		func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			userID := strings.TrimSpace(r.PathValue("user_id"))

			// Freeze changes whenever an operational Session is active. The
			// future cutover must introduce an atomic SHOW/GO authority fence
			// rather than a simplistic stale read of this status.
			active, err := projects.ActiveOperationalSessionType(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_ACCESS_SESSION_UNKNOWN"})
				return
			}
			if active != "" {
				writeJSON(w, http.StatusLocked, map[string]any{
					"error_code": "PROJECT_ACCESS_SESSION_ACTIVE", "session_type": active,
				})
				return
			}

			var request struct {
				Permissions json.RawMessage `json:"permissions"`
			}
			if !decodeBoundedJSON(w, r, &request) {
				return
			}
			raw := bytes.TrimSpace(request.Permissions)
			if len(raw) == 0 || raw[0] != '[' {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_EXPLICIT_ARRAY_REQUIRED"})
				return
			}
			var permissions []projectaccess.Permission
			if err := json.Unmarshal(raw, &permissions); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_PERMISSIONS_INVALID"})
				return
			}
			if len(permissions) > 32 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_PERMISSIONS_INVALID"})
				return
			}
			// This mutation and its success audit event commit in ONE DB
			// transaction. If audit fails, no Project grant is changed.
			err = access.SetPermissionsAudited(r.Context(), session.User.ID, userID, projectID, permissions, audit)
			if err != nil {
				writeProjectAccessError(w, err)
				return
			}
			grants, err := access.PermissionsForUser(r.Context(), session.User.ID, userID, projectID)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_ACCESS_READBACK_FAILED"})
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, map[string]any{
				"project_id": projectID, "user_id": userID, "permissions": grants,
				"enforced": false, "phase": "PREPARED_NOT_ENFORCED",
			})
		}))
}

func writeProjectAccessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, projectaccess.ErrDenied):
		writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "PROJECT_ACCESS_DENIED"})
	case errors.Is(err, projectaccess.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_ACCESS_TARGET_NOT_FOUND"})
	case errors.Is(err, projectaccess.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_INVALID"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_ACCESS_UNAVAILABLE"})
	}
}
