package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestScopedProjectFilesIsOptInAndNeverFallsBackToGlobalProjectRole(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	a, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Public rehearsal"})
	if err != nil { t.Fatal(err) }
	b, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Private show"})
	if err != nil { t.Fatal(err) }
	operator, err := h.auth.CreateUser(ctx, "files-operator", h.password, userauth.RoleOperator)
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	op, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	access, err := projectaccess.New(h.db.DB)
	if err != nil { t.Fatal(err) }

	// An unconfigured Hub must NOT publish a Project Files route.
	defaultServer := New(WithUserAuth(h.auth, h.hub))
	noOptIn := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+a.ID+"/files", nil)
	noOptIn.RemoteAddr = "127.0.0.1:42001"
	noOptIn.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	noOptInResponse := httptest.NewRecorder()
	defaultServer.Handler().ServeHTTP(noOptInResponse, noOptIn)
	if noOptInResponse.Code != http.StatusNotFound {
		t.Fatalf("Project Files unexpectedly enabled by default: %d", noOptInResponse.Code)
	}

	api := New(WithUserAuth(h.auth, h.hub), WithScopedProjectFilesRead(h.auth, access))
	get := func(token, path string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = "127.0.0.1:42001"
		if token != "" { request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token}) }
		response := httptest.NewRecorder()
		api.Handler().ServeHTTP(response, request)
		return response
	}
	listA := "/api/v1/projects/"+a.ID+"/files"
	listB := "/api/v1/projects/"+b.ID+"/files"
	probeA := listA+"/availability"
	if got := get("",listA); got.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous file metadata read %d",got.Code)
	}
	if got := get(op.Token,listA); got.Code != http.StatusNotFound {
		t.Fatalf("globally authorized OPERATOR without media grant saw Files: %d %s",got.Code,got.Body.String())
	}
	if got := get(op.Token,probeA); got.Code != http.StatusNotFound {
		t.Fatalf("ungranted availability leaked: %d",got.Code)
	}
	if err := access.SetPermissions(ctx,owner.Session.User.ID,operator.ID,a.ID,
		[]projectaccess.Permission{projectaccess.ProjectView});err!=nil {t.Fatal(err)}
	if got := get(op.Token,listA);got.Code != http.StatusNotFound {
		t.Fatalf("project.view accidentally gave media metadata: %d",got.Code)
	}
	if err := access.SetPermissions(ctx,owner.Session.User.ID,operator.ID,a.ID,
		[]projectaccess.Permission{projectaccess.ProjectView,projectaccess.MediaRead});err!=nil {t.Fatal(err)}
	if got := get(op.Token,probeA);got.Code != http.StatusNoContent {
		t.Fatalf("valid Project Files availability = %d %s",got.Code,got.Body.String())
	}
	if got := get(op.Token,listA);got.Code != http.StatusOK ||
		!strings.Contains(got.Body.String(), `"read_only":true`) ||
		!strings.Contains(got.Body.String(),a.ID) ||
		!strings.Contains(got.Body.String(),`"files":[]`) ||
		!strings.Contains(got.Header().Get("Cache-Control"),"no-store") {
		t.Fatalf("valid empty workspace not returned: %d %s",got.Code,got.Body.String())
	}
	if got := get(op.Token,listB);got.Code != http.StatusNotFound ||
		strings.Contains(got.Body.String(),b.Name) {
		t.Fatalf("Project B name leaked: %d %s",got.Code,got.Body.String())
	}
	for _, path := range []string{
		"/api/v1/projects/unknown-project/files",
		"/api/v1/projects/unknown-project/files/availability",
	} {
		if got := get(op.Token,path);got.Code != http.StatusNotFound {
			t.Fatalf("guessable ProjectID endpoint=%s status=%d",path,got.Code)
		}
	}

	media, err := st.RegisterManagedMedia(ctx,store.RegisterManagedMediaParams{
		ProjectID: a.ID, Name: "Opening audio", AssetPolicy: store.MediaPolicyManaged,
		ContentHash: strings.Repeat("c",64), SizeBytes: 42,
		RelativePath: "objects/audio", OriginalFilename: "opening.wav",
	})
	if err != nil {t.Fatal(err)}
	if got := get(op.Token,listA);got.Code != http.StatusOK ||
		!strings.Contains(got.Body.String(),media.Asset.ID) ||
		!strings.Contains(got.Body.String(),`"category":"AUDIO"`) {
		t.Fatalf("authorized file list missing media: %d %s",got.Code,got.Body.String())
	}
	if err := access.SetPermissions(ctx,owner.Session.User.ID,operator.ID,a.ID,nil);err!=nil {t.Fatal(err)}
	if got := get(op.Token,listA);got.Code != http.StatusNotFound {
		t.Fatalf("revoked access could read files: %d",got.Code)
	}
	if got := get(op.Token,probeA);got.Code != http.StatusNotFound {
		t.Fatalf("revoked availability still visible: %d",got.Code)
	}
	if got := get(owner.Token,listB);got.Code != http.StatusOK {
		t.Fatalf("Owner empty workspace unavailable: %d",got.Code)
	}
}

func TestScopedProjectFilesRejectsInsecureRemoteTransport(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	a,_,err:=st.CreateProject(ctx,store.CreateProjectParams{Name:"Secure Project"})
	if err!=nil {t.Fatal(err)}
	owner,err:=h.auth.Login(ctx,"owner",h.password,"127.0.0.1")
	if err!=nil {t.Fatal(err)}
	access,err:=projectaccess.New(h.db.DB)
	if err!=nil {t.Fatal(err)}
	api:=New(WithUserAuth(h.auth,h.hub),WithScopedProjectFilesRead(h.auth,access))
	req:=httptest.NewRequest(http.MethodGet,"/api/v1/projects/"+a.ID+"/files",nil)
	req.RemoteAddr="192.168.7.2:42001"
	req.AddCookie(&http.Cookie{Name:browserSessionCookie,Value:owner.Token})
	w:=httptest.NewRecorder()
	api.Handler().ServeHTTP(w,req)
	if w.Code!=http.StatusUpgradeRequired {t.Fatalf("insecure Project Files HTTP status %d",w.Code)}
}
