package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// WithScopedProjectFilesRead registers a NEW, independently scoped,
// read-only Project Files surface when explicitly opted in by the Hub
// assembler. Existing global /projects, Vault, Capsule and GO routes are
// intentionally not altered and this Option is NOT enabled by default.
//
// Never replace this with a global role check or use a caller supplied
// user/project identity without a validated browser session.
func WithScopedProjectFilesRead(auth *userauth.Service, access *projectaccess.Service) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || access == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/files/availability",
			withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
				projectID := strings.TrimSpace(r.PathValue("project_id"))
				if projectID == "" {
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_FILES_NOT_FOUND"})
					return
				}
				if !checkFilesReadGrant(w, r, access, session.User.ID, projectID) {
					return
				}
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusNoContent)
			}))

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/files",
			withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
				projectID := strings.TrimSpace(r.PathValue("project_id"))
				if projectID == "" {
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_FILES_NOT_FOUND"})
					return
				}
				// ListScopedProjectFiles rechecks live grants on both sides of
				// its database reads; no raw filesystem path is returned.
				workspace, err := access.ListScopedProjectFiles(r.Context(), session.User.ID, projectID)
				switch {
				case errors.Is(err, projectaccess.ErrDenied), errors.Is(err, projectaccess.ErrNotFound):
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_FILES_NOT_FOUND"})
					return
				case errors.Is(err, projectaccess.ErrWorkspacePaginationRequired):
					writeJSON(w, http.StatusConflict, map[string]any{"error_code": "PROJECT_FILES_PAGINATION_REQUIRED"})
					return
				case err != nil:
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_FILES_UNAVAILABLE"})
					return
				}
				w.Header().Set("Cache-Control", "no-store")
				writeJSON(w, http.StatusOK, map[string]any{
					"workspace": workspace,
					"read_only": true,
					"scoped": true,
				})
			}))
	}
}

func checkFilesReadGrant(w http.ResponseWriter, r *http.Request,
	access *projectaccess.Service, userID, projectID string) bool {
	err := access.Check(r.Context(), userID, projectID, projectaccess.MediaRead)
	if err == nil {
		return true
	}
	if errors.Is(err, projectaccess.ErrDenied) || errors.Is(err, projectaccess.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_FILES_NOT_FOUND"})
		return false
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_FILES_UNAVAILABLE"})
	return false
}
