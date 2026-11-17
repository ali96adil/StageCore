package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// WithOwnerPluginAdministration qualifies the Hub-wide extension library,
// installation lifecycle and plugin trust/permissions as Owner operations.
// Project grants never confer administrative authority over shared packages.
func WithOwnerPluginAdministration(auth *userauth.Service) Option {
	return func(s *Server) {
		s.pluginAdminGuard = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if auth == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PLUGIN_AUTH_UNAVAILABLE"})
				return
			}
			withAuthenticatedSession(auth, isUnsafeMethod(r.Method),
				func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
					if session.User.Role != userauth.RoleOwner {
						writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
						return
					}
					s.mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ownerPluginDispatchKey{}, true)))
				})(w, r)
		}
	}
}

type ownerPluginDispatchKey struct{}

func pluginMutationContext(r *http.Request, auth *userauth.Service, session userauth.Session, token string, permission userauth.Permission) context.Context {
	ownerGuard, _ := r.Context().Value(ownerPluginDispatchKey{}).(bool)
	ownerOnly := ownerGuard || qualificationAccess(r, nil) != nil
	csrf := r.Header.Get(csrfHeader)
	dispatch := capability.WithDispatchCheck(r.Context(), func(ctx context.Context) error {
		current, err := auth.ValidateCSRF(ctx, token, csrf)
		if err != nil || current.ID != session.ID || current.User.ID != session.User.ID ||
			userauth.Authorize(current.User.Role, permission) != nil ||
			(ownerOnly && current.User.Role != userauth.RoleOwner) {
			return capability.ErrDispatchAuthorityRevoked
		}
		return nil
	})
	return capability.WithWriteAuthorization(dispatch, func(ctx context.Context, tx *sql.Tx) error {
		role, err := auth.SessionRoleInTransaction(ctx, tx, session.ID, session.User.ID)
		if errors.Is(err, userauth.ErrSessionInvalid) || (err == nil && (userauth.Authorize(role, permission) != nil ||
			(ownerOnly && role != userauth.RoleOwner))) {
			return capability.ErrDispatchAuthorityRevoked
		}
		return err
	})
}

func isHubPluginAdminPattern(pattern string) bool {
	if _, path, ok := strings.Cut(pattern, " "); ok {
		pattern = path
	}
	for _, namespace := range []string{"/api/v1/extensions", "/api/v1/security/plugins"} {
		if pattern == namespace || strings.HasPrefix(pattern, namespace+"/") {
			return true
		}
	}
	return false
}
