package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type PreflightEvaluator interface {
	Evaluate(context.Context, string, string) (preflight.Report, error)
}

func WithOperatorPreflight(auth *userauth.Service, service PreflightEvaluator) Option {
	return func(s *Server) {
		if auth == nil || service == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/preflight", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			report, err := service.Evaluate(r.Context(), r.PathValue("project_id"), r.URL.Query().Get("runtime_snapshot_id"))
			if err != nil {
				// Errors can include Project diagnostics too. An in-flight grant
				// revocation must win before exposing any error detail.
				if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectRead) {
					return
				}
				writeProjectStoreError(w, err)
				return
			}
			grant, err := currentProjectRouteGrant(r)
			if !writeProjectQualificationError(w, err) {
				return
			}
			if grant.access != nil {
				err := grant.access.Check(r.Context(), grant.user.User.ID, r.PathValue("project_id"), projectaccess.MediaRead)
				if errors.Is(err, projectaccess.ErrDenied) {
					report.Media = []preflight.MediaStatus{}
					report.Checks = slices.DeleteFunc(slices.Clone(report.Checks), func(check preflight.Check) bool {
						return check.Category == "media"
					})
				} else if !writeProjectQualificationError(w, err) {
					return
				}
			}
			if !allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectRead) {
				return
			}
			writeJSON(w, http.StatusOK, report)
		}))
	}
}
