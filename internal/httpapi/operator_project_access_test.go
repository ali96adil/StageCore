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
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/secretstore"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type projectAccessAPIFixture struct {
	h *authHarness
	access *projectaccess.Service
	audit *securityaudit.Service
	projects *store.Store
	handler http.Handler
	owner userauth.Credential
	operator userauth.Credential
	viewer userauth.Credential
	operatorID string
	viewerID string
	projectID string
	otherID string
}

func setupProjectAccessAPI(t *testing.T) projectAccessAPIFixture {
	t.Helper()
	ctx:=context.Background()
	h:=newAuthHarness(t)
	stageStore:=store.New(h.db.DB, clock.Real{})
	first,_,err:=stageStore.CreateProject(ctx,store.CreateProjectParams{Name:"قرية صالح"})
	if err!=nil{t.Fatal(err)}
	second,_,err:=stageStore.CreateProject(ctx,store.CreateProjectParams{Name:"Another show"})
	if err!=nil{t.Fatal(err)}
	owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	operator,err:=h.auth.CreateUser(ctx,"operator-acl-api","correct horse battery staple",userauth.RoleOperator)
	if err!=nil{t.Fatal(err)}
	viewer,err:=h.auth.CreateUser(ctx,"viewer-acl-api","correct horse battery staple",userauth.RoleViewer)
	if err!=nil{t.Fatal(err)}
	opCred,err:=h.auth.Login(ctx,operator.Username,"correct horse battery staple","127.0.0.2")
	if err!=nil{t.Fatal(err)}
	viewCred,err:=h.auth.Login(ctx,viewer.Username,"correct horse battery staple","127.0.0.3")
	if err!=nil{t.Fatal(err)}
	access,err:=projectaccess.New(h.db.DB)
	if err!=nil{t.Fatal(err)}
	secrets,err:=secretstore.Open(ctx,h.db.DB,t.TempDir())
	if err!=nil{t.Fatal(err)}
	audit,err:=securityaudit.New(h.db.DB,secrets)
	if err!=nil{t.Fatal(err)}
	handler:=New(WithOperatorProjectAccess(h.auth,access,stageStore,audit)).Handler()
	return projectAccessAPIFixture{h:h,access:access,audit:audit,projects:stageStore,handler:handler,
		owner:owner,operator:opCred,viewer:viewCred,operatorID:operator.ID,viewerID:viewer.ID,
		projectID:first.ID,otherID:second.ID}
}

func callProjectAccess(t *testing.T,f projectAccessAPIFixture,as *userauth.Credential,
	method,projectID,userID,body string,includeCSRF bool) *httptest.ResponseRecorder {
	t.Helper()
	path:="/api/v1/projects/"+projectID+"/access/users/"+userID
	req:=httptest.NewRequest(method,path,bytes.NewReader([]byte(body)))
	req.RemoteAddr="127.0.0.1:17701"
	req.Header.Set("Content-Type","application/json")
	if as!=nil{
		req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:as.Token})
		if includeCSRF{req.Header.Set(csrfHeader,as.CSRFToken)}
	}
	res:=httptest.NewRecorder()
	f.handler.ServeHTTP(res,req)
	return res
}

