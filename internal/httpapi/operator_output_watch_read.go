package httpapi

import (
 "context"
 "errors"
 "net/http"
 "strings"

 "github.com/ali96adil/StageCore/internal/outputwatch"
 "github.com/ali96adil/StageCore/internal/projectaccess"
 "github.com/ali96adil/StageCore/internal/userauth"
)

type outputWatchReader interface {
 Read(context.Context,string,string)(outputwatch.HubDeviceView,error)
}

// WithScopedOutputWatchRead is strictly opt-in. It reads existing authenticated
// device-socket and assignment metadata without issuing a probe or command.
// It is NOT wired to the Hub assembler; the normal Hub does not serve it.
func WithScopedOutputWatchRead(auth *userauth.Service, grants *projectaccess.Service,
 reader outputWatchReader) Option {
 return func(s *Server){
  if s==nil||s.mux==nil||auth==nil||grants==nil||reader==nil{return}
  s.mux.HandleFunc("GET /api/v1/projects/{project_id}/stage-devices/{device_id}/output-watch",
   withAuthenticatedSession(auth,false,func(w http.ResponseWriter,r *http.Request,session userauth.Session,_ string){
    w.Header().Set("Cache-Control","no-store")
    projectID:=strings.TrimSpace(r.PathValue("project_id"))
    deviceID:=strings.TrimSpace(r.PathValue("device_id"))
    if projectID==""||deviceID==""{
     writeJSON(w,http.StatusNotFound,map[string]any{"error_code":"OUTPUT_WATCH_NOT_FOUND"});return
    }
    allowed:=func()error{return grants.Check(r.Context(),session.User.ID,projectID,projectaccess.ProjectView)}
    if err:=allowed();err!=nil{
     writeWatchAccessError(w,err);return
    }
    view,err:=reader.Read(r.Context(),projectID,deviceID)
    if errors.Is(err,outputwatch.ErrDeviceOutOfScope){
     writeJSON(w,http.StatusNotFound,map[string]any{"error_code":"OUTPUT_WATCH_NOT_FOUND"});return
    }
    if err!=nil{
     writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"OUTPUT_WATCH_UNAVAILABLE"});return
    }
    // A last live-grant evaluation prevents reporting a foreign device
    // after a simple revoke. Full ACL cutover still needs atomic fencing.
    if err:=allowed();err!=nil{writeWatchAccessError(w,err);return}
    if view.ProjectID!=projectID||view.DeviceID!=deviceID{
     writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"OUTPUT_WATCH_SCOPE_INVALID"});return
    }
    view.PhysicalOutputVerified=false
    view.CommandsEnabled=false
    view.OperatorReviewRequired=true
    writeJSON(w,http.StatusOK,map[string]any{"read_only":true,"scope":"PROJECT_DEVICE",
     "view":view,"physical_status":"UNKNOWN","automated_replay":false})
   }))
 }
}
func writeWatchAccessError(w http.ResponseWriter,err error){
 if errors.Is(err,projectaccess.ErrDenied)||errors.Is(err,projectaccess.ErrNotFound){
  writeJSON(w,http.StatusNotFound,map[string]any{"error_code":"OUTPUT_WATCH_NOT_FOUND"})
 }else{
  writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"OUTPUT_WATCH_UNAVAILABLE"})
 }
}
