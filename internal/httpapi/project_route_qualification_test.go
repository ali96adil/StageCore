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

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
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

func TestQualifiedEnvironmentOperationSuppressesResultAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "revoke-operation-grant", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Operation Grant Revocation"})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("video"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	setPermissions := func(permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, permissions); err != nil {
			t.Fatal(err)
		}
	}
	setPermissions(projectaccess.ProjectView, projectaccess.MediaRead, projectaccess.RehearsalControl)
	runtime := &fakeExecutionEnvironmentOperationRuntime{
		status: companionchannel.EnvironmentOperationCompleted, summary: "private completed VDMX operation",
		beforeReply: func() { setPermissions(projectaccess.ProjectView, projectaccess.MediaRead) },
	}
	handler := New(WithOperatorExecutionEnvironmentOperations(h.auth, stageStore, runtime), WithProjectRouteQualification(access)).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/operations"
	req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, path,
		[]byte(`{"operation_id":"grant-revoked-after-operation","kind":"OPEN","timeout_ms":5000}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if runtime.calls != 1 {
		t.Fatalf("runtime calls=%d, want one operation before revocation", runtime.calls)
	}
	if err := access.Check(ctx, operator.ID, project.ID, projectaccess.ProjectView); err != nil {
		t.Fatalf("test must preserve project visibility while revoking operation grant: %v", err)
	}
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), runtime.summary) ||
		!strings.Contains(response.Body.String(), `"effect_may_have_been_committed":true`) ||
		!strings.Contains(response.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked operator received operation result or retryable response: HTTP %d %s", response.Code, response.Body.String())
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

func TestProjectRouteQualificationWithMissingServiceFailsClosed(t *testing.T) {
	const path = "/api/v1/projects/project-id"
	register := func(s *Server) {
		s.mux.HandleFunc("GET "+projectRoute, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}

	request := httptest.NewRequest(http.MethodGet, path, nil)
	misconfigured := New(WithProjectRouteQualification(nil), register).Handler()
	response := httptest.NewRecorder()
	misconfigured.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "PROJECT_ACCESS_UNAVAILABLE") {
		t.Fatalf("explicit qualification with missing ACL service returned %d %q", response.Code, response.Body.String())
	}

	// The qualification switch is opt-in; the ordinary production legacy
	// assembly remains unchanged until a complete coordinated cutover.
	legacy := New(register).Handler()
	response = httptest.NewRecorder()
	legacy.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("legacy assembly changed without qualification option: %d %q", response.Code, response.Body.String())
	}
}

func TestScopedProjectCapabilityCannotReplaceUnrelatedGlobalPermission(t *testing.T) {
	if scopedProjectPermissionBackedByACL(userauth.PermissionCompanionPair, []projectaccess.Permission{projectaccess.DeviceConfigure}) {
		t.Fatal("Project device.configure capability replaced the unrelated global Companion pairing permission")
	}
	if scopedProjectPermissionBackedByACL(userauth.PermissionProjectEdit, []projectaccess.Permission{projectaccess.DeviceConfigure}) != true {
		t.Fatal("Project device.configure capability did not satisfy a project mutation route")
	}
}

func TestQualifiedProjectRoleMatrixAndCrossProjectIsolation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	ownerLogin, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "scoped-role-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.CreateUser(ctx, "scoped-role-viewer", h.password, userauth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	operatorLogin, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	viewerLogin, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	projectA, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Scoped A"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Private B"})
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, ownerLogin.Session.User.ID, operator.ID, projectA.ID, []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.ProjectRename, projectaccess.CueEdit,
	}); err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, ownerLogin.Session.User.ID, viewer.ID, projectA.ID, []projectaccess.Permission{
		projectaccess.ProjectView,
	}); err != nil {
		t.Fatal(err)
	}
	handler := New(
		// Exercise the same composition required at cutover: project-list
		// filtering and route-level authorization must agree in one router.
		WithScopedOperatorProjects(h.auth, projects, access),
		WithOperatorCuePublish(h.auth, projects, publish.New(projects, capability.NewRegistry())),
		WithProjectRouteQualification(access),
	).Handler()
	request := func(login userauth.Credential, method, path, body string, want int) string {
		t.Helper()
		req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, method, path, []byte(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s %s returned %d want %d: %s", method, path, response.Code, want, response.Body.String())
		}
		return response.Body.String()
	}
	if body := request(ownerLogin, http.MethodGet, "/api/v1/projects", "", http.StatusOK); !strings.Contains(body, projectA.ID) || !strings.Contains(body, projectB.ID) {
		t.Fatalf("Owner did not retain Hub-wide Project visibility: %s", body)
	}
	for _, login := range []userauth.Credential{operatorLogin, viewerLogin} {
		if body := request(login, http.MethodGet, "/api/v1/projects", "", http.StatusOK); !strings.Contains(body, projectA.ID) || strings.Contains(body, projectB.ID) {
			t.Fatalf("scoped user Project listing crossed membership: %s", body)
		}
		request(login, http.MethodGet, "/api/v1/projects/"+projectA.ID, "", http.StatusOK)
		request(login, http.MethodGet, "/api/v1/projects/"+projectB.ID, "", http.StatusNotFound)
	}
	request(operatorLogin, http.MethodPatch, "/api/v1/projects/"+projectA.ID, `{"name":"Operator renamed A"}`, http.StatusOK)
	request(operatorLogin, http.MethodPatch, "/api/v1/projects/"+projectB.ID, `{"name":"Must stay private"}`, http.StatusNotFound)
	const cueBody = `{"display_label":"1","name":"Scoped Cue","order_index":1,"cue_type":"STANDARD","criticality":"NORMAL","enabled":true,"execution_policy":{},"actions":[]}`
	request(operatorLogin, http.MethodPost, "/api/v1/projects/"+projectA.ID+"/cues", cueBody, http.StatusCreated)
	request(operatorLogin, http.MethodPost, "/api/v1/projects/"+projectB.ID+"/cues", cueBody, http.StatusNotFound)
	request(operatorLogin, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/cue-editor-recovery", "", http.StatusOK)
	request(viewerLogin, http.MethodPatch, "/api/v1/projects/"+projectA.ID, `{"name":"Viewer cannot rename"}`, http.StatusNotFound)
	request(viewerLogin, http.MethodPost, "/api/v1/projects/"+projectA.ID+"/cues", cueBody, http.StatusNotFound)
	request(viewerLogin, http.MethodGet, "/api/v1/projects/"+projectA.ID+"/cue-editor-recovery", "", http.StatusNotFound)
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
		s.mux.HandleFunc("/api/v1/projects/{alternate_id}/future-methodless", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
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
	get(scoped, "/api/v1/projects/"+a.ID+"/future-methodless", 404)
	get(legacy, "/api/v1/projects/"+b.ID, 200)
	get(legacy, "/api/v1/projects/"+a.ID+"/future-legacy", http.StatusTeapot)
	for _, resource := range []string{"media", "status", "outputs", "bluetooth"} {
		get(scoped, "/api/v1/system/audio/"+resource, 404)
	}
	if len(audio.calls) != 0 {
		t.Fatal("denied audio request reached the sidecar")
	}
	get(legacy, "/api/v1/system/audio/media", 200)
}
