package httpapi

import (
	"errors"
	"net/http"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// WithOwnerProjectCutoverPreview adds a strictly opt-in GET-only Owner
// migration/role impact report. This is NOT the ACL switch. The actual Hub
// assembler intentionally does not enable it and legacy routes keep their
// current authorization policy. No grant replacement, GO or STOP occurs.
func WithOwnerProjectCutoverPreview(auth *userauth.Service, access *projectaccess.Service) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || access == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/security/project-cutover/preview",
			withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
				w.Header().Set("Cache-Control", "no-store")
				if session.User.Role != userauth.RoleOwner ||
					userauth.Authorize(session.User.Role, userauth.PermissionUserManage) != nil {
					writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "PROJECT_CUTOVER_OWNER_REQUIRED"})
					return
				}
				report, err := access.PreviewProjectCutover(r.Context(), session.User.ID)
				switch {
				case errors.Is(err, projectaccess.ErrDenied):
					writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "PROJECT_CUTOVER_OWNER_REQUIRED"})
				case errors.Is(err, projectaccess.ErrCutoverPreviewTooLarge):
					writeJSON(w, http.StatusConflict, map[string]any{"error_code": "PROJECT_CUTOVER_PAGINATION_REQUIRED"})
				case err != nil:
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_CUTOVER_PREVIEW_UNAVAILABLE"})
				default:
					writeJSON(w, http.StatusOK, report)
				}
			}))
	}
}
