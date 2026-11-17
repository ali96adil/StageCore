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

func TestExecutionEnvironmentRevisionReadSuppressesStoreErrorAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "environment-revision-error-operator", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Execution environment revision fence", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE projects RENAME COLUMN current_revision_id TO current_revision_fault"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE projects RENAME COLUMN current_revision_fault TO current_revision_id"); err != nil {
			t.Errorf("restore projects schema: %v", err)
		}
	}()
	revoked := false
	handler := New(
		withOperatorExecutionEnvironments(h.auth, st, executionEnvironmentReadHooks{BeforeStoreErrorResponse: func(route string) {
			if route != "revision-project" {
				t.Errorf("error hook route=%q", route)
			}
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Errorf("revoke ProjectView: %v", err)
			}
		}}),
		WithProjectRouteQualification(access),
	).Handler()
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, path, login.Token))
	if !revoked {
		t.Fatalf("Project read error hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "EXECUTION_ENVIRONMENTS_UNAVAILABLE") {
		t.Fatalf("revoked Execution Environment read returned status=%d body=%s", response.Code, response.Body.String())
	}
}
