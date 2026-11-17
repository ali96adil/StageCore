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

func TestExecutionEnvironmentReadErrorSuppressesStoreErrorAfterGrantRevocation(t *testing.T) {
	for _, operation := range []string{"update", "bind", "delete"} {
		t.Run(operation, func(t *testing.T) {
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
				Name: "Environment read error revocation " + operation, CreatedBy: owner.Session.User.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			environment, err := stageStore.CreateExecutionEnvironmentManifest(
				ctx, revision.ID, testVDMXExecutionEnvironmentManifest("read-error-"+operation), owner.Session.User.ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			operatorName := "env-read-error-" + operation
			operator, err := h.auth.CreateUser(ctx, operatorName, h.password, userauth.RoleOperator)
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
			if err != nil {
				t.Fatal(err)
			}
			permissions := []projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}
			if operation == "bind" {
				permissions = append(permissions, projectaccess.DeviceConfigure)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, permissions); err != nil {
				t.Fatal(err)
			}
			if _, err := h.db.DB.ExecContext(ctx, "DROP TABLE execution_environment_manifests"); err != nil {
				t.Fatal(err)
			}
			revoked := false
			route := "environment-" + operation + "-read"
			handler := New(withOperatorExecutionEnvironments(h.auth, stageStore, executionEnvironmentReadHooks{
				BeforeStoreErrorResponse: func(actual string) {
					if actual != route {
						return
					}
					revoked = true
					if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
						t.Errorf("revoke Project grant after %s read failure: %v", operation, err)
					}
				},
			}), WithProjectRouteQualification(access)).Handler()
			collection := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments"
			method, path, body := http.MethodPut, collection+"/"+environment.ID, []byte(`{"manifest":{}}`)
			switch operation {
			case "bind":
				path += "/machine-role"
				body = []byte(`{"machine_role_id":null}`)
			case "delete":
				method, path, body = http.MethodDelete, path+"?confirm=true", nil
			}
			request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, method, path, body)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("%s read-error hook did not revoke grant: status=%d body=%s", operation, response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "FAILED") {
				t.Fatalf("revoked %s read error escaped: status=%d body=%s", operation, response.Code, response.Body.String())
			}
		})
	}
}
