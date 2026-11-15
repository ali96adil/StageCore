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
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestExecutionEnvironmentAssetReadSuppressesStoreErrorAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	v, err := vault.Open(t.TempDir(), st)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "environment-asset-error-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Execution environment asset fence", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := st.CreateExecutionEnvironmentManifest(ctx, revision.ID, testVDMXExecutionEnvironmentManifest("asset-error"), owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.MediaRead}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE execution_environment_manifests RENAME COLUMN manifest_json TO manifest_fault"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE execution_environment_manifests RENAME COLUMN manifest_fault TO manifest_json"); err != nil {
			t.Errorf("restore execution environment schema: %v", err)
		}
	}()
	revoked := false
	handler := New(
		WithOperatorExecutionEnvironments(h.auth, st),
		withOperatorExecutionEnvironmentCapture(h.auth, st, v, executionEnvironmentCaptureHooks{BeforeAssetReadErrorResponse: func() {
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Errorf("revoke MediaRead: %v", err)
			}
		}}),
		WithProjectRouteQualification(access),
	).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/assets/workspace/vault-status"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, path, login.Token))
	if !revoked {
		t.Fatalf("asset Store error hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "EXECUTION_ENVIRONMENT_CAPTURE_FAILED") {
		t.Fatalf("revoked asset read returned status=%d body=%s", response.Code, response.Body.String())
	}
}
