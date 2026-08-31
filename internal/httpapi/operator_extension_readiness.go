package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type operatorExtensionReadiness struct {
	users    *userauth.Service
	assessor *extension.ReadinessAssessor
}

func WithOperatorExtensionReadiness(users *userauth.Service, assessor *extension.ReadinessAssessor) Option {
	return func(s *Server) {
		if users == nil || assessor == nil {
			return
		}
		h := &operatorExtensionReadiness{users: users, assessor: assessor}
		s.mux.HandleFunc("GET /api/v1/extensions/installations/{installation_id}/readiness", withPermission(users, userauth.PermissionProjectRead, h.get))
	}
}

func (h *operatorExtensionReadiness) get(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	assessment, err := h.assessor.Assess(r.Context(), strings.TrimSpace(r.PathValue("installation_id")))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXTENSION_INSTALLATION_NOT_FOUND"})
	case errors.Is(err, extension.ErrInstalledPayloadIntegrity), errors.Is(err, extension.ErrPermissionReviewIntegrity):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXTENSION_READINESS_INTEGRITY_FAILED"})
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_READINESS_UNAVAILABLE"})
	default:
		writeJSON(w, http.StatusOK, assessment)
	}
}
