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

func TestExecutionEnvironmentMutationsSuppressCommittedResultsAfterGrantRevocation(t *testing.T) {
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
				Name: "Environment mutation revocation " + operation, CreatedBy: owner.Session.User.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
				RoleKey: "VIDEO-" + strings.ToUpper(operation), DisplayName: "Video " + operation,
			})
			if err != nil {
				t.Fatal(err)
			}
			var environment store.ExecutionEnvironmentManifest
			if operation != "create" {
				environment, err = stageStore.CreateExecutionEnvironmentManifest(
					ctx, revision.ID, testVDMXExecutionEnvironmentManifest("mutation-"+operation), owner.Session.User.ID,
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			operatorName := "env-mutation-" + operation
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
			route := "environment-" + operation
			handler := New(withOperatorExecutionEnvironments(h.auth, stageStore, executionEnvironmentReadHooks{
				BeforeResponse: func(actual string) {
					if actual != route {
						return
					}
					revoked = true
					if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
						t.Errorf("revoke Project grant after %s: %v", operation, err)
					}
				},
			}), WithProjectRouteQualification(access)).Handler()
			collection := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments"
			var method, path string
			var body []byte
			switch operation {
			case "create":
				method, path = http.MethodPost, collection
				body, err = json.Marshal(map[string]any{"manifest": testVDMXExecutionEnvironmentManifest("new-after-revoke")})
			case "update":
				method, path = http.MethodPut, collection+"/"+environment.ID
				manifest := environment.Manifest
				manifest.Name = "committed update"
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
				t.Fatalf("%s response hook did not revoke grant: status=%d body=%s", operation, response.Code, response.Body.String())
			}
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
				t.Fatalf("revoked %s returned status=%d body=%s", operation, response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("revoked %s response Cache-Control=%q", operation, response.Header().Get("Cache-Control"))
			}
			switch operation {
			case "create":
				items, err := stageStore.ListExecutionEnvironmentManifests(ctx, revision.ID)
				if err != nil || len(items) != 1 || items[0].Manifest.EnvironmentKey != "new-after-revoke" {
					t.Fatalf("create effect was not retained: items=%+v err=%v", items, err)
				}
			case "update":
				stored, err := stageStore.GetExecutionEnvironmentManifest(ctx, environment.ID)
				if err != nil || stored.Manifest.Name != "committed update" {
					t.Fatalf("update effect was not retained: item=%+v err=%v", stored, err)
				}
			case "bind":
				stored, err := stageStore.GetExecutionEnvironmentManifest(ctx, environment.ID)
				if err != nil || stored.MachineRoleID == nil || *stored.MachineRoleID != role.ID {
					t.Fatalf("binding effect was not retained: item=%+v err=%v", stored, err)
				}
			case "delete":
				if _, err := stageStore.GetExecutionEnvironmentManifest(ctx, environment.ID); err == nil {
					t.Fatal("delete effect was not retained after response was suppressed")
				}
			}
		})
	}
}
