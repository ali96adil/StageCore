package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

// WithOperatorProjectGrantsStaging exposes OWNER-only draft access policies.
// STAGING ONLY: no current StageCore project/asset/GO route consults these
// grants. Do not advertise them as effective access restrictions.
func WithOperatorProjectGrantsStaging(auth *userauth.Service, grants *projectaccess.Repository) Option {
	return func(s *Server) {
		if s==nil||s.mux==nil||auth==nil||grants==nil{return}
		const collection = "/api/v1/security/projects/{project_id}/grants/staging"
		const target = "/api/v1/security/projects/{project_id}/grants/staging/{user_id}"
		s.mux.HandleFunc("GET "+collection,withPermission(auth,userauth.PermissionUserManage,
			func(w http.ResponseWriter,r *http.Request,session userauth.Session){
				if session.User.Role!=userauth.RoleOwner {
					writeJSON(w,http.StatusForbidden,map[string]any{"error_code":"OWNER_REQUIRED"})
					return
				}
				result,err:=grants.ListStagedGrants(r.Context(),session.User.ID,r.PathValue("project_id"))
				if err!=nil{writeStagingGrantError(w,err);return}
				w.Header().Set("Cache-Control","no-store")
				writeJSON(w,http.StatusOK,map[string]any{
					"project_id":r.PathValue("project_id"),
					"grants":result,
					"available_capabilities":projectaccess.AvailableCapabilities(),
					"enforced":false,
					"phase":"STAGING_ONLY",
					"current_policy":"GLOBAL_ROLES",
					"warning":"PROJECT_GRANTS_NOT_ENFORCED",
				})
			}))
		s.mux.HandleFunc("PUT "+target,withPermission(auth,userauth.PermissionUserManage,
			func(w http.ResponseWriter,r *http.Request,session userauth.Session){
				if session.User.Role!=userauth.RoleOwner {
					writeJSON(w,http.StatusForbidden,map[string]any{"error_code":"OWNER_REQUIRED"})
					return
				}
				var input struct{Capabilities []string `json:"capabilities"`}
				if !decodeBoundedJSON(w,r,&input){return}
				grant,err:=grants.ReplaceStagedGrant(r.Context(),session.User.ID,
					strings.TrimSpace(r.PathValue("project_id")),
					strings.TrimSpace(r.PathValue("user_id")),input.Capabilities)
				if err!=nil{writeStagingGrantError(w,err);return}
				w.Header().Set("Cache-Control","no-store")
				writeJSON(w,http.StatusOK,map[string]any{
					"grant":grant,
					"enforced":false,
					"phase":"STAGING_ONLY",
					"warning":"PROJECT_GRANTS_NOT_ENFORCED",
				})
			}))
	}
}

func writeStagingGrantError(w http.ResponseWriter,err error){
	status,code:=http.StatusServiceUnavailable,"PROJECT_GRANTS_UNAVAILABLE"
	switch {
	case errors.Is(err,projectaccess.ErrForbidden):
		status,code=http.StatusForbidden,"OWNER_REQUIRED"
	case errors.Is(err,projectaccess.ErrInvalid):
		status,code=http.StatusBadRequest,"PROJECT_GRANTS_INVALID"
	case errors.Is(err,projectaccess.ErrNotFound):
		status,code=http.StatusNotFound,"PROJECT_OR_USER_NOT_FOUND"
	}
	writeJSON(w,status,map[string]any{"error_code":code})
}
