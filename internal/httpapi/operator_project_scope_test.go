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

func TestScopedOperatorProjectsAndCapsulesRespectLiveGrants(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	create := func(name string) string {
		t.Helper()
		project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return project.ID
	}
	a, b := create("Operator Rehearsal"), create("Private Legacy Show")
	operator, err := h.auth.CreateUser(ctx, "project-operator", h.password, userauth.RoleOperator)
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
	capsules := &fakeShowCapsuleService{}
	scoped := New(WithScopedOperatorProjects(h.auth, projects, access),
		WithScopedOperatorShowCapsules(h.auth, capsules, t.TempDir(), access)).Handler()
	legacy := New(WithOperatorProjects(h.auth, projects)).Handler()
	request := func(handler http.Handler, method, path, body string, status int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:41000"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeader, login.CSRFToken)
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, res.Code, status, res.Body.String())
		}
		return res.Body.String()
	}
	const collection = "/api/v1/projects"
	pathA, pathB := collection+"/"+a, collection+"/"+b
	if got := request(legacy, "GET", collection, "", 200); !strings.Contains(got, b) {
		t.Fatal("legacy behavior changed")
	}
	if got := request(scoped, "GET", collection, "", 200); strings.Contains(got, a) || strings.Contains(got, b) {
		t.Fatal("no-grant operator saw existing projects")
	}
	request(scoped, "GET", pathB, "", 404)
	request(scoped, "POST", pathB+"/duplicate", `{"name":"Unauthorized copy"}`, 403)
	request(scoped, "POST", collection, `{"name":"Unowned destination"}`, 403)
	request(scoped, "GET", "/api/v1/show-capsules", "", 403)
	request(scoped, "GET", "/api/v1/show-capsules/imports/00000000-0000-7000-8000-000000000611/plan", "", 403)

	set(projectaccess.ProjectView, projectaccess.CapsuleExport)
	if got := request(scoped, "GET", collection, "", 200); !strings.Contains(got, a) || strings.Contains(got, b) {
		t.Fatal("scoped list returned incorrect projects")
	}
	request(scoped, "GET", pathA+"/dashboard", "", 200)
	request(scoped, "PATCH", pathA, `{"name":"Renamed"}`, 404)
	request(scoped, "GET", pathA+"/show-capsule/preview", "", 404)
	request(scoped, "POST", pathA+"/show-capsules", `{}`, 404)

	set(projectaccess.ProjectView, projectaccess.ProjectRename, projectaccess.CapsuleExport, projectaccess.MediaRead)
	request(scoped, "PATCH", pathA, `{"name":"Renamed"}`, 200)
	request(scoped, "GET", pathA+"/show-capsule/preview", "", 200)
	request(scoped, "POST", pathA+"/show-capsules", `{}`, 201)
	request(scoped, "POST", pathB+"/show-capsules", `{}`, 404)
	if capsules.previewCalls != 1 || capsules.exportCalls != 1 || capsules.lastProjectID != a {
		t.Fatalf("unexpected service calls: %+v", capsules)
	}

	set()
	request(scoped, "GET", pathA, "", 404)
	request(scoped, "GET", pathA+"/dashboard", "", 404)
	request(scoped, "POST", pathA+"/show-capsules", `{}`, 404)
}
