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

func TestExecutionEnvironmentRebuildPlanSourceReadSuppressesStoreErrorAfterGrantRevocation(t *testing.T) {
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
	operator, err := h.auth.CreateUser(ctx, "rebuild-plan-source-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Rebuild plan source fence", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("rebuild-plan-source-error"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE execution_environment_snapshots RENAME COLUMN snapshot_json TO snapshot_fault"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE execution_environment_snapshots RENAME COLUMN snapshot_fault TO snapshot_json"); err != nil {
			t.Errorf("restore execution environment snapshot schema: %v", err)
		}
	}()
	revoked := false
	handler := New(withOperatorExecutionEnvironments(h.auth, stageStore, executionEnvironmentReadHooks{
		BeforeStoreErrorResponse: func(route string) {
			if route != "rebuild-plan-source-read" {
				return
			}
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Errorf("revoke project.edit: %v", err)
			}
		},
	}), WithProjectRouteQualification(access)).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID +
		"/execution-environments/" + environment.ID + "/rebuild-plan/seed"
	request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, path,
		[]byte(`{"source_snapshot_id":"source-snapshot"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatalf("snapshot Store error hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "EXECUTION_ENVIRONMENT_REBUILD_PLAN_SEED_FAILED") {
		t.Fatalf("revoked rebuild-plan seed returned status=%d body=%s", response.Code, response.Body.String())
	}
}
