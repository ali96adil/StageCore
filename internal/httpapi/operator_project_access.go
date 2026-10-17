package httpapi

import (
 "errors"
 "net/http"
 "strings"

 "github.com/ali96adil/StageCore/internal/userauth"
)

type projectAccessRequest struct {
 Capabilities []userauth.ProjectCapability `json:"capabilities"`
}

// WithOperatorProjectAccess adds Owner-only *grant administration* APIs.
// PHASE 1 IS NOT ENFORCEMENT: project listing/Cues/Vault/GO still use legacy
// global roles until all project routes are audited and safely migrated.
// Responses make this explicit to avoid implying that grants protect files.
func WithOperatorProjectAccess(auth *userauth.Service) Option {
 return func(s *Server) {
  if auth==nil{return}
  s.mux.HandleFunc("GET /api/v1/projects/{project_id}/access",
   withPermission(auth,userauth.PermissionUserManage,
    func(w http.ResponseWriter,r *http.Request,session userauth.Session){
     if session.User.Role!=userauth.RoleOwner {
      writeJSON(w,http.StatusForbidden,map[string]any{"error_code":"OWNER_REQUIRED"});return
     }
     items,err:=auth.ListProjectGrants(r.Context(),session.User.ID,strings.TrimSpace(r.PathValue("project_id")))
     if err!=nil{writeProjectGrantError(w,err);return}
     w.Header().Set("Cache-Control","no-store")
     writeJSON(w,http.StatusOK,map[string]any{
      "grants":items,
      "available_capabilities":userauth.AvailableProjectCapabilities(),
      "enforcement_active":false,
      "warning":"PROJECT_ACCESS_PREPARATION_ONLY_GLOBAL_ROLES_STILL_ACTIVE",
     })
    }))
  s.mux.HandleFunc("PUT /api/v1/projects/{project_id}/access/{user_id}",
   withPermission(auth,userauth.PermissionUserManage,
    func(w http.ResponseWriter,r *http.Request,session userauth.Session){
     if session.User.Role!=userauth.RoleOwner{
      writeJSON(w,http.StatusForbidden,map[string]any{"error_code":"OWNER_REQUIRED"});return
     }
     var body projectAccessRequest
     if !decodeBoundedJSON(w,r,&body){return}
     grant,err:=auth.ReplaceProjectGrant(r.Context(),session.User.ID,
      strings.TrimSpace(r.PathValue("project_id")),
      strings.TrimSpace(r.PathValue("user_id")),body.Capabilities)
     if err!=nil{writeProjectGrantError(w,err);return}
     w.Header().Set("Cache-Control","no-store")
     writeJSON(w,http.StatusOK,map[string]any{
      "grant":grant,"enforcement_active":false,
      "warning":"PROJECT_ACCESS_PREPARATION_ONLY_GLOBAL_ROLES_STILL_ACTIVE",
     })
    }))
  s.mux.HandleFunc("DELETE /api/v1/projects/{project_id}/access/{user_id}",
   withPermission(auth,userauth.PermissionUserManage,
    func(w http.ResponseWriter,r *http.Request,session userauth.Session){
     if session.User.Role!=userauth.RoleOwner{
      writeJSON(w,http.StatusForbidden,map[string]any{"error_code":"OWNER_REQUIRED"});return
     }
     if err:=auth.RevokeProjectGrant(r.Context(),session.User.ID,
      strings.TrimSpace(r.PathValue("project_id")),
      strings.TrimSpace(r.PathValue("user_id")));err!=nil{
      writeProjectGrantError(w,err);return
     }
     w.Header().Set("Cache-Control","no-store")
     writeJSON(w,http.StatusOK,map[string]any{
      "revoked":true,"enforcement_active":false,
      "warning":"PROJECT_ACCESS_PREPARATION_ONLY_GLOBAL_ROLES_STILL_ACTIVE",
     })
    }))
 }
}

func writeProjectGrantError(w http.ResponseWriter,err error){
 switch {
 case errors.Is(err,userauth.ErrForbidden):
  writeJSON(w,http.StatusForbidden,map[string]any{"error_code":"OWNER_REQUIRED"})
 case errors.Is(err,userauth.ErrProjectGrantInvalid):
  writeJSON(w,http.StatusBadRequest,map[string]any{"error_code":"PROJECT_GRANT_INVALID"})
 case errors.Is(err,userauth.ErrProjectGrantMissing):
  writeJSON(w,http.StatusNotFound,map[string]any{"error_code":"PROJECT_OR_USER_NOT_FOUND"})
 default:
  writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"PROJECT_GRANT_UNAVAILABLE"})
 }
}
