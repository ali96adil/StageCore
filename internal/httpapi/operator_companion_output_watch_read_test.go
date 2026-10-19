package httpapi

import(
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/ali96adil/StageCore/internal/outputwatch"
 "github.com/ali96adil/StageCore/internal/projectaccess"
)
type fakeCompanionRoleWatch struct {calls int;foreign bool}
func(f *fakeCompanionRoleWatch)ReadProject(_ context.Context,projectID string)(outputwatch.CompanionProjectView,error){
 f.calls++
 if f.foreign{projectID="foreign-project"}
 return outputwatch.CompanionProjectView{
  SchemaVersion:1,ProjectID:projectID,ReadOnly:true,PhysicalStatus:"UNKNOWN",
  Roles:[]outputwatch.CompanionRoleView{{MachineRoleID:"r",RoleKey:"AUDIO-ABLETON",
   Condition:"COMPANION_READY",MIDIExecutionConfirmed:true,OSCDeliveryConfirmed:true,PhysicalOutputVerified:true}},
 },nil
}
func TestScopedCompanionWatchOptInSecurityAndNoFalseExecutionProof(t *testing.T){
 f:=setupProjectAccessAPI(t)
 reader:=&fakeCompanionRoleWatch{}
 baseline:=New(WithUserAuth(f.h.auth,f.h.hub)).Handler()
 api:=New(WithUserAuth(f.h.auth,f.h.hub),
  WithScopedCompanionRoleWatch(f.h.auth,f.access,reader)).Handler()
 path:="/api/v1/projects/"+f.projectID+"/companion-output-watch"
 get:=func(h http.Handler,token,path,remote string)*httptest.ResponseRecorder{
  t.Helper()
  req:=httptest.NewRequest(http.MethodGet,path,nil)
  req.RemoteAddr=remote
  if token!=""{req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:token})}
  w:=httptest.NewRecorder();h.ServeHTTP(w,req);return w
 }
 if v:=get(baseline,f.owner.Token,path,"127.0.0.1:10101");v.Code!=404{
  t.Fatalf("unwired Companion endpoint available in default Hub: %d",v.Code)
 }
 if v:=get(api,"",path,"127.0.0.1:10101");v.Code!=401{
  t.Fatalf("anonymous Companion metadata: %d",v.Code)
 }
 if v:=get(api,f.operator.Token,path,"127.0.0.1:10101");v.Code!=404||reader.calls!=0{
  t.Fatalf("global OPERATOR leaked Companion role metadata: %d",v.Code)
 }
 if err:=f.access.SetPermissions(context.Background(),f.owner.Session.User.ID,
  f.operatorID,f.projectID,[]projectaccess.Permission{projectaccess.ProjectView});err!=nil{t.Fatal(err)}
 if v:=get(api,f.operator.Token,
  "/api/v1/projects/"+f.otherID+"/companion-output-watch","127.0.0.1:10101");v.Code!=404{
  t.Fatalf("foreign Project role metadata leaked: %d",v.Code)
 }
 if v:=get(api,f.operator.Token,path,"192.0.2.21:10101");v.Code!=426{
  t.Fatalf("insecure network metadata exposed: %d",v.Code)
 }
 ok:=get(api,f.operator.Token,path,"127.0.0.1:10101")
 if ok.Code!=200||ok.Header().Get("Cache-Control")!="no-store"||
  !strings.Contains(ok.Body.String(),`"read_only":true`)||
  !strings.Contains(ok.Body.String(),`"midi_execution_confirmed":false`)||
  !strings.Contains(ok.Body.String(),`"osc_delivery_confirmed":false`)||
  !strings.Contains(ok.Body.String(),`"physical_output_verified":false`){
  t.Fatalf("Companion readiness leaked execution certainty: %d %s",ok.Code,ok.Body.String())
 }
 reader.foreign=true
 if v:=get(api,f.operator.Token,path,"127.0.0.1:10101");v.Code!=503||
  strings.Contains(v.Body.String(),"foreign-project"){
  t.Fatalf("reader returned foreign Project role data: %d %s",v.Code,v.Body.String())
 }
 reader.foreign=false
 if err:=f.access.SetPermissions(context.Background(),f.owner.Session.User.ID,
  f.operatorID,f.projectID,nil);err!=nil{t.Fatal(err)}
 count:=reader.calls
 if v:=get(api,f.operator.Token,path,"127.0.0.1:10101");v.Code!=404||reader.calls!=count{
  t.Fatalf("Project grant revocation ineffective: %d",v.Code)
 }
}
