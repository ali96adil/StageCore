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
	"github.com/ali96adil/StageCore/internal/showcapsule"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOwnerCapsuleImportReviewIsReadOnlyAndStrictlyOwnerOnly(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	v, err := vault.Open(t.TempDir(), st)
	if err != nil { t.Fatal(err) }
	capsuleService, err := showcapsule.New(st, v, clock.Real{})
	if err != nil { t.Fatal(err) }
	show, revision, err := st.CreateProject(ctx, store.CreateProjectParams{
		Name: "قريه صالح", CreatedBy: "owner",
	})
	if err != nil { t.Fatal(err) }
	if _, err := st.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening",
		OrderIndex: 1, Enabled: true,
	}, nil); err != nil { t.Fatal(err) }
	if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snap, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, "owner")
	if err != nil { t.Fatal(err) }

	root := t.TempDir()
	exported, err := capsuleService.Export(ctx, show.ID, filepath.Join(root, "exports"),
		showcapsule.BuildOptions{Mode: showcapsule.ExportManifestOnly})
	if err != nil { t.Fatal(err) }
	importRoot := filepath.Join(root, "imports")
	if err := os.MkdirAll(importRoot, 0o750); err != nil { t.Fatal(err) }
	importPath := filepath.Join(importRoot, exported.CapsuleID)
	if err := os.Rename(exported.Path, importPath); err != nil { t.Fatal(err) }

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	type actor struct{ name, role string }
	var nonOwners []string
	for _, actor := range []actor{
		{"review-operator", userauth.RoleOperator},
		{"review-technician", userauth.RoleTechnician},
		{"review-viewer", userauth.RoleViewer},
	} {
		if _, err := h.auth.CreateUser(ctx, actor.name, h.password, actor.role); err != nil {
			t.Fatal(err)
		}
		credentials, err := h.auth.Login(ctx, actor.name, h.password, "127.0.0.1")
		if err != nil { t.Fatal(err) }
		nonOwners = append(nonOwners, credentials.Token)
	}

	old := New(WithUserAuth(h.auth,h.hub))
	s := New(WithUserAuth(h.auth,h.hub), WithOwnerCapsuleImportReview(h.auth,capsuleService,root))
	get := func(handler http.Handler, token, path string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = "127.0.0.1:19800"
		if token != "" { request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token}) }
		w := httptest.NewRecorder()
		handler.ServeHTTP(w,request)
		return w
	}
	inventory := "/api/v1/show-capsules/imports/review"
	detail := "/api/v1/show-capsules/imports/"+exported.CapsuleID+"/review"
	if got:=get(old.Handler(), owner.Token,inventory);got.Code!=http.StatusNotFound{
		t.Fatalf("owner review route unexpectedly enabled by default: %d",got.Code)
	}
	if got:=get(s.Handler(),"",inventory);got.Code!=http.StatusUnauthorized{
		t.Fatalf("anonymous could inspect imported Capsules: %d",got.Code)
	}
	for _,token:=range nonOwners{
		for _,path:=range []string{inventory,detail}{
			if got:=get(s.Handler(),token,path);got.Code!=http.StatusForbidden ||
				strings.Contains(got.Body.String(),show.Name) ||
				strings.Contains(got.Body.String(),exported.CapsuleID) {
				t.Fatalf("non-Owner read import metadata: %d %s",got.Code,got.Body.String())
			}
		}
	}
	got:=get(s.Handler(),owner.Token,inventory)
	if got.Code!=http.StatusOK ||
		!strings.Contains(got.Body.String(),exported.CapsuleID) ||
		!strings.Contains(got.Body.String(),`"authorized_to_restore":false`) ||
		!strings.Contains(got.Body.String(),`"read_only":true`) ||
		!strings.Contains(got.Body.String(),`"skipped_unverified":0`) {
		t.Fatalf("Owner import inventory incorrect: %d %s",got.Code,got.Body.String())
	}
	got=get(s.Handler(),owner.Token,detail)
	if got.Code!=http.StatusOK ||
		!strings.Contains(got.Body.String(),exported.CapsuleID) ||
		!strings.Contains(got.Body.String(),snap.ID) ||
		!strings.Contains(got.Body.String(),`"authorized_to_restore":false`) ||
		!strings.Contains(got.Body.String(),"identity.project_collision") ||
		!strings.Contains(got.Header().Get("Cache-Control"),"no-store") {
		t.Fatalf("Owner import plan preview incorrect: %d %s",got.Code,got.Body.String())
	}
	// Review invokes a planning interface, never materialization.
	if project, err:=st.GetProject(ctx,show.ID);err!=nil || project.ID!=show.ID {
		t.Fatalf("read-only preview changed Project: %v %+v",err,project)
	}
	if got:=get(s.Handler(),owner.Token,
		"/api/v1/show-capsules/imports/11111111-2222-4333-8444-555555555555/review");got.Code!=http.StatusNotFound {
		t.Fatalf("Owner can read nonexistent import: %d",got.Code)
	}
	if got:=get(s.Handler(),owner.Token,"/api/v1/show-capsules/imports/../review");
		got.Code==http.StatusOK {
		t.Fatalf("unsafe import path accepted: %d",got.Code)
	}

	// SHOW state is reflected as a blocking readiness check; preview itself
	// remains read-only and never enters STOP/GO/restore.
	if _,err:=st.CreateSession(ctx,snap.ID,domain.SessionShow,"SHOW safety test");err!=nil {t.Fatal(err)}
	got=get(s.Handler(),owner.Token,detail)
	if got.Code!=http.StatusOK ||
		!strings.Contains(got.Body.String(),"runtime.active_show") ||
		!strings.Contains(got.Body.String(),`"materialization_ready":false`) {
		t.Fatalf("preview falsely declares active SHOW restorable: %d %s",got.Code,got.Body.String())
	}

	// Symlinked content anywhere in an import is excluded from inspection.
	if err:=os.Symlink(filepath.Join(root,"exports"),filepath.Join(importPath,"untrusted-link"));err!=nil {t.Fatal(err)}
	got=get(s.Handler(),owner.Token,inventory)
	if got.Code!=http.StatusOK || strings.Contains(got.Body.String(),exported.CapsuleID) ||
		!strings.Contains(got.Body.String(),`"skipped_unverified":1`) {
		t.Fatalf("import with symlink appeared verified: %d %s",got.Code,got.Body.String())
	}
	if got:=get(s.Handler(),owner.Token,detail);got.Code!=http.StatusNotFound {
		t.Fatalf("symlinked import detail accepted: %d",got.Code)
	}
	if err:=os.Remove(filepath.Join(importPath,"untrusted-link"));err!=nil {t.Fatal(err)}
	if err:=os.WriteFile(filepath.Join(importPath,showcapsule.ManifestChecksumFile),
		[]byte("corrupt"),0o600);err!=nil {t.Fatal(err)}
	got=get(s.Handler(),owner.Token,inventory)
	if got.Code!=http.StatusOK || strings.Contains(got.Body.String(),exported.CapsuleID) {
		t.Fatalf("corrupt import entered verified inventory: %d %s",got.Code,got.Body.String())
	}
	if got:=get(s.Handler(),owner.Token,detail);got.Code!=http.StatusUnprocessableEntity{
		t.Fatalf("corrupt import detail did not fail: %d %s",got.Code,got.Body.String())
	}
}

