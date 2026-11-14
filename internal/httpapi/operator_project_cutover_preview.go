package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const ownerProjectCutoverPreviewRoute = "GET /api/v1/project-access/cutover-preview"

type ownerProjectCutoverPreviewHooks struct {
	BeforeResponse func()
	Preview        func(context.Context, string) (projectaccess.CutoverImpactPreview, error)
}

// WithOwnerProjectCutoverPreview exposes the read-only cutover impact report
// to Owners in qualification assemblies. Production stays on its legacy
// assembler until the complete atomic ACL cutover is approved.
func WithOwnerProjectCutoverPreview(auth *userauth.Service, access *projectaccess.Service) Option {
	return withOwnerProjectCutoverPreview(auth, access, ownerProjectCutoverPreviewHooks{})
}

func withOwnerProjectCutoverPreview(auth *userauth.Service, access *projectaccess.Service, hooks ownerProjectCutoverPreviewHooks) Option {
	return func(s *Server) {
		if auth == nil || access == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/project-access/cutover-preview", withPermission(auth, userauth.PermissionUserManage,
			func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
				w.Header().Set("Cache-Control", "no-store")
				if !allowScopedOwner(w, session, access) {
					return
				}
				var preview projectaccess.CutoverImpactPreview
				var err error
				if hooks.Preview != nil {
					preview, err = hooks.Preview(r.Context(), session.User.ID)
				} else {
					preview, err = access.PreviewProjectCutover(r.Context(), session.User.ID)
				}
				if hooks.BeforeResponse != nil {
					hooks.BeforeResponse()
				}
				if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionUserManage) {
					return
				}
				if err != nil {
					switch {
					case errors.Is(err, projectaccess.ErrDenied):
						writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
					case errors.Is(err, projectaccess.ErrCutoverPreviewTooLarge):
						writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error_code": "PROJECT_ACCESS_PREVIEW_REQUIRES_PAGINATION"})
					default:
						writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_ACCESS_PREVIEW_UNAVAILABLE"})
					}
					return
				}
				writeJSON(w, http.StatusOK, preview)
			}))
	}
}
