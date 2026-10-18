package httpapi

import (
 "context"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/ali96adil/StageCore/internal/outputwatch"
 "github.com/ali96adil/StageCore/internal/projectaccess"
)

type fakeOutputWatchRead struct{calls int; deny bool}
func(f *fakeOutputWatchRead)Read(_ context.Context,projectID,deviceID string)(outputwatch.HubDeviceView,error){
 f.calls++
 if f.deny{return outputwatch.HubDeviceView{},outputwatch.ErrDeviceOutOfScope}
 return outputwatch.HubDeviceView{SchemaVersion:1,DeviceID:deviceID,ProjectID:projectID,
  Condition:outputwatch.AwaitingFreshReport,CommandsEnabled:true,PhysicalOutputVerified:true},nil
}

func TestScopedOutputWatchOptionalBrowserRouteAndRevocation(t *testing.T){
 f:=setupProjectAccessAPI(t)
 reader:=&fakeOutputWatchRead{}
 noOpt:=New(WithUserAuth(f.h.auth,f.h.hub)).Handler()
 api:=New(WithUserAuth(f.h.auth,f.h.hub),
  WithScopedOutputWatchRead(f.h.auth,f.access,reader)).Handler()
 path:="/api/v1/projects/"+f.projectID+"/stage-devices/device-a/output-watch"
 get:=func(h http.Handler,token,route,remote string)*httptest.ResponseRecorder{
  t.Helper()
  req:=httptest.NewRequest(http.MethodGet,route,nil)
  req.RemoteAddr=remote
  if token!="" {req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:token})}
  w:=httptest.NewRecorder();h.ServeHTTP(w,req);return w
 }
 if v:=get(noOpt,f.owner.Token,path,"127.0.0.1:1222");v.Code!=http.StatusNotFound{
  t.Fatalf("Output Watch enabled by default: %d",v.Code)
 }
 if v:=get(api,"",path,"127.0.0.1:1222");v.Code!=http.StatusUnauthorized{
  t.Fatalf("anonymous metadata access: %d",v.Code)
 }
 if v:=get(api,f.operator.Token,path,"127.0.0.1:1222");v.Code!=http.StatusNotFound||reader.calls!=0{
  t.Fatalf("global OPERATOR role leaked observation: %d",v.Code)
 }
 if err:=f.access.SetPermissions(context.Background(),f.owner.Session.User.ID,
  f.operatorID,f.projectID,[]projectaccess.Permission{projectaccess.ProjectView});err!=nil{t.Fatal(err)}
 other:="/api/v1/projects/"+f.otherID+"/stage-devices/device-a/output-watch"
 if v:=get(api,f.operator.Token,other,"127.0.0.1:1222");v.Code!=http.StatusNotFound{
  t.Fatalf("cross-Project device metadata leaked: %d",v.Code)
 }
 if v:=get(api,f.operator.Token,path,"192.0.2.44:1222");v.Code!=http.StatusUpgradeRequired{
  t.Fatalf("insecure browser allowed: %d",v.Code)
 }
 v:=get(api,f.operator.Token,path,"127.0.0.1:1222")
 if v.Code!=http.StatusOK||!strings.Contains(v.Body.String(),`"read_only":true`)||
  !strings.Contains(v.Body.String(),`"physical_output_verified":false`)||
  !strings.Contains(v.Body.String(),`"commands_enabled":false`)||
  !strings.Contains(v.Body.String(),`"physical_status":"UNKNOWN"`)||
  v.Header().Get("Cache-Control")!="no-store"{
  t.Fatalf("nonactuating read not returned: %d %s",v.Code,v.Body.String())
 }
 reader.deny=true
 if v:=get(api,f.operator.Token,path,"127.0.0.1:1222");v.Code!=http.StatusNotFound{
  t.Fatalf("unassigned device disclosed: %d",v.Code)
 }
 reader.deny=false
 if err:=f.access.SetPermissions(context.Background(),f.owner.Session.User.ID,
  f.operatorID,f.projectID,nil);err!=nil{t.Fatal(err)}
 n:=reader.calls
 if v:=get(api,f.operator.Token,path,"127.0.0.1:1222");v.Code!=http.StatusNotFound||reader.calls!=n {
  t.Fatalf("grant revoke not immediate: %d",v.Code)
 }
}

// Ensure an upstream reader failure is not transformed into 'CONNECTED'.
func TestScopedOutputWatchCannotPublishWrongProjectResult(t *testing.T){
 f:=setupProjectAccessAPI(t)
 bad:=&wrongOutputWatchRead{}
 api:=New(WithUserAuth(f.h.auth,f.h.hub),WithScopedOutputWatchRead(f.h.auth,f.access,bad)).Handler()
 req:=httptest.NewRequest(http.MethodGet,"/api/v1/projects/"+f.projectID+
  "/stage-devices/a/output-watch",nil)
 req.RemoteAddr="127.0.0.1:1222"
 req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:f.owner.Token})
 w:=httptest.NewRecorder();api.ServeHTTP(w,req)
 if w.Code!=http.StatusServiceUnavailable||strings.Contains(w.Body.String(),"other-project"){
  t.Fatalf("foreign reader result exposed: %d %s",w.Code,w.Body.String())
 }
}
type wrongOutputWatchRead struct{}
func(*wrongOutputWatchRead)Read(context.Context,string,string)(outputwatch.HubDeviceView,error){
 return outputwatch.HubDeviceView{ProjectID:"other-project",DeviceID:"a"},nil
}
var _=errors.Is
