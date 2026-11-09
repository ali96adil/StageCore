package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/timingintelligence"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type TimingIntelligenceService interface {
	Report(context.Context, string, timingintelligence.ReportOptions) (timingintelligence.Report, error)
	SetSessionSelection(context.Context, string, string, store.TimingSelectionMode, string) (store.TimingSessionSelection, error)
	SetTimingCapturePaused(context.Context, string, string, string, bool) error
	SetTimingSegment(context.Context, string, string, string, store.TimingSegment) error
}

type timingSegmentRequest struct {
	First    int64 `json:"first"`
	Last     int64 `json:"last"`
	Excluded *bool `json:"excluded"`
}

type timingSelectionRequest struct {
	Segment *timingSegmentRequest     `json:"segment,omitempty"`
	Mode    store.TimingSelectionMode `json:"mode,omitempty"`
	Paused  *bool                     `json:"paused,omitempty"`
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
				SessionID:         strings.TrimSpace(r.URL.Query().Get("session_id")),
				ReviewSessionID:   strings.TrimSpace(r.URL.Query().Get("review_session_id")),
				LeadTime:          leadTime,
				SectionFromCueID:  fromCue,
				SectionToCueID:    toCue,
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
			if _, err := currentProjectRouteGrant(r); !writeProjectQualificationError(w, err) {
				return
			}
			if body.Segment != nil {
				if body.Mode != "" || body.Paused != nil || body.Segment.Excluded == nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "TIMING_MUTATION_AMBIGUOUS"})
					return
				}
				segment := store.TimingSegment{First: body.Segment.First, Last: body.Segment.Last, Excluded: *body.Segment.Excluded}
				err := service.SetTimingSegment(r.Context(), r.PathValue("project_id"), r.PathValue("session_id"), session.User.Username, segment)
				switch {
				case errors.Is(err, domain.ErrInvalidInput):
					writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "INVALID_TIMING_SEGMENT"})
					return
				case errors.Is(err, domain.ErrShowConfigurationLocked):
					writeJSON(w, http.StatusConflict, map[string]string{"error_code": "SHOW_CONFIGURATION_LOCKED"})
					return
				case err != nil:
					writeProjectStoreError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"segment": segment})
				return
			}
			if body.Paused != nil {
				if body.Mode != "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error_code": "TIMING_MUTATION_AMBIGUOUS"})
					return
				}
				if err := service.SetTimingCapturePaused(r.Context(), r.PathValue("project_id"), r.PathValue("session_id"), session.User.Username, *body.Paused); err != nil {
					writeProjectStoreError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, map[string]bool{"capture_paused": *body.Paused})
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
