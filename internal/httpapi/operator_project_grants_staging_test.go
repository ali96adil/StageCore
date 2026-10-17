package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOwnerCanStageProjectGrantsWithoutEnforcingExistingProjectAccess(t *testing.T){
	h:=newAuthHarness(t)
	ctx:=context.Background()
	st:=store.New(h.db.DB,clock.Real{})
	first,_,err:=st.CreateProject(ctx,store.CreateProjectParams{Name:"Show Alpha",CreatedBy:"owner"})
	if err!=nil{t.Fatal(err)}
	second,_,err:=st.CreateProject(ctx,store.CreateProjectParams{Name:"Show Beta",CreatedBy:"owner"})
	if err!=nil{t.Fatal(err)}
	account,err:=h.auth.CreateUser(ctx,"operator-staging","correct horse battery staple",userauth.RoleOperator)
	if err!=nil{t.Fatal(err)}
	handler:=New(
		WithOperatorProjectGrantsStaging(h.auth,projectaccess.New(h.db.DB)),
		WithOperatorProjects(h.auth,st),
	).Handler()
	owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	operator,err:=h.auth.Login(ctx,account.Username,"correct horse battery staple","127.0.0.2")
	if err!=nil{t.Fatal(err)}
	policyURL:="/api/v1/security/projects/"+first.ID+"/grants/staging"
	updateURL:=policyURL+"/"+account.ID
	req:=func(method,url,body string,cred userauth.Credential,csrf bool)*httptest.ResponseRecorder {
		t.Helper()
		r:=httptest.NewRequest(method,url,strings.NewReader(body))
		r.RemoteAddr="127.0.0.1:14777"
		r.Header.Set("Content-Type","application/json")
		if cred.Token!="" {r.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:cred.Token})}
		if csrf{r.Header.Set(csrfHeader,cred.CSRFToken)}
		out:=httptest.NewRecorder()
		handler.ServeHTTP(out,r)
		return out
	}
	for _,c:=range []struct{
		method,url,body string
		cred userauth.Credential
		csrf bool
		status int
	}{
		{"GET",policyURL,"",userauth.Credential{},false,http.StatusUnauthorized},
		{"GET",policyURL,"",operator,false,http.StatusForbidden},
		{"PUT",updateURL,`{"capabilities":["project.view","media.read"]}`,owner,false,http.StatusForbidden},
		{"PUT",updateURL,`{"capabilities":["project.view","secret.manage"]}`,owner,true,http.StatusBadRequest},
		{"PUT",updateURL,`{"capabilities":["show.go"]}`,owner,true,http.StatusBadRequest},
		{"PUT",updateURL,`{"capabilities":["project.view","project.view"]}`,owner,true,http.StatusBadRequest},
	}{
		o:=req(c.method,c.url,c.body,c.cred,c.csrf)
		if o.Code!=c.status{t.Fatalf("guard %s %s got %d, want %d: %s",c.method,c.url,o.Code,c.status,o.Body.String())}
	}
	grantBody:=`{"capabilities":["project.view","media.read","show.go"]}`
	applied:=req("PUT",updateURL,grantBody,owner,true)
	if applied.Code!=http.StatusOK{
		t.Fatalf("valid Owner stage code=%d body=%s",applied.Code,applied.Body.String())
	}
	var result struct{
		Enforced bool `json:"enforced"`
		Phase string `json:"phase"`
		Grant projectaccess.Grant `json:"grant"`
	}
	if err:=json.Unmarshal(applied.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
	if result.Enforced||result.Phase!="STAGING_ONLY"||len(result.Grant.Capabilities)!=3{
		t.Fatalf("staged grant misrepresented: %+v",result)
	}
	list:=req("GET",policyURL,"",owner,false)
	if list.Code!=http.StatusOK||!bytes.Contains(list.Body.Bytes(),[]byte(`"enforced":false`))||
		!bytes.Contains(list.Body.Bytes(),[]byte(account.Username)){
		t.Fatalf("Owner staged list failed: %d %s",list.Code,list.Body.String())
	}

	// Phase 1 must not accidentally narrow today's global role behavior.
	for _,p:=range []string{first.ID,second.ID}{
		o:=req("GET","/api/v1/projects/"+p,"",operator,false)
		if o.Code!=http.StatusOK{
			t.Fatalf("staging broke existing Operator access to %s: %d %s",p,o.Code,o.Body.String())
		}
	}
	all:=req("GET","/api/v1/projects","",operator,false)
	if all.Code!=http.StatusOK ||
		!bytes.Contains(all.Body.Bytes(),[]byte("Show Alpha")) ||
		!bytes.Contains(all.Body.Bytes(),[]byte("Show Beta")){
		t.Fatalf("staging unexpectedly changed global project listings: %d %s",all.Code,all.Body.String())
	}
	var changes int
	if err:=h.db.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_access_grant_changes WHERE project_id=?`,first.ID).Scan(&changes);err!=nil{t.Fatal(err)}
	if changes!=1{t.Fatalf("owner update missing atomic audit: %d",changes)}
	revoke:=req("PUT",updateURL,`{"capabilities":[]}`,owner,true)
	if revoke.Code!=http.StatusOK {t.Fatalf("staged revoke=%d %s",revoke.Code,revoke.Body.String())}
	if err:=h.db.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_access_grant_changes WHERE project_id=?`,first.ID).Scan(&changes);err!=nil{t.Fatal(err)}
	if changes!=2{t.Fatalf("revoke missing audit event: %d",changes)}
}

func TestStagingCannotBeManagedByTechnicianOrOperator(t *testing.T) {
	h:=newAuthHarness(t)
	ctx:=context.Background()
	st:=store.New(h.db.DB,clock.Real{})
	p,_,err:=st.CreateProject(ctx,store.CreateProjectParams{Name:"Guarded Show"})
	if err!=nil{t.Fatal(err)}
	tech,err:=h.auth.CreateUser(ctx,"tech-stage-grants","correct horse battery staple",userauth.RoleTechnician)
	if err!=nil{t.Fatal(err)}
	target,err:=h.auth.CreateUser(ctx,"operator-stage-grants","correct horse battery staple",userauth.RoleOperator)
	if err!=nil{t.Fatal(err)}
	tc,err:=h.auth.Login(ctx,tech.Username,"correct horse battery staple","127.0.0.3")
	if err!=nil{t.Fatal(err)}
	base:="/api/v1/security/projects/"+p.ID+"/grants/staging"
	handler:=New(WithOperatorProjectGrantsStaging(h.auth,projectaccess.New(h.db.DB))).Handler()
	for _,test:=range []struct{method,url,body string}{
		{"GET",base,""},
		{"PUT",base+"/"+target.ID,`{"capabilities":["project.view","show.go"]}`},
	}{
		req:=httptest.NewRequest(test.method,test.url,bytes.NewBufferString(test.body))
		req.RemoteAddr="127.0.0.1:14778"
		req.Header.Set(csrfHeader,tc.CSRFToken)
		req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:tc.Token})
		rec:=httptest.NewRecorder()
		handler.ServeHTTP(rec,req)
		if rec.Code!=http.StatusForbidden{t.Fatalf("technician modified access: %d %s",rec.Code,rec.Body.String())}
	}
	var total int
	if err:=h.db.DB.QueryRowContext(ctx,`SELECT COUNT(*) FROM project_access_grants`).Scan(&total);err!=nil{t.Fatal(err)}
	if total!=0{t.Fatalf("technician staged %d forbidden grants",total)}
}
