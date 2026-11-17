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

func TestExecutionEnvironmentRebuildPlanScopeMismatchSuppressesResultAfterGrantRevocation(t *testing.T) {
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
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Rebuild-plan route scope", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherProject, otherRevision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Rebuild-plan foreign environment", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, otherRevision.ID, testVDMXExecutionEnvironmentManifest("foreign-rebuild-plan"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "rebuild-plan-scope-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withOperatorExecutionEnvironments(h.auth, stageStore, executionEnvironmentReadHooks{
		BeforeResponse: func(route string) {
			if route != "rebuild-plan-environment-mismatch" {
				return
			}
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Errorf("revoke ProjectView: %v", err)
			}
		},
	}), WithProjectRouteQualification(access)).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID +
		"/execution-environments/" + environment.ID + "/rebuild-plan"
	request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatalf("mismatch response hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "EXECUTION_ENVIRONMENT_NOT_FOUND") {
		t.Fatalf("revoked scope mismatch returned status=%d body=%s", response.Code, response.Body.String())
	}
	if otherProject.ID == project.ID {
		t.Fatal("test projects unexpectedly share an identity")
	}
}