func TestOwnerCapsuleReviewRejectsSymlinkRootAndInventoryOverflow(t *testing.T) {
	h:=newAuthHarness(t)
	ctx:=context.Background()
	owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err!=nil{t.Fatal(err)}
	root:=t.TempDir()
	actual:=filepath.Join(root,"safe","imports")
	if err:=os.MkdirAll(actual,0o750);err!=nil{t.Fatal(err)}
	fake:=filepath.Join(root,"symlink","imports")
	if err:=os.MkdirAll(filepath.Dir(fake),0o750);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(actual,fake);err!=nil{t.Fatal(err)}
	get:=func(path string) *httptest.ResponseRecorder {
		t.Helper()
		s:=New(WithUserAuth(h.auth,h.hub),WithOwnerCapsuleImportReview(h.auth,&fakeShowCapsuleService{},path))
		req:=httptest.NewRequest(http.MethodGet,"/api/v1/show-capsules/imports/review",nil)
		req.RemoteAddr="127.0.0.1:19800"
		req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
		w:=httptest.NewRecorder()
		s.Handler().ServeHTTP(w,req)
		return w
	}
	if got:=get(filepath.Dir(fake));got.Code!=http.StatusServiceUnavailable {
		t.Fatalf("symlinked imports root accepted: %d %s",got.Code,got.Body.String())
	}
	for i:=0;i<maxOwnerImportEntries+1;i++ {
		name:=fmt.Sprintf("unverified-%03d",i)
		if err:=os.Mkdir(filepath.Join(actual,name),0o700);err!=nil {t.Fatal(err)}
	}
	if got:=get(filepath.Dir(actual));got.Code!=http.StatusServiceUnavailable{
		t.Fatalf("overflow should fail instead of truncation: %d %s",got.Code,got.Body.String())
	}
}
