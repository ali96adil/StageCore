package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestScopedBrowserMediaDownloadChecksSharedVersionsAndLiveRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	objects, err := vault.Open(t.TempDir(), projects)
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("media"), 55000)
	importFile := func(name string) (string, string) {
		t.Helper()
		project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		media, err := objects.ImportManaged(ctx, vault.ImportParams{ProjectID: project.ID, Name: name, OriginalFilename: "clip.mov"}, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		return project.ID, media.Version.ID
	}
	a, versionA := importFile("Allowed")
	b, versionB := importFile("Other project, same bytes")
	operator, err := h.auth.CreateUser(ctx, "media-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	set := func(permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, a, permissions); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(WithScopedProjectMediaDownload(h.auth, access, projects, objects)).Handler()
	fetch := func(projectID, versionID, byteRange string, res http.ResponseWriter) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/files/"+versionID+"/download", nil)
		req.RemoteAddr = "127.0.0.1:41000"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
		req.Header.Set("Range", byteRange)
		handler.ServeHTTP(res, req)
	}
	assertStatus := func(project, version, byteRange string, status int) *httptest.ResponseRecorder {
		t.Helper()
		res := httptest.NewRecorder()
		fetch(project, version, byteRange, res)
		if res.Code != status {
			t.Fatalf("download returned %d, want %d", res.Code, status)
		}
		return res
	}
	set(projectaccess.ProjectView)
	assertStatus(a, versionA, "", 404)
	set(projectaccess.ProjectView, projectaccess.MediaRead)
	assertStatus(a, versionB, "", 404)
	assertStatus(b, versionB, "", 404)
	res := assertStatus(a, versionA, "bytes=3-9", 206)
	if !bytes.Equal(res.Body.Bytes(), data[3:10]) || !strings.HasPrefix(res.Header().Get("Content-Disposition"), "attachment;") ||
		res.Header().Get("X-Content-Type-Options") != "nosniff" || res.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unsafe or incorrect response: %v %q", res.Header(), res.Body.Bytes())
	}
	stream := &revokeOnMediaWrite{ResponseRecorder: httptest.NewRecorder(), revoke: func() { set() }}
	fetch(a, versionA, "", stream)
	if got := stream.Body.Len(); got == 0 || got > 64<<10 {
		t.Fatalf("grant revocation did not stop stream: %d bytes", got)
	}
	assertStatus(a, versionA, "", 404)
	set(projectaccess.ProjectView, projectaccess.MediaRead)
	stream = &revokeOnMediaWrite{ResponseRecorder: httptest.NewRecorder(), revoke: func() {
		if err := h.auth.Logout(ctx, login.Token); err != nil {
			t.Fatal(err)
		}
	}}
	fetch(a, versionA, "", stream)
	if got := stream.Body.Len(); got == 0 || got > 64<<10 {
		t.Fatalf("session revocation did not stop stream: %d bytes", got)
	}
	assertStatus(a, versionA, "", 401)
}

type revokeOnMediaWrite struct {
	*httptest.ResponseRecorder
	revoke func()
}

func (w *revokeOnMediaWrite) Write(data []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(data)
	if w.revoke != nil {
		w.revoke()
		w.revoke = nil
	}
	return n, err
}

func TestSSEReadyDoesNotUseRevokedSession(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.auth.Logout(ctx, login.Token); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	res := httptest.NewRecorder()
	serveAuthenticatedSSE(res, req, h.auth, h.hub, login.Session)
	if res.Code != http.StatusUnauthorized || strings.Contains(res.Body.String(), "session.ready") {
		t.Fatalf("stale SSE session disclosed readiness: %d %s", res.Code, res.Body.String())
	}
}
