package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// SC-024 PHASE 3 (QUALIFICATION ONLY):
// These primitives are intentionally NOT registered on the production Hub.
// Replacing only GET /projects while Cue, media, runtime and download routes
// remain globally authorized would give an incorrect privacy guarantee.
//
// A full server-wide scoped-route rollout must use the same Project policy
// for derived IDs (Cue -> Revision -> Project, media -> Project, etc).
// The production Hub continues using the old role checks until then.

// withScopedProjectRead authenticates the current browser session and checks
// exact Project identity against live DB grants on every request. Intentionally
// returns 404 for *both* missing and unauthorized Project IDs, avoiding a
// difference in presence probing by unauthorized users.
func withScopedProjectRead(auth *userauth.Service, access *projectaccess.Service, next sessionHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
			if access == nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code":"PROJECT_SCOPE_UNAVAILABLE"})
				return
			}
			projectID:=strings.TrimSpace(r.PathValue("project_id"))
			if projectID=="" {
				writeJSON(w, http.StatusNotFound, map[string]any{"error_code":"PROJECT_NOT_FOUND"})
				return
			}
			err:=access.Check(r.Context(),session.User.ID,projectID,projectaccess.ProjectView)
			if errors.Is(err,projectaccess.ErrDenied) || errors.Is(err,projectaccess.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error_code":"PROJECT_NOT_FOUND"})
				return
			}
			if err!=nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code":"PROJECT_SCOPE_UNAVAILABLE"})
				return
			}
			next(w,r,session)
		})(w,r)
	}
}

// scopedProjectList returns only objects belonging to projects for which the
// current account holds a live project.view grant (Owner sees all). The
// display name, project count and metadata for other Projects MUST NOT be
// included in the result. This is the future /projects read path, not yet
// wired into the production routing table.
func scopedProjectList(ctx context.Context, access *projectaccess.Service,
	projectStore *store.Store, userID string) ([]domain.Project, error) {
	if access==nil || projectStore==nil { return nil, errors.New("scoped Project read unavailable") }
	ids,err:=access.AccessibleProjectIDs(ctx,userID)
	if err!=nil {return nil,err}
	allowed:=make(map[string]bool,len(ids))
	for _,id:=range ids{allowed[id]=true}
	if len(allowed)==0{return []domain.Project{},nil}
	all,err:=projectStore.ListProjects(ctx)
	if err!=nil {return nil,err}
	visible:=make([]domain.Project,0,len(ids))
	for _,project:=range all{
		if allowed[project.ID] {visible=append(visible,project)}
	}
	return visible,nil
}
