package httpapi

import(
 "context"
 "errors"
 "net/http"
 "strings"

 "github.com/ali96adil/StageCore/internal/outputwatch"
 "github.com/ali96adil/StageCore/internal/projectaccess"
 "github.com/ali96adil/StageCore/internal/userauth"
)
type scopedCompanionRoleReader interface {
 ReadProject(context.Context,string)(outputwatch.CompanionProjectView,error)
}

// WithScopedCompanionRoleWatch is a separately optional Owner/Operator
// read-only view. It never enumerates all global Companions or permits
// a Project grant to execute MIDI/OSC, GO or Stage Device commands.
func WithScopedCompanionRoleWatch(auth *userauth.Service, grants *projectaccess.Service,
 reader scopedCompanionRoleReader) Option{
 return func(s *Server){
  if s==nil||s.mux==nil||auth==nil||grants==nil||reader==nil{return}
  s.mux.HandleFunc("GET /api/v1/projects/{project_id}/companion-output-watch",
   withAuthenticatedSession(auth,false,func(w http.ResponseWriter,r *http.Request,session userauth.Session,_ string){
    w.Header().Set("Cache-Control","no-store")
    projectID:=strings.TrimSpace(r.PathValue("project_id"))
    if projectID==""{
     writeJSON(w,http.StatusNotFound,map[string]any{"error_code":"COMPANION_WATCH_NOT_FOUND"});return
    }
    check:=func()error{return grants.Check(r.Context(),session.User.ID,projectID,projectaccess.ProjectView)}
    if err:=check();err!=nil{writeCompanionRoleWatchError(w,err);return}
    data,err:=reader.ReadProject(r.Context(),projectID)
    if errors.Is(err,outputwatch.ErrCompanionRolesOutOfScope){
     writeJSON(w,http.StatusNotFound,map[string]any{"error_code":"COMPANION_WATCH_NOT_FOUND"});return
    }
    if err!=nil{
     writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"COMPANION_WATCH_UNAVAILABLE"});return
    }
    if err:=check();err!=nil{writeCompanionRoleWatchError(w,err);return}
    if data.ProjectID!=projectID||!data.ReadOnly||data.PhysicalStatus!="UNKNOWN"||data.AutomatedReplay{
     writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"COMPANION_WATCH_SCOPE_INVALID"});return
    }
    // Hard-code non-claims at the external security/diagnostics boundary.
    // A future reader cannot accidentally introduce OSC/MIDI success.
    for i:=range data.Roles{
     data.Roles[i].MIDIExecutionConfirmed=false
     data.Roles[i].OSCDeliveryConfirmed=false
     data.Roles[i].PhysicalOutputVerified=false
    }
    writeJSON(w,http.StatusOK,data)
   }))
 }
}
func writeCompanionRoleWatchError(w http.ResponseWriter,err error){
 if errors.Is(err,projectaccess.ErrDenied)||errors.Is(err,projectaccess.ErrNotFound){
  writeJSON(w,http.StatusNotFound,map[string]any{"error_code":"COMPANION_WATCH_NOT_FOUND"})
 }else{
  writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"COMPANION_WATCH_UNAVAILABLE"})
 }
}
