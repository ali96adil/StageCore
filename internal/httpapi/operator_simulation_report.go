package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulationreport"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// WithOperatorSimulationReport exposes the read-only F-024 report for a
// specific SIMULATION Session. Session identity is explicit so completed runs
// remain inspectable instead of disappearing when another Session becomes
// active.
func WithOperatorSimulationReport(auth *userauth.Service, reports *simulationreport.Service) Option {
	return func(s *Server) {
		if auth == nil || reports == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/simulation/report/{session_id}", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			projectID := strings.TrimSpace(r.PathValue("project_id"))
			sessionID := strings.TrimSpace(r.PathValue("session_id"))
			report, err := reports.Generate(r.Context(), sessionID)
			if err != nil {
				writeSimulationReportError(w, err)
				return
			}
			if report.ProjectID != projectID {
				writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "SIMULATION_REPORT_NOT_FOUND"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"report": report})
		}))
	}
}

func writeSimulationReportError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	code := "SIMULATION_REPORT_FAILED"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "SIMULATION_REPORT_NOT_FOUND"
	case errors.Is(err, domain.ErrInvalidInput):
		status, code = http.StatusBadRequest, "INVALID_SIMULATION_REPORT_REQUEST"
	case errors.Is(err, domain.ErrConflict):
		status, code = http.StatusConflict, "SIMULATION_REPORT_CONFLICT"
	}
	writeJSON(w, status, map[string]any{"error_code": code, "message": err.Error()})
}
