package httpapi

import (
 "bytes"
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/ali96adil/StageCore/internal/clock"
 "github.com/ali96adil/StageCore/internal/store"
 "github.com/ali96adil/StageCore/internal/userauth"
)

func TestOwnerOnlyProjectGrantAdministrationIsPreparationOnly(t *testing.T){
 h:=newAuthHarness(t)
 ctx:=context.Background()
 db:=store.New(h.db.DB,clock.Real{})
 p,_,err:=db.CreateProject(ctx,store.CreateProjectParams{Name:"ACL test",CreatedBy:"owner"})
 if err!=nil{t.Fatal(err)}
 operator,err:=h.auth.CreateUser(ctx,"acl-op","correct horse battery staple",userauth.RoleOperator)
 if err!=nil{t.Fatal(err)}
 owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
 if err!=nil{t.Fatal(err)}
 op,err:=h.auth.Login(ctx,"acl-op","correct horse battery staple","127.0.0.1")
 if err!=nil{t.Fatal(err)}
 handler:=New(WithOperatorProjectAccess(h.auth)).Handler()
 path:="/api/v1/projects/"+p.ID+"/access/"+operator.ID
 call:=func(method,uri,body,token,csrf string)*httptest.ResponseRecorder{
  t.Helper()
  req:=httptest.NewRequest(method,uri,bytes.NewReader([]byte(body)))
  req.RemoteAddr="127.0.0.1:2391"
  req.Header.Set("Content-Type","application/json")
  if token!=""{req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:token})}
  if csrf!=""{req.Header.Set(csrfHeader,csrf)}
  res:=httptest.NewRecorder()
  handler.ServeHTTP(res,req)
  return res
 }
 body:=`{"capabilities":["project.view","show.go"]}`
 if res:=call("PUT",path,body,"","");res.Code!=http.StatusUnauthorized{
  t.Fatalf("anonymous grant %d",res.Code)
 }
 if res:=call("PUT",path,body,op.Token,op.CSRFToken);res.Code!=http.StatusForbidden{
  t.Fatalf("operator grant %d",res.Code)
 }
 if res:=call("PUT",path,body,owner.Token,"");res.Code!=http.StatusForbidden{
  t.Fatalf("missing CSRF grant %d",res.Code)
 }
 if res:=call("PUT",path,`{"capabilities":["project.view","undefined.capability"]}`,owner.Token,owner.CSRFToken);res.Code!=http.StatusBadRequest{
  t.Fatalf("invalid capability status %d: %s",res.Code,res.Body.String())
 }
 res:=call("PUT",path,body,owner.Token,owner.CSRFToken)
 if res.Code!=http.StatusOK||!strings.Contains(res.Body.String(),`"enforcement_active":false`){
  t.Fatalf("grant must explicitly say NOT enforced yet: %d %s",res.Code,res.Body.String())
 }
 denied:=call("GET","/api/v1/projects/"+p.ID+"/access","",op.Token,"")
 if denied.Code!=http.StatusForbidden{t.Fatalf("nonowner leaked grant list status=%d",denied.Code)}
 listed:=call("GET","/api/v1/projects/"+p.ID+"/access","",owner.Token,"")
 if listed.Code!=http.StatusOK||!strings.Contains(listed.Body.String(),operator.ID){
  t.Fatalf("owner list %d %s",listed.Code,listed.Body.String())
 }
 if res:=call("DELETE",path,"",owner.Token,"");res.Code!=http.StatusForbidden{
  t.Fatalf("DELETE needs CSRF: %d",res.Code)
 }
 gone:=call("DELETE",path,"",owner.Token,owner.CSRFToken)
 if gone.Code!=http.StatusOK||!strings.Contains(gone.Body.String(),`"enforcement_active":false`){
  t.Fatalf("revoke status %d %s",gone.Code,gone.Body.String())
 }
 if err:=h.auth.CheckProjectAccess(ctx,operator.ID,p.ID,userauth.ProjectView);err!=userauth.ErrForbidden{
  t.Fatalf("revocation must deny membership evaluator: %v",err)
 }
}
