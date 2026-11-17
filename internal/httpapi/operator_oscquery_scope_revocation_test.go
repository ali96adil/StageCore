package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type fakeOSCQueryScopedRuntime struct {
	fakeExecutionEnvironmentOperationRuntime
	discoveryCalls int
}

func (f *fakeOSCQueryScopedRuntime) DiscoverOSCQuery(context.Context, string, string) companionchannel.OSCQueryDiscoveryResult {
	f.discoveryCalls++
	return companionchannel.OSCQueryDiscoveryResult{Status: "COMPLETED", Name: "simulated VDMX"}
}

func TestOSCQueryEnvironmentScopeMismatchSuppressesResultAfterGrantRevocation(t *testing.T) {
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
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "OSCQuery scope route", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, foreignRevision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Foreign OSCQuery environment", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	foreignEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, foreignRevision.ID, testVDMXExecutionEnvironmentManifest("foreign-oscquery"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	unboundEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("unbound-oscquery"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	unassignedEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("unassigned-oscquery"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{RoleKey: "VDMX-OSCQUERY", Required: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.SetExecutionEnvironmentMachineRole(ctx, unassignedEnvironment.ID, &role.ID); err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "oscquery-scope-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.ProjectEdit, projectaccess.DeviceConfigure,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &fakeOSCQueryScopedRuntime{}
	revoked := false
	expectedHook := ""
	handler := New(
		withOperatorExecutionEnvironmentOperations(h.auth, stageStore, runtime, executionEnvironmentOperationHooks{
			BeforeResponse: func(route string) {
				if route != expectedHook {
					return
				}
				revoked = true
				if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
					t.Errorf("revoke DeviceConfigure grant: %v", err)
				}
			},
		}),
		WithProjectRouteQualification(access),
	).Handler()
	for _, tc := range []struct {
		name          string
		environmentID string
		hook          string
		leakedCode    string
	}{
		{name: "foreign revision", environmentID: foreignEnvironment.ID, hook: "oscquery-environment-mismatch", leakedCode: "VDMX_ENVIRONMENT_NOT_FOUND"},
		{name: "unbound machine role", environmentID: unboundEnvironment.ID, hook: "oscquery-machine-role-unbound", leakedCode: "VDMX_MACHINE_ROLE_NOT_BOUND"},
		{name: "unassigned companion", environmentID: unassignedEnvironment.ID, hook: "oscquery-companion-unassigned", leakedCode: "VDMX_COMPANION_NOT_ASSIGNED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{
				projectaccess.ProjectView, projectaccess.ProjectEdit, projectaccess.DeviceConfigure,
			}); err != nil {
				t.Fatal(err)
			}
			revoked = false
			expectedHook = tc.hook
			path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID +
				"/execution-environments/" + tc.environmentID + "/oscquery/discover"
			request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("OSCQuery %s hook did not revoke grant: status=%d body=%s", tc.name, response.Code, response.Body.String())
			}
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"error_code":"FORBIDDEN"`) || strings.Contains(response.Body.String(), tc.leakedCode) {
				t.Fatalf("revoked OSCQuery %s response escaped: status=%d body=%s", tc.name, response.Code, response.Body.String())
			}
			if runtime.discoveryCalls != 0 {
				t.Fatalf("OSCQuery %s dispatched discovery %d times", tc.name, runtime.discoveryCalls)
			}
		})
	}
}
