package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/showcapsule"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestScopedCapsuleExportListEnforcesVerifiedProjectSnapshotAndGrant(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	stageVault, err := vault.Open(t.TempDir(), st)
	if err != nil { t.Fatal(err) }
	capsuleService, err := showcapsule.New(st, stageVault, clock.Real{})
	if err != nil { t.Fatal(err) }
	root := t.TempDir()
	exportsRoot := filepath.Join(root, "exports")
	var showIDs []string
	var capsules []showcapsule.ExportResult
	for _, name := range []string{"قرية صالح A", "Private Show B"} {
		show, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: name, CreatedBy:"owner"})
		if err != nil { t.Fatal(err) }
		if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil { t.Fatal(err) }
		if _, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, "owner"); err != nil { t.Fatal(err) }
		capsule, err := capsuleService.Export(ctx, show.ID, exportsRoot, showcapsule.BuildOptions{
			Mode: showcapsule.ExportManifestOnly,
		})
		if err != nil { t.Fatal(err) }
		showIDs = append(showIDs, show.ID)
		capsules = append(capsules, capsule)
	}

	operator, err := h.auth.CreateUser(ctx, "capsule-scoped-operator", h.password, userauth.RoleOperator)
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	op, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	access, err := projectaccess.New(h.db.DB)
	if err != nil { t.Fatal(err) }

	pathA := "/api/v1/projects/"+showIDs[0]+"/show-capsules/exports"
	pathB := "/api/v1/projects/"+showIDs[1]+"/show-capsules/exports"
	old := New(WithUserAuth(h.auth,h.hub))
	api := New(WithUserAuth(h.auth,h.hub),WithScopedLocalCapsuleExports(h.auth,access,root))
	get := func(handler http.Handler, token, path string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet,path,nil)
		r.RemoteAddr = "127.0.0.1:18110"
		if token != "" { r.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:token}) }
		w:=httptest.NewRecorder()
		handler.ServeHTTP(w,r)
		return w
	}
	assertDenial := func(token,path string) {
		t.Helper()
		got:=get(api.Handler(),token,path)
		if got.Code!=http.StatusNotFound ||
			strings.Contains(got.Body.String(), "Private Show B") ||
			strings.Contains(got.Body.String(), capsules[1].CapsuleID) {
			t.Fatalf("denied Capsule list leaked: %d %s",got.Code,got.Body.String())
		}
	}
	if got:=get(old.Handler(),owner.Token,pathA);got.Code!=http.StatusNotFound {
		t.Fatalf("opt-in Capsule route enabled by default: %d",got.Code)
	}
	if got:=get(api.Handler(),"",pathA);got.Code!=http.StatusUnauthorized {
		t.Fatalf("anonymous Capsule list status=%d",got.Code)
	}
	assertDenial(op.Token,pathA)
	if err:=access.SetPermissions(ctx,owner.Session.User.ID,operator.ID,showIDs[0],
		[]projectaccess.Permission{projectaccess.ProjectView});err!=nil {t.Fatal(err)}
	assertDenial(op.Token,pathA)

	if err:=access.SetPermissions(ctx,owner.Session.User.ID,operator.ID,showIDs[0],
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.CapsuleExport});err!=nil {t.Fatal(err)}
	for _, path:=range []string{pathB,"/api/v1/projects/does-not-exist/show-capsules/exports"} {
		assertDenial(op.Token,path)
	}
	got:=get(api.Handler(),op.Token,pathA)
	if got.Code!=http.StatusOK || !strings.Contains(got.Body.String(), capsules[0].CapsuleID) ||
		!strings.Contains(got.Body.String(), `"scoped":true`) ||
		!strings.Contains(got.Body.String(), `"read_only":true`) ||
		strings.Contains(got.Body.String(),capsules[1].CapsuleID) ||
		strings.Contains(got.Body.String(),"Private Show B") ||
		!strings.Contains(got.Header().Get("Cache-Control"),"no-store") {
		t.Fatalf("Project A export filter failed: %d %s",got.Code,got.Body.String())
	}
	if got:=get(api.Handler(),owner.Token,pathB);got.Code!=http.StatusOK ||
		!strings.Contains(got.Body.String(),capsules[1].CapsuleID) ||
		strings.Contains(got.Body.String(),capsules[0].CapsuleID) {
		t.Fatalf("Owner scoped B export list: %d %s",got.Code,got.Body.String())
	}
	// Symlinks are never treated as Capsule directories, even with canonical IDs.
	symlinkID:="55555555-5555-4555-8555-555555555555"
	if err:=os.Symlink(capsules[1].Path,filepath.Join(exportsRoot,symlinkID));err!=nil{t.Fatal(err)}
	if got:=get(api.Handler(),op.Token,pathA);got.Code!=http.StatusOK ||
		strings.Contains(got.Body.String(),symlinkID) {
		t.Fatalf("symlink leaked into scoped exports: %d %s",got.Code,got.Body.String())
	}
	if err:=access.SetPermissions(ctx,owner.Session.User.ID,operator.ID,showIDs[0],nil);err!=nil {t.Fatal(err)}
	assertDenial(op.Token,pathA)

	// Even Owner scope is subject to manifest integrity checks.
	if err:=os.WriteFile(filepath.Join(capsules[0].Path,showcapsule.ManifestChecksumFile),[]byte("invalid"),0o600);err!=nil{t.Fatal(err)}
	ownerResult:=get(api.Handler(),owner.Token,pathA)
	if ownerResult.Code!=http.StatusOK || strings.Contains(ownerResult.Body.String(),capsules[0].CapsuleID) ||
		!strings.Contains(ownerResult.Body.String(),`"capsules":[]`) {
		t.Fatalf("tampered capsule remained visible: %d %s",ownerResult.Code,ownerResult.Body.String())
	}
}

