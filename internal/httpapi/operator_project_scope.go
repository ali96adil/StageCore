package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func allowScopedProject(w http.ResponseWriter, r *http.Request, session userauth.Session, access *projectaccess.Service, permissions ...projectaccess.Permission) bool {
	if access == nil {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	err := access.CheckAll(r.Context(), session.User.ID, projectID, permissions...)
	switch {
	case errors.Is(err, projectaccess.ErrDenied), errors.Is(err, projectaccess.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_NOT_FOUND"})
		return false
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_ACCESS_UNAVAILABLE"})
		return false
	}
	return true
}

func allowScopedOwner(w http.ResponseWriter, session userauth.Session, access *projectaccess.Service) bool {
	if access == nil {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	if session.User.Role != userauth.RoleOwner {
		writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
		return false
	}
	return true
}
