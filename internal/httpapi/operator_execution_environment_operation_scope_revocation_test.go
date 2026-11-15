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

func TestExecutionEnvironmentOperationScopeMismatchSuppressesResultAfterGrantRevocation(t *testing.T) {
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
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Operation scope route", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, foreignRevision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Foreign operation environment", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, foreignRevision.ID, testVDMXExecutionEnvironmentManifest("foreign-operation"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "operation-scope-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.RehearsalControl, projectaccess.MediaRead,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &fakeExecutionEnvironmentOperationRuntime{status: companionchannel.EnvironmentOperationCompleted}
	revoked := false
	handler := New(
		withOperatorExecutionEnvironmentOperations(h.auth, stageStore, runtime, executionEnvironmentOperationHooks{
			BeforeResponse: func(route string) {
				if route != "operation-environment-mismatch" {
					return
				}
				revoked = true
				if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
					t.Errorf("revoke runtime grant: %v", err)
				}
			},
		}),
		WithProjectRouteQualification(access),
	).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID +
		"/execution-environments/" + environment.ID + "/operations"
	request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, path,
		[]byte(`{"operation_id":"scope-mismatch","kind":"OPEN","timeout_ms":1000}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatalf("operation scope hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "EXECUTION_ENVIRONMENT_NOT_FOUND") {
		t.Fatalf("revoked operation scope response escaped: status=%d body=%s", response.Code, response.Body.String())
	}
	if runtime.calls != 0 {
		t.Fatalf("scope mismatch dispatched %d operations after grant revocation", runtime.calls)
	}
}
