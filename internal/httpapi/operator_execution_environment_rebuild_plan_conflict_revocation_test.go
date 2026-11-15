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

func TestExecutionEnvironmentRebuildPlanConflictSuppressesPlanIDAfterGrantRevocation(t *testing.T) {
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
		Name: "Rebuild plan conflict fence", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("rebuild-plan-conflict"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshotValue := executionenv.Snapshot{
		SchemaVersion:             executionenv.SnapshotSchemaVersion,
		EnvironmentKey:            environment.Manifest.EnvironmentKey,
		AdapterKey:                environment.Manifest.AdapterKey,
		SourceManifestSHA256:      environment.ContentSHA256,
		CaptureStatus:             executionenv.SnapshotPartial,
		RebuildPlanVersion:        executionenv.SnapshotRebuildPlanVersion,
		ReconstructionFingerprint: strings.Repeat("c", 64),
		RebuildPlan: []executionenv.SnapshotRebuildStep{{
			Step: 1, Action: "Open the application", Status: "OBSERVED",
			ProvenanceClass: executionenv.SnapshotProvenanceObserved,
		}},
	}
	snapshot, err := stageStore.CreateExecutionEnvironmentSnapshot(ctx, environment.ID, snapshotValue, owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := executionenv.SeedAssistedRebuildPlan(snapshot.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.UpsertExecutionEnvironmentRebuildPlan(ctx, environment.ID, snapshot.ID, seeded, owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "rebuild-plan-conflict-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}); err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(withOperatorExecutionEnvironments(h.auth, stageStore, executionEnvironmentReadHooks{
		BeforeResponse: func(route string) {
			if route != "rebuild-plan-existing" {
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
		[]byte(`{"source_snapshot_id":"`+snapshot.ID+`"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatalf("existing-plan boundary did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "rebuild_plan_id") {
		t.Fatalf("revoked rebuild-plan conflict returned status=%d body=%s", response.Code, response.Body.String())
	}
}
