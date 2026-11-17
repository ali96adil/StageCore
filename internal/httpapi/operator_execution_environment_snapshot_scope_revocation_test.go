package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestExecutionEnvironmentSnapshotScopeErrorsSuppressResultAfterGrantRevocation(t *testing.T) {
	for _, scopeError := range []string{"foreign-environment", "foreign-detail", "foreign-diff"} {
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
			project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Snapshot scope", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			foreignProject, foreignRevision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Foreign snapshot scope", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			localEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, testVDMXExecutionEnvironmentManifest("local-snapshot"), owner.Session.User.ID)
			if err != nil {
				t.Fatal(err)
			}
			otherEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, testVDMXExecutionEnvironmentManifest("other-snapshot"), owner.Session.User.ID)
			if err != nil {
				t.Fatal(err)
			}
			foreignEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, foreignRevision.ID, testVDMXExecutionEnvironmentManifest("foreign-snapshot"), owner.Session.User.ID)
			if err != nil {
				t.Fatal(err)
			}
			localSnapshot, err := stageStore.CreateExecutionEnvironmentSnapshot(ctx, localEnvironment.ID, testSnapshotForEnvironment(localEnvironment), owner.Session.User.ID)
			if err != nil {
				t.Fatal(err)
			}
			otherSnapshot, err := stageStore.CreateExecutionEnvironmentSnapshot(ctx, otherEnvironment.ID, testSnapshotForEnvironment(otherEnvironment), owner.Session.User.ID)
			if err != nil {
				t.Fatal(err)
			}
			operator, err := h.auth.CreateUser(ctx, "snapshot-scope-operator", h.password, userauth.RoleOperator)
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
			expectedRoute := "snapshot-list-environment-mismatch"
			path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + foreignEnvironment.ID + "/snapshots"
			switch scopeError {
			case "foreign-detail":
				expectedRoute = "snapshot-detail-snapshot-mismatch"
				path = "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + localEnvironment.ID + "/snapshots/" + otherSnapshot.ID
			case "foreign-diff":
				expectedRoute = "snapshot-diff-snapshot-mismatch"
				path = "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + localEnvironment.ID +
					"/snapshots/diff?before_snapshot_id=" + localSnapshot.ID + "&after_snapshot_id=" + otherSnapshot.ID
			}
			_ = foreignProject
			revoked := false
			handler := New(
				withOperatorExecutionEnvironments(h.auth, stageStore, executionEnvironmentReadHooks{
					BeforeResponse: func(route string) {
						if route != expectedRoute {
							return
						}
						revoked = true
						if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
							t.Errorf("revoke MediaRead: %v", err)
						}
					},
				}),
				WithProjectRouteQualification(access),
			).Handler()
			request := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("%s scope response hook did not revoke grant: status=%d body=%s", scopeError, response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "EXECUTION_ENVIRONMENT_") {
				t.Fatalf("revoked %s scope response escaped: status=%d body=%s", scopeError, response.Code, response.Body.String())
			}
		})
	}
}

func testSnapshotForEnvironment(environment store.ExecutionEnvironmentManifest) executionenv.Snapshot {
	return executionenv.Snapshot{
		SchemaVersion:             executionenv.SnapshotSchemaVersion,
		EnvironmentKey:            environment.Manifest.EnvironmentKey,
		AdapterKey:                environment.Manifest.AdapterKey,
		SourceManifestSHA256:      environment.ContentSHA256,
		CaptureStatus:             executionenv.SnapshotPartial,
		RebuildPlanVersion:        executionenv.SnapshotRebuildPlanVersion,
		ReconstructionFingerprint: strings.Repeat("d", 64),
		RebuildPlan: []executionenv.SnapshotRebuildStep{{
			Step: 1, Action: "Inspect observed state", Status: "OBSERVED",
			ProvenanceClass: executionenv.SnapshotProvenanceObserved,
		}},
	}
}
