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

// Direct-ID metadata resolves the current audited assignment. When a binding
// was captured before loading metadata, it must match again before delivery.
func allowScopedDeviceMetadata(w http.ResponseWriter, r *http.Request, session userauth.Session, access *projectaccess.Service, expectedProject string) (string, bool) {
	if access == nil {
		return "", true
	}
	w.Header().Set("Cache-Control", "no-store")
	if session.User.Role == userauth.RoleOwner {
		return "", true
	}
	ctx := r.Context()
	id := r.PathValue("device_id")
	projectID, err := access.ResolveActiveV2DeviceProject(ctx, id)
	if err == nil {
		if expectedProject != "" && projectID != expectedProject {
			err = projectaccess.ErrDenied
		} else {
			err = access.Check(ctx, session.User.ID, projectID, projectaccess.ProjectView)
		}
	}
	switch {
	case errors.Is(err, projectaccess.ErrDenied), errors.Is(err, projectaccess.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "STAGE_DEVICE_NOT_FOUND"})
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_ACCESS_UNAVAILABLE"})
	default:
		return projectID, true
	}
	return "", false
}

func reauthorizeScopedDeviceMetadata(w http.ResponseWriter, r *http.Request, auth *userauth.Service,
	session userauth.Session, access *projectaccess.Service, expectedProject string, permission userauth.Permission,
) bool {
	w.Header().Set("Cache-Control", "no-store")
	token, ok := browserSessionToken(r)
	current, err := auth.ValidatePassive(r.Context(), token)
	if !ok || err != nil || current.ID != session.ID || current.User.ID != session.User.ID {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": "SESSION_INVALID"})
		return false
	}
	if userauth.Authorize(current.User.Role, permission) != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
		return false
	}
	_, allowed := allowScopedDeviceMetadata(w, r, current, access, expectedProject)
	return allowed
}
