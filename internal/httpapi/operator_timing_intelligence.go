package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/timingintelligence"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type TimingIntelligenceService interface {
	Report(context.Context, string, timingintelligence.ReportOptions) (timingintelligence.Report, error)
	SetSessionSelection(context.Context, string, string, store.TimingSelectionMode, string) (store.TimingSessionSelection, error)
}

type timingSelectionRequest struct {
	Mode store.TimingSelectionMode `json:"mode"`
}

func WithOperatorTimingIntelligence(auth *userauth.Service, service TimingIntelligenceService) Option {
	return func(s *Server) {
		if auth == nil || service == nil {
			return
		}

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/timing-intelligence", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			leadTime := 30 * time.Second
			if raw := strings.TrimSpace(r.URL.Query().Get("lead_time_seconds")); raw != "" {
				seconds, err := strconv.Atoi(raw)
				if err != nil || seconds < 0 || seconds > 3600 {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_LEAD_TIME"})
					return
				}
				if seconds == 0 {
					leadTime = time.Nanosecond
				} else {
					leadTime = time.Duration(seconds) * time.Second
				}
			}
			fromCue := strings.TrimSpace(r.URL.Query().Get("section_from_cue_id"))
			toCue := strings.TrimSpace(r.URL.Query().Get("section_to_cue_id"))
			if (fromCue == "") != (toCue == "") {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_SECTION"})
				return
			}
			report, err := service.Report(r.Context(), r.PathValue("project_id"), timingintelligence.ReportOptions{
				RuntimeSnapshotID: strings.TrimSpace(r.URL.Query().Get("runtime_snapshot_id")),
				SessionID: strings.TrimSpace(r.URL.Query().Get("session_id")),
				LeadTime: leadTime,
				SectionFromCueID: fromCue,
				SectionToCueID: toCue,
			})
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"report": report})
		}))

		s.mux.HandleFunc("PUT /api/v1/projects/{project_id}/timing-intelligence/sessions/{session_id}", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			var body timingSelectionRequest
			if !decodeBoundedJSON(w, r, &body) {
				return
			}
			selection, err := service.SetSessionSelection(
				r.Context(), r.PathValue("project_id"), r.PathValue("session_id"), body.Mode, session.User.Username,
			)
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"selection": selection})
		}))
	}
}
