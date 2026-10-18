package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOwnerCutoverPreviewOptInPrivateAndNeverEnablesACL(t *testing.T) {
	f:=setupProjectAccessAPI(t)
	opt:=New(WithUserAuth(f.h.auth,f.h.hub),
		WithOwnerProjectCutoverPreview(f.h.auth,f.access)).Handler()
	defaultHub:=New(WithUserAuth(f.h.auth,f.h.hub)).Handler()
	url:="/api/v1/security/project-cutover/preview"
	get:=func(handler http.Handler, token, path, remote string) *httptest.ResponseRecorder{
		t.Helper()
		req:=httptest.NewRequest(http.MethodGet,path,nil)
		req.RemoteAddr=remote
		if token!="" {req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:token})}
		rec:=httptest.NewRecorder()
		handler.ServeHTTP(rec,req)
		return rec
	}
	if got:=get(defaultHub,f.owner.Token,url,"127.0.0.1:14200");got.Code!=http.StatusNotFound{
		t.Fatalf("Owner cutover preview unexpectedly active by default: %d",got.Code)
	}
	if got:=get(opt,"",url,"127.0.0.1:14200");got.Code!=http.StatusUnauthorized{
		t.Fatalf("anonymous preview HTTP %d",got.Code)
	}
	for _,nonOwner:=range []userauth.Credential{f.operator,f.viewer}{
		got:=get(opt,nonOwner.Token,url,"127.0.0.1:14200")
		if got.Code!=http.StatusForbidden||
			strings.Contains(got.Body.String(),"قرية صالح") ||
			strings.Contains(got.Body.String(),f.projectID) {
			t.Fatalf("nonowner viewed private impact: %d %s",got.Code,got.Body.String())
		}
	}
	if got:=get(opt,f.owner.Token,url,"192.168.1.55:14200");got.Code!=http.StatusUpgradeRequired{
		t.Fatalf("cleartext LAN impact preview=%d",got.Code)
	}
	got:=get(opt,f.owner.Token,url,"127.0.0.1:14200")
	if got.Code!=http.StatusOK||got.Header().Get("Cache-Control")!="no-store"{
		t.Fatalf("Owner preview unavailable or cached: %d %s",got.Code,got.Body.String())
	}
	var report projectaccess.CutoverImpactPreview
	if err:=json.Unmarshal(got.Body.Bytes(),&report);err!=nil{t.Fatal(err)}
	if report.Enforced||report.ReadyForCutover||!report.ReadOnly||
		report.ProjectCount!=2||report.UserCount<3{
		t.Fatalf("migration impact falsely declared active/ready: %+v",report)
	}
	var foundOp bool
	for _,u:=range report.Users{
		if u.UserID==f.operatorID{
			foundOp=true
			if u.LegacyVisibleProjects!=2||u.PreparedVisibleProjects!=0||
				len(u.WillLoseProjectView)!=2||u.LegacyGOPermittedProjects!=2||
				u.PreparedGOPermittedProjects!=0 {
				t.Fatalf("role access loss missing: %+v",u)
			}
		}
	}
	if !foundOp{t.Fatal("Operator impact missing")}
	// GET preview must not grant or revoke in either Project and cannot
	// claim a runtime authorization cutover has occurred.
	for _,p:=range []string{f.projectID,f.otherID}{
		if err:=f.access.Check(context.Background(),f.operatorID,p,projectaccess.ProjectView);err==nil{
			t.Fatalf("impact GET accidentally granted Project %s",p)
		}
	}
}
