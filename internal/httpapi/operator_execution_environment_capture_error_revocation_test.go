package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestExecutionEnvironmentCaptureErrorSuppressesResultAfterGrantRevocation(t *testing.T) {
	for _, operation := range []string{"frozen", "store-write"} {
		t.Run(operation, func(t *testing.T) {
			h := newAuthHarness(t)
			ctx := context.Background()
			stageStore := store.New(h.db.DB, clock.Real{})
			access, err := projectaccess.New(h.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			objects, err := vault.Open(t.TempDir(), stageStore)
			if err != nil {
				t.Fatal(err)
			}
			owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
				Name: "Capture error revocation " + operation, CreatedBy: owner.Session.User.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			environment, err := stageStore.CreateExecutionEnvironmentManifest(
				ctx, revision.ID, testVDMXExecutionEnvironmentManifest("capture-"+operation), owner.Session.User.ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "frozen" {
				if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := h.db.DB.ExecContext(ctx, `CREATE TRIGGER fail_environment_capture
					BEFORE UPDATE ON execution_environment_manifests
					BEGIN SELECT RAISE(ABORT, 'injected capture update failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			operator, err := h.auth.CreateUser(ctx, "capture-error-"+operation, h.password, userauth.RoleOperator)
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
			if err != nil {
				t.Fatal(err)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{
				projectaccess.ProjectView, projectaccess.ProjectEdit, projectaccess.MediaUpload,
			}); err != nil {
				t.Fatal(err)
			}
			expectedHook := "capture-revision-frozen"
			if operation == "store-write" {
				expectedHook = "capture-store-write"
			}
			revoked := false
			handler := New(
				withOperatorExecutionEnvironmentCapture(h.auth, stageStore, objects, executionEnvironmentCaptureHooks{
					BeforeCaptureErrorResponse: func(route string) {
						if route != expectedHook {
							return
						}
						revoked = true
						if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
							t.Errorf("revoke capture grant: %v", err)
						}
					},
				}),
				WithProjectRouteQualification(access),
			).Handler()
			path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/assets/workspace/capture"
			payload := []byte("capture payload " + operation)
			digest := sha256.Sum256(payload)
			hash := hex.EncodeToString(digest[:])
			request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodPost, path, payload)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("%s capture error hook did not revoke grant: status=%d body=%s", operation, response.Code, response.Body.String())
			}
			projectDenied := response.Code == http.StatusNotFound && strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`)
			roleDenied := response.Code == http.StatusForbidden && strings.Contains(response.Body.String(), `"error_code":"FORBIDDEN"`)
			if (!projectDenied && !roleDenied) || strings.Contains(response.Body.String(), "CAPTURE_FAILED") {
				t.Fatalf("revoked %s capture error escaped: status=%d body=%s", operation, response.Code, response.Body.String())
			}
			if _, err := stageStore.GetVaultObject(ctx, hash); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("failed %s capture left a Vault object: %v", operation, err)
			}
		})
	}
}

func TestExecutionEnvironmentAssetScopeErrorsSuppressResultAfterGrantRevocation(t *testing.T) {
	for _, scopeError := range []string{"foreign-environment", "missing-asset"} {
		t.Run(scopeError, func(t *testing.T) {
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
			project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Asset scope error", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			_, foreignRevision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Foreign Environment", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			environment, err := stageStore.CreateExecutionEnvironmentManifest(
				ctx, foreignRevision.ID, testVDMXExecutionEnvironmentManifest("foreign-asset"), owner.Session.User.ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if scopeError == "missing-asset" {
				environment, err = stageStore.CreateExecutionEnvironmentManifest(
					ctx, revision.ID, testVDMXExecutionEnvironmentManifest("local-asset"), owner.Session.User.ID,
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			operator, err := h.auth.CreateUser(ctx, "asset-scope-operator", h.password, userauth.RoleOperator)
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
			if err != nil {
				t.Fatal(err)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{
				projectaccess.ProjectView, projectaccess.MediaRead,
			}); err != nil {
				t.Fatal(err)
			}
			objects, err := vault.Open(t.TempDir(), stageStore)
			if err != nil {
				t.Fatal(err)
			}
			revoked := false
			handler := New(withOperatorExecutionEnvironmentCapture(h.auth, stageStore, objects, executionEnvironmentCaptureHooks{
				BeforeAssetReadErrorResponse: func() {
					revoked = true
					if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
						t.Errorf("revoke asset read grant: %v", err)
					}
				},
			}), WithProjectRouteQualification(access)).Handler()
			assetKey := "workspace"
			if scopeError == "missing-asset" {
				assetKey = "not-present"
			}
			path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/assets/" + assetKey + "/vault-status"
			request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("%s asset scope hook did not revoke grant: status=%d body=%s", scopeError, response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "EXECUTION_ENVIRONMENT_") {
				t.Fatalf("revoked %s asset scope response escaped: status=%d body=%s", scopeError, response.Code, response.Body.String())
			}
		})
	}
}
