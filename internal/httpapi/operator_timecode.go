package httpapi

import (
	"context"
	"net/http"

	"github.com/ali96adil/StageCore/internal/timecode"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type TimecodeSummaryReader interface {
	Summary(context.Context, string, string) (timecode.RuntimeSummary, error)
}

func WithOperatorTimecode(auth *userauth.Service, service TimecodeSummaryReader) Option {
	return func(s *Server) {
		if auth == nil || service == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/timecode", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			summary, err := service.Summary(r.Context(), r.PathValue("project_id"), r.URL.Query().Get("runtime_snapshot_id"))
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"summary": summary,
				"supported_source_kinds": []timecode.SourceKind{timecode.SourceInternal, timecode.SourceMTC, timecode.SourceLTC},
				"supported_rates": timecode.SupportedRates(),
				"presentation": map[string]any{
					"en": map[string]string{
						"title": "Timecode & Show Sync",
						"healthy": "Healthy",
						"missing": "Missing",
						"stale": "Stale",
						"automatic_inhibit": "Automatic timecode cues are inhibited until the selected source is healthy.",
					},
					"ar": map[string]string{
						"title": "التايم كود ومزامنة العرض",
						"healthy": "سليم",
						"missing": "غير موجود",
						"stale": "متوقف/قديم",
						"automatic_inhibit": "يتم منع تشغيل الكيوات التلقائية المرتبطة بالتايم كود إلى أن يصبح المصدر المحدد سليماً.",
					},
				},
			})
		}))
	}
}
