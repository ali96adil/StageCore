package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
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
	return withOperatorProjectAccessHooks(auth, access, projects, audit, operatorProjectAccessHooks{})
}

type operatorProjectAccessHooks struct {
	BeforeResponse func()
}

func withOperatorProjectAccessHooks(auth *userauth.Service, access *projectaccess.Service,
	projects *store.Store, audit *securityaudit.Service, hooks operatorProjectAccessHooks) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || access == nil ||
			projects == nil || audit == nil {
			return
		}
		registerProjectAccessRoutes(s.mux, auth, access, projects, audit, hooks)
	}
}

func registerProjectAccessRoutes(mux *http.ServeMux, auth *userauth.Service,
	access *projectaccess.Service, projects *store.Store, audit *securityaudit.Service, hooks operatorProjectAccessHooks) {
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
			if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionUserManage) {
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, map[string]any{
				"project_id": projectID, "user_id": userID, "permissions": grants,
				"access_level": projectaccess.AccessLevelForPermissions(grants),
				"enforced":     false, "phase": "PREPARED_NOT_ENFORCED",
			})
		}))

	mux.HandleFunc("PUT "+pattern, withPermission(auth, userauth.PermissionUserManage,
		func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			userID := strings.TrimSpace(r.PathValue("user_id"))

			// Report an active Session early and recheck in the write transaction.
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
				AccessLevel *string         `json:"access_level"`
			}
			if !decodeBoundedJSON(w, r, &request) {
				return
			}
			var permissions []projectaccess.Permission
			raw := bytes.TrimSpace(request.Permissions)
			if request.AccessLevel != nil {
				if len(raw) != 0 {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_GRANT_INPUT_CONFLICT"})
					return
				}
				level := projectaccess.AccessLevel(strings.ToUpper(strings.TrimSpace(*request.AccessLevel)))
				var err error
				permissions, err = projectaccess.PermissionsForAccessLevel(level)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_ACCESS_LEVEL_INVALID"})
					return
				}
			} else {
				if len(raw) == 0 || raw[0] != '[' {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_EXPLICIT_ARRAY_REQUIRED"})
					return
				}
				if err := json.Unmarshal(raw, &permissions); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_PERMISSIONS_INVALID"})
					return
				}
			}
			if len(permissions) > 32 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_ACCESS_PERMISSIONS_INVALID"})
				return
			}
			// This mutation and its success audit event commit in ONE DB
			// transaction. If audit fails, no Project grant is changed.
			writeCtx := capability.WithWriteAuthorization(r.Context(), func(ctx context.Context, tx *sql.Tx) error {
				role, err := auth.SessionRoleInTransaction(ctx, tx, session.ID, session.User.ID)
				if errors.Is(err, userauth.ErrSessionInvalid) || (err == nil && role != userauth.RoleOwner) {
					return projectaccess.ErrDenied
				}
				if err != nil {
					return err
				}
				active, err = store.ActiveOperationalSessionTypeInTransaction(ctx, tx)
				if err == nil && active != "" {
					return domain.ErrShowConfigurationLocked
				}
				return err
			})
			err = access.SetPermissionsAudited(writeCtx, session.User.ID, userID, projectID, permissions, audit)
			if errors.Is(err, domain.ErrShowConfigurationLocked) {
				writeJSON(w, http.StatusLocked, map[string]any{
					"error_code": "PROJECT_ACCESS_SESSION_ACTIVE", "session_type": active,
				})
				return
			}
			if err != nil {
				writeProjectAccessError(w, err)
				return
			}
			grants, err := access.PermissionsForUser(r.Context(), session.User.ID, userID, projectID)
			if err != nil {
				w.Header().Set("Cache-Control", "no-store")
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"error_code":                     "PROJECT_ACCESS_READBACK_FAILED",
					"effect_may_be_committed":        true,
					"effect_may_have_been_committed": true,
					"retryable":                      false,
				})
				return
			}
			if hooks.BeforeResponse != nil {
				hooks.BeforeResponse()
			}
			if !recheckCommittedProjectMutation(w, r, auth, session, userauth.PermissionUserManage, nil) {
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, map[string]any{
				"project_id": projectID, "user_id": userID, "permissions": grants,
				"access_level": projectaccess.AccessLevelForPermissions(grants),
				"enforced":     false, "phase": "PREPARED_NOT_ENFORCED",
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
