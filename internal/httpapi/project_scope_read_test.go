package httpapi

import (
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

// These probe handlers exist ONLY in tests. Production registration is
// deliberately postponed until ALL sibling Cue/runtime/Vault endpoints have
// the same server-side Project ACL and user migration is approved.
func TestScopedProjectReadCannotEnumerateOrOpenOtherShows(t *testing.T) {
	h:=newAuthHarness(t)
	ctx:=context.Background()
	projectStore:=store.New(h.db.DB,clock.Real{})
	showA,_,err:=projectStore.CreateProject(ctx,store.CreateProjectParams{Name:"قرية صالح"})
	if err!=nil{t.Fatal(err)}
	showB,_,err:=projectStore.CreateProject(ctx,store.CreateProjectParams{Name:"Secret unpublished show"})
	if err!=nil{t.Fatal(err)}
	operator,err:=h.auth.CreateUser(ctx,"scoped-reader","correct horse battery staple",userauth.RoleOperator)
	if err!=nil{t.Fatal(err)}
	viewer,err:=h.auth.CreateUser(ctx,"scoped-viewer","correct horse battery staple",userauth.RoleViewer)
	if err!=nil{t.Fatal(err)}
	ownerCred,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	opCred,err:=h.auth.Login(ctx,operator.Username,"correct horse battery staple","127.0.0.1")
	if err!=nil{t.Fatal(err)}
	viewerCred,err:=h.auth.Login(ctx,viewer.Username,"correct horse battery staple","127.0.0.1")
	if err!=nil{t.Fatal(err)}
	access,err:=projectaccess.New(h.db.DB)
	if err!=nil{t.Fatal(err)}
	if err:=access.SetPermissions(ctx,ownerCred.Session.User.ID,operator.ID,showA.ID,
		[]projectaccess.Permission{projectaccess.ProjectView});err!=nil{t.Fatal(err)}

	mux:=http.NewServeMux()
	mux.HandleFunc("GET /probe/projects",withAuthenticatedSession(h.auth,false,
		func(w http.ResponseWriter,r *http.Request,session userauth.Session,_ string) {
			projects,err:=scopedProjectList(r.Context(),access,projectStore,session.User.ID)
			if err!=nil{
				writeJSON(w,http.StatusServiceUnavailable,map[string]any{"error_code":"PROJECT_SCOPE_UNAVAILABLE"})
				return
			}
			views:=make([]projectView,0,len(projects))
			for _,p:=range projects {views=append(views,makeProjectView(p))}
			writeJSON(w,http.StatusOK,map[string]any{"projects":views})
		}))
	mux.HandleFunc("GET /probe/projects/{project_id}",withScopedProjectRead(h.auth,access,
		func(w http.ResponseWriter,r *http.Request,_ userauth.Session) {
			project,revision,ok:=loadProjectAndRevision(w,r,projectStore)
			if !ok {return}
			writeJSON(w,http.StatusOK,map[string]any{
				"project":makeProjectView(project),"draft_revision":makeRevisionView(revision),
			})
		}))

	get:=func(token,path string) *httptest.ResponseRecorder {
		t.Helper()
		req:=httptest.NewRequest(http.MethodGet,path,nil)
		req.RemoteAddr="127.0.0.1:11001"
		if token!=""{req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:token})}
		response:=httptest.NewRecorder()
		mux.ServeHTTP(response,req)
		return response
	}
	expectCount:=func(token string,want int,hidden string){
		t.Helper()
		res:=get(token,"/probe/projects")
		if res.Code!=http.StatusOK{t.Fatalf("scoped list=%d: %s",res.Code,res.Body.String())}
		if hidden!="" && strings.Contains(res.Body.String(),hidden){
			t.Fatalf("restricted project name leaked: %s",res.Body.String())
		}
		var body struct{ Projects []projectView `json:"projects"` }
		if err:=json.Unmarshal(res.Body.Bytes(),&body);err!=nil{t.Fatal(err)}
		if len(body.Projects)!=want{t.Fatalf("list expected %d, got %+v",want,body.Projects)}
	}

	expectCount(opCred.Token,1,showB.Name)
	expectCount(viewerCred.Token,0,showA.Name)
	expectCount(ownerCred.Token,2,"")
	if res:=get(opCred.Token,"/probe/projects/"+showA.ID);res.Code!=http.StatusOK {
		t.Fatalf("assigned Project invisible: %d %s",res.Code,res.Body.String())
	}
	for _,id:=range []string{showB.ID,"unknown-project"}{
		res:=get(opCred.Token,"/probe/projects/"+id)
		if res.Code!=http.StatusNotFound ||
			!strings.Contains(res.Body.String(),`"error_code":"PROJECT_NOT_FOUND"`) ||
			strings.Contains(res.Body.String(),showB.Name){
			t.Fatalf("unauthorized project disclosure %s: %d %s",id,res.Code,res.Body.String())
		}
	}
	if res:=get("","/probe/projects/"+showA.ID);res.Code!=http.StatusUnauthorized {
		t.Fatalf("anonymous may open show: %d",res.Code)
	}
	// The existing open session must immediately reflect Owner revocation.
	if err:=access.SetPermissions(ctx,ownerCred.Session.User.ID,operator.ID,showA.ID,nil);err!=nil{t.Fatal(err)}
	expectCount(opCred.Token,0,showA.Name)
	if res:=get(opCred.Token,"/probe/projects/"+showA.ID);res.Code!=http.StatusNotFound {
		t.Fatalf("revoked Project open returned %d",res.Code)
	}
	if err:=access.SetPermissions(ctx,ownerCred.Session.User.ID,operator.ID,showA.ID,
		[]projectaccess.Permission{projectaccess.ProjectView});err!=nil{t.Fatal(err)}
	// A disabled account cannot use a formerly valid browser session.
	if _,err:=h.auth.SetUserEnabled(ctx,operator.ID,false);err!=nil{t.Fatal(err)}
	if res:=get(opCred.Token,"/probe/projects");res.Code!=http.StatusUnauthorized{
		t.Fatalf("disabled browser token still authorized: %d",res.Code)
	}
}

func TestScopedProjectListRejectsUnconfiguredSecurityGate(t *testing.T) {
	h:=newAuthHarness(t)
	s:=store.New(h.db.DB,clock.Real{})
	_,err:=scopedProjectList(context.Background(),nil,s,"operator")
	if err==nil{t.Fatal("nil ACL silently returned all Projects")}
	var nilStore *store.Store
	a,err:=projectaccess.New(h.db.DB)
	if err!=nil{t.Fatal(err)}
	_,err=scopedProjectList(context.Background(),a,nilStore,"operator")
	if err==nil{t.Fatal("nil Project Store silently returned all Projects")}
}

