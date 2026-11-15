package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestExecutionEnvironmentWriteErrorSuppressesStoreErrorAfterGrantRevocation(t *testing.T) {
	for _, operation := range []string{"create", "update", "bind", "delete"} {
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
				Name: "Environment write error revocation " + operation, CreatedBy: owner.Session.User.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
				RoleKey: "ERROR-" + strings.ToUpper(operation), DisplayName: "Error " + operation,
			})
			if err != nil {
				t.Fatal(err)
			}
			var environment store.ExecutionEnvironmentManifest
			if operation != "create" {
				environment, err = stageStore.CreateExecutionEnvironmentManifest(
					ctx, revision.ID, testVDMXExecutionEnvironmentManifest("error-"+operation), owner.Session.User.ID,
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			trigger := map[string]string{
				"create": `CREATE TRIGGER fail_environment_create BEFORE INSERT ON execution_environment_manifests BEGIN SELECT RAISE(ABORT, 'injected create failure'); END`,
				"update": `CREATE TRIGGER fail_environment_update BEFORE UPDATE ON execution_environment_manifests BEGIN SELECT RAISE(ABORT, 'injected update failure'); END`,
				"bind":   `CREATE TRIGGER fail_environment_bind BEFORE UPDATE ON execution_environment_manifests BEGIN SELECT RAISE(ABORT, 'injected bind failure'); END`,
				"delete": `CREATE TRIGGER fail_environment_delete BEFORE DELETE ON execution_environment_manifests BEGIN SELECT RAISE(ABORT, 'injected delete failure'); END`,
			}[operation]
			if _, err := h.db.DB.ExecContext(ctx, trigger); err != nil {
				t.Fatal(err)
			}
			operatorName := "env-write-error-" + operation
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
			revoked := false
			route := "environment-" + operation + "-write"
			handler := New(withOperatorExecutionEnvironments(h.auth, stageStore, executionEnvironmentReadHooks{
				BeforeStoreErrorResponse: func(actual string) {
					if actual != route {
						return
					}
					revoked = true
					if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
						t.Errorf("revoke Project grant after %s failure: %v", operation, err)
					}
				},
			}), WithProjectRouteQualification(access)).Handler()
			collection := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments"
			var method, path string
			var body []byte
			switch operation {
			case "create":
				method, path = http.MethodPost, collection
				body, err = json.Marshal(map[string]any{"manifest": testVDMXExecutionEnvironmentManifest("failed-create")})
			case "update":
				method, path = http.MethodPut, collection+"/"+environment.ID
				manifest := environment.Manifest
				manifest.Name = "failed update"
				body, err = json.Marshal(map[string]any{"manifest": manifest})
			case "bind":
				method, path = http.MethodPut, collection+"/"+environment.ID+"/machine-role"
				body, err = json.Marshal(map[string]any{"machine_role_id": role.ID})
			case "delete":
				method, path = http.MethodDelete, collection+"/"+environment.ID+"?confirm=true"
			default:
				t.Fatalf("unknown operation %q", operation)
			}
			if err != nil {
				t.Fatal(err)
			}
			request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, method, path, body)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("%s Store-error hook did not revoke grant: status=%d body=%s", operation, response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "FAILED") {
				t.Fatalf("revoked %s Store error escaped: status=%d body=%s", operation, response.Code, response.Body.String())
			}
			switch operation {
			case "create":
				items, err := stageStore.ListExecutionEnvironmentManifests(ctx, revision.ID)
				if err != nil || len(items) != 0 {
					t.Fatalf("failed create changed the store: items=%+v err=%v", items, err)
				}
			case "update":
				stored, err := stageStore.GetExecutionEnvironmentManifest(ctx, environment.ID)
				if err != nil || stored.Manifest.Name == "failed update" {
					t.Fatalf("failed update changed the store: item=%+v err=%v", stored, err)
				}
			case "bind":
				stored, err := stageStore.GetExecutionEnvironmentManifest(ctx, environment.ID)
				if err != nil || stored.MachineRoleID != nil {
					t.Fatalf("failed binding changed the store: item=%+v err=%v", stored, err)
				}
			case "delete":
				if _, err := stageStore.GetExecutionEnvironmentManifest(ctx, environment.ID); err != nil {
					t.Fatalf("failed delete removed Environment: %v", err)
				}
			}
			if environment.ID != "" && response.Body.Len() > 0 && strings.Contains(response.Body.String(), environment.ID) {
				t.Fatalf("revoked %s error disclosed Environment ID: %s", operation, response.Body.String())
			}
		})
	}
}
