package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"

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

func TestSC024QualifiedPolicyPatternsExistInReviewedInventory(t *testing.T) {
	raw, err := os.ReadFile("../../docs/architecture/sc024-http-route-inventory.tsv")
	if err != nil {
		t.Fatal(err)
	}
	patterns := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		_, pattern, _ := strings.Cut(line, "\t")
		patterns[pattern] = true
		if projectURLPattern(pattern) && !projectRouteIsQualified(pattern) {
			t.Errorf("project URL has no qualification policy: %s", pattern)
		}
	}
	for pattern := range projectRoutePermissions {
		if !patterns[pattern] {
			t.Errorf("qualification route does not exist: %s", pattern)
		}
	}
	for pattern := range ownerRouteQualification {
		if !patterns[pattern] {
			t.Errorf("Owner qualification route does not exist: %s", pattern)
		}
	}
}

func TestQualifiedRawDeviceCommandsRejectScopedOperator(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	p, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Device scope"})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	user, err := h.auth.CreateUser(ctx, "device-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, user.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, user.ID, p.ID, []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.DeviceConfigure, projectaccess.RehearsalControl,
	}); err != nil {
		t.Fatal(err)
	}
	repo, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	runtime := devicechannel.New(repo, nil)
	defer runtime.Close()
	handler := New(WithOperatorStageDevices(h.auth, repo, runtime, st), WithProjectRouteQualification(access)).Handler()
	for _, path := range []string{"/api/v1/stage-devices/device/commands", "/api/v1/projects/" + p.ID + "/stage-device-commands"} {
		req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, path, []byte(`{}`))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusForbidden {
			t.Fatalf("%s: status=%d body=%s", path, res.Code, res.Body.String())
		}
	}
}

func TestQualifiedLegacyProjectsAndAudioRejectForeignAccess(t *testing.T) {
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
	user, err := h.auth.CreateUser(ctx, "legacy-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, user.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Granted rehearsal"})
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Private rehearsal"})
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, user.ID, a.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	audio := &fakeStageAudio{}
	unknown := func(s *Server) {
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/future-legacy", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	}
	opts := []Option{WithOperatorProjects(h.auth, st), WithOperatorStageAudio(h.auth, st, audio, nil), unknown}
	legacy := New(opts...).Handler()
	scoped := New(append(opts, WithProjectRouteQualification(access))...).Handler()
	get := func(handler http.Handler, path string, want int) string {
		t.Helper()
		req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodGet, path, nil)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		if out.Code != want {
			t.Fatalf("%s returned %d want %d: %s", path, out.Code, want, out.Body.String())
		}
		return out.Body.String()
	}
	if body := get(scoped, "/api/v1/projects", 200); !strings.Contains(body, a.ID) || strings.Contains(body, b.ID) {
		t.Fatalf("unscoped list: %s", body)
	}
	get(scoped, "/api/v1/projects/"+a.ID, 200)
	get(scoped, "/api/v1/projects/"+b.ID, 404)
	get(scoped, "/api/v1/projects/"+a.ID+"/future-legacy", 404)
	get(legacy, "/api/v1/projects/"+b.ID, 200)
	get(legacy, "/api/v1/projects/"+a.ID+"/future-legacy", http.StatusTeapot)
	get(scoped, "/api/v1/system/audio/media", 404)
	if len(audio.calls) != 0 {
		t.Fatal("denied audio request reached the sidecar")
	}
	get(legacy, "/api/v1/system/audio/media", 200)
}