func TestOwnerProjectAccessAPIPreparationAndImmediateGrantReadback(t *testing.T){
	f:=setupProjectAccessAPI(t)
	ctx:=context.Background()
	urlProject,userID:=f.projectID,f.operatorID
	noLogin:=callProjectAccess(t,f,nil,http.MethodGet,urlProject,userID,"",false)
	if noLogin.Code!=http.StatusUnauthorized{t.Fatalf("anonymous GET=%d",noLogin.Code)}
	for _,nonOwner:=range []*userauth.Credential{&f.operator,&f.viewer}{
		for _,method:=range []string{http.MethodGet,http.MethodPut}{
			r:=callProjectAccess(t,f,nonOwner,method,urlProject,userID,
				`{"permissions":["project.view","show.go"]}`,true)
			if r.Code!=http.StatusForbidden{t.Fatalf("%s nonowner response=%d body=%s",method,r.Code,r.Body.String())}
		}
	}
	missingCSRF:=callProjectAccess(t,f,&f.owner,http.MethodPut,urlProject,userID,
		`{"permissions":["project.view"]}`,false)
	if missingCSRF.Code!=http.StatusForbidden{t.Fatalf("CSRF missing response=%d",missingCSRF.Code)}
	for _,input:=range []string{
		`{}`,`{"permissions":null}`,`{"permissions":"revoke"}`,
		`{"permissions":["show.go"]}`,`{"permissions":["project.view","root.manage"]}`,
		`{"permissions":["project.view","project.view"]}`,
	}{
		result:=callProjectAccess(t,f,&f.owner,http.MethodPut,urlProject,userID,input,true)
		if result.Code!=http.StatusBadRequest{
			t.Fatalf("invalid grant %s response=%d %s",input,result.Code,result.Body.String())
		}
	}
	rights:=`{"permissions":["project.view","show.go","rehearsal.control"]}`
	put:=callProjectAccess(t,f,&f.owner,http.MethodPut,urlProject,userID,rights,true)
	if put.Code!=http.StatusOK{t.Fatalf("Owner PUT=%d body=%s",put.Code,put.Body.String())}
	var response struct{
		Enforced bool `json:"enforced"`
		Phase string `json:"phase"`
		Permissions []projectaccess.Permission `json:"permissions"`
	}
	if err:=json.Unmarshal(put.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
	if response.Enforced || response.Phase!="PREPARED_NOT_ENFORCED" || len(response.Permissions)!=3{
		t.Fatalf("must clearly disclose not enforced and preserve grants: %+v",response)
	}
	other:=callProjectAccess(t,f,&f.owner,http.MethodGet,f.otherID,userID,"",false)
	if other.Code!=http.StatusOK || strings.Contains(other.Body.String(),string(projectaccess.ShowGO)){
		t.Fatalf("other Project exposed grant: %d %s",other.Code,other.Body.String())
	}
	got:=callProjectAccess(t,f,&f.owner,http.MethodGet,urlProject,userID,"",false)
	if got.Code!=http.StatusOK || !strings.Contains(got.Body.String(),string(projectaccess.ShowGO)){
		t.Fatalf("Owner grants GET=%d body=%s",got.Code,got.Body.String())
	}
	if err:=f.access.Check(ctx,userID,urlProject,projectaccess.ShowGO);err!=nil{t.Fatal(err)}
	if err:=f.access.Check(ctx,userID,f.otherID,projectaccess.ShowGO);err==nil{
		t.Fatal("Project A grants leaked to Project B")
	}
	revoked:=callProjectAccess(t,f,&f.owner,http.MethodPut,urlProject,userID,`{"permissions":[]}`,true)
	if revoked.Code!=http.StatusOK{t.Fatalf("revoke=%d %s",revoked.Code,revoked.Body.String())}
	if err:=f.access.Check(ctx,userID,urlProject,projectaccess.ShowGO);err==nil{
		t.Fatal("revoke not reflected in live Project grant policy")
	}
	records,err:=f.audit.List(ctx,40)
	if err!=nil{t.Fatal(err)}
	changes:=0
	for _,a:=range records{
		if a.EventType=="project.permissions.replace"{
			changes++
			if a.ActorUserID!=f.owner.Session.User.ID || a.ResourceID!=urlProject{
				t.Fatalf("audit scope missing: %+v",a)
			}
		}
	}
	if changes!=2{t.Fatalf("expected 2 atomic change audits, got %d",changes)}
}

func TestProjectAccessAPIRejectsViewerActuationAndAuditFailureRollsBack(t *testing.T){
	f:=setupProjectAccessAPI(t)
	ctx:=context.Background()
	target:=f.viewerID
	view:=callProjectAccess(t,f,&f.owner,http.MethodPut,f.projectID,target,
		`{"permissions":["project.view","media.read"]}`,true)
	if view.Code!=http.StatusOK{t.Fatalf("read-only Viewer grant=%d: %s",view.Code,view.Body.String())}
	actuation:=callProjectAccess(t,f,&f.owner,http.MethodPut,f.projectID,target,
		`{"permissions":["project.view","show.go"]}`,true)
	if actuation.Code!=http.StatusForbidden{t.Fatalf("Viewer got GO privilege: %d %s",actuation.Code,actuation.Body.String())}
	if err:=f.access.Check(ctx,target,f.projectID,projectaccess.ShowGO);err==nil{
		t.Fatal("viewer may control SHOW")
	}

	// Abort only security audit writes. The replacement must not be committed
	// because its audit event cannot be persisted in the same SQL transaction.
	if _,err:=f.h.db.DB.ExecContext(ctx,`CREATE TRIGGER reject_grant_audit
		BEFORE INSERT ON security_audit_records
		WHEN NEW.event_type='project.permissions.replace'
		BEGIN SELECT RAISE(ABORT,'test audit unavailable'); END`);err!=nil{t.Fatal(err)}
	res:=callProjectAccess(t,f,&f.owner,http.MethodPut,f.projectID,target,
		`{"permissions":["project.view"]}`,true)
	if res.Code!=http.StatusServiceUnavailable {
		t.Fatalf("failed audit must reject grant mutation: %d %s",res.Code,res.Body.String())
	}
	// Original Viewer media.read grant must survive failed replacement.
	if err:=f.access.Check(ctx,target,f.projectID,projectaccess.MediaRead);err!=nil{
		t.Fatalf("audit failure changed permissions: %v",err)
	}
}

func TestProjectAccessAPIRejectsMutationDuringShowAndUnknownTargets(t *testing.T){
	f:=setupProjectAccessAPI(t)
	ctx:=context.Background()
	notFound:=callProjectAccess(t,f,&f.owner,http.MethodGet,"unknown-project",f.operatorID,"",false)
	if notFound.Code!=http.StatusNotFound{t.Fatalf("GET unknown Project=%d",notFound.Code)}
	notFound=callProjectAccess(t,f,&f.owner,http.MethodGet,f.projectID,"unknown-user","",false)
	if notFound.Code!=http.StatusNotFound{t.Fatalf("GET unknown user=%d",notFound.Code)}
	startShowSession(t,ctx,f.projects)
	blocked:=callProjectAccess(t,f,&f.owner,http.MethodPut,f.projectID,f.operatorID,
		`{"permissions":["project.view","show.go"]}`,true)
	if blocked.Code!=http.StatusLocked{t.Fatalf("active SHOW grant mutation=%d body=%s",blocked.Code,blocked.Body.String())}
	if err:=f.access.Check(ctx,f.operatorID,f.projectID,projectaccess.ShowGO);err==nil{
		t.Fatal("grant was changed during SHOW")
	}
}
