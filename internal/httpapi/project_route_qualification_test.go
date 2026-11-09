package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestQualifiedEnvironmentOperationChecksPartialAndRevokedGrants(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	user, err := h.auth.CreateUser(ctx, "qualification", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, user.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	makeEnvironment := func(name string) (string, string) {
		t.Helper()
		p, rev, err := st.CreateProject(ctx, store.CreateProjectParams{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		env, err := st.CreateExecutionEnvironmentManifest(ctx, rev.ID, testVDMXExecutionEnvironmentManifest("video"), owner.Session.User.ID)
		if err != nil {
			t.Fatal(err)
		}
		return "/api/v1/projects/" + p.ID + "/revisions/" + rev.ID + "/execution-environments/" + env.ID + "/operations", p.ID
	}
	a, project := makeEnvironment("A")
	b, _ := makeEnvironment("B")
	apply := func(perms ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, user.ID, project, perms); err != nil {
			t.Fatal(err)
		}
	}
	runtime := &fakeExecutionEnvironmentOperationRuntime{status: companionchannel.EnvironmentOperationCompleted}
	handler := New(WithOperatorExecutionEnvironmentOperations(h.auth, st, runtime), WithProjectRouteQualification(access)).Handler()
	do := func(path string, want int) {
		t.Helper()
		req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, path, []byte(`{"operation_id":"check","kind":"OPEN"}`))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("status %d want %d: %s", res.Code, want, res.Body)
		}
	}
	apply(projectaccess.ProjectView, projectaccess.RehearsalControl)
	do(a, 404)
	apply(projectaccess.ProjectView, projectaccess.RehearsalControl, projectaccess.MediaRead)
	do(b, 404)
	do(a, 200)
	apply()
	do(a, 404)
	if runtime.calls != 1 {
		t.Fatalf("runtime dispatched %d times", runtime.calls)
	}
}
