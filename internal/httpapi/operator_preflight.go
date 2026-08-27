package httpapi

import (
	"net/http"

	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func WithOperatorPreflight(auth *userauth.Service, service *preflight.Service) Option {
	return func(s *Server) {
		if auth == nil || service == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/preflight", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			report, err := service.Evaluate(r.Context(), r.PathValue("project_id"), r.URL.Query().Get("runtime_snapshot_id"))
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, report)
		}))
	}
}