func TestScopedCapsuleExportListFailsClosedForSymlinkRootAndTooManyItems(t *testing.T) {
	h := newAuthHarness(t)
	ctx:=context.Background()
	st:=store.New(h.db.DB,clock.Real{})
	show,_,err:=st.CreateProject(ctx,store.CreateProjectParams{Name:"Test Show"})
	if err!=nil {t.Fatal(err)}
	owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err!=nil {t.Fatal(err)}
	access,err:=projectaccess.New(h.db.DB)
	if err!=nil {t.Fatal(err)}
	base:=t.TempDir()
	realRoot:=filepath.Join(base,"controlled")
	if err:=os.MkdirAll(filepath.Join(realRoot,"exports"),0o750);err!=nil {t.Fatal(err)}
	symRoot:=filepath.Join(base,"symlinked")
	if err:=os.MkdirAll(symRoot,0o750);err!=nil {t.Fatal(err)}
	if err:=os.Symlink(filepath.Join(realRoot,"exports"),filepath.Join(symRoot,"exports"));err!=nil {t.Fatal(err)}
	get:=func(root string) *httptest.ResponseRecorder {
		t.Helper()
		api:=New(WithUserAuth(h.auth,h.hub),WithScopedLocalCapsuleExports(h.auth,access,root))
		req:=httptest.NewRequest(http.MethodGet,"/api/v1/projects/"+show.ID+"/show-capsules/exports",nil)
		req.RemoteAddr="127.0.0.1:18110"
		req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
		w:=httptest.NewRecorder()
		api.Handler().ServeHTTP(w,req)
		return w
	}
	if got:=get(symRoot);got.Code!=http.StatusServiceUnavailable {
		t.Fatalf("symlinked exports root served: %d %s",got.Code,got.Body.String())
	}
	for i:=0;i<maxScopedCapsuleScanEntries+1;i++{
		if err:=os.Mkdir(filepath.Join(realRoot,"exports",
			fmt.Sprintf("untrusted-%03d",i)),0o700);err!=nil {t.Fatal(err)}
	}
	if got:=get(realRoot);got.Code!=http.StatusServiceUnavailable ||
		strings.Contains(got.Body.String(),show.Name) {
		t.Fatalf("large export inventory leaked or truncated: %d %s",got.Code,got.Body.String())
	}
}
