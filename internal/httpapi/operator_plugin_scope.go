package httpapi

import (
	"net/http"
	"strings"

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
					s.mux.ServeHTTP(w, r)
				})(w, r)
		}
	}
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
