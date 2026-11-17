package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestExecutionEnvironmentRebuildPlanMutationsSuppressCommittedResultsAfterGrantRevocation(t *testing.T) {
	cases := []struct {
		name, method, suffix, query, responseHook string
	}{
		{"seed", http.MethodPost, "/seed", "", "rebuild-plan-seed"},
		{"update", http.MethodPut, "", "", "rebuild-plan-update"},
		{"delete", http.MethodDelete, "", "?confirm=true", "rebuild-plan-delete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
				Name: "Rebuild plan mutation fence " + tc.name, CreatedBy: owner.Session.User.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			environment, err := stageStore.CreateExecutionEnvironmentManifest(
				ctx, revision.ID, testVDMXExecutionEnvironmentManifest("rebuild-plan-mutation-"+tc.name), owner.Session.User.ID,
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
				ReconstructionFingerprint: strings.Repeat("d", 64),
				RebuildPlan: []executionenv.SnapshotRebuildStep{{
					Step: 1, Action: "Open the application", Status: "OBSERVED",
					ProvenanceClass: executionenv.SnapshotProvenanceObserved,
				}},
			}
			snapshot, err := stageStore.CreateExecutionEnvironmentSnapshot(ctx, environment.ID, snapshotValue, owner.Session.User.ID)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := executionenv.SeedAssistedRebuildPlan(snapshot.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name != "seed" {
				if _, err := stageStore.UpsertExecutionEnvironmentRebuildPlan(ctx, environment.ID, snapshot.ID, plan, owner.Session.User.ID); err != nil {
					t.Fatal(err)
				}
			}
			operator, err := h.auth.CreateUser(ctx, "rebuild-plan-mutation-"+tc.name, h.password, userauth.RoleOperator)
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
					if route != tc.responseHook {
						return
					}
					revoked = true
					if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
						t.Errorf("revoke ProjectEdit: %v", err)
					}
				},
			}), WithProjectRouteQualification(access)).Handler()
			base := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID +
				"/execution-environments/" + environment.ID + "/rebuild-plan"
			path := base + tc.suffix + tc.query
			body := []byte(`{"source_snapshot_id":"` + snapshot.ID + `"}`)
			if tc.name == "update" {
				plan.Steps[0].Notes = "Updated by operator"
				plan.Steps[0].ProvenanceClass = executionenv.SnapshotProvenanceUserDeclared
				encoded, err := json.Marshal(executionEnvironmentRebuildPlanUpdateRequest{
					SourceSnapshotID: snapshot.ID,
					Plan:             plan,
				})
				if err != nil {
					t.Fatal(err)
				}
				body = encoded
			}
			if tc.name == "delete" {
				body = nil
			}
			request := authenticatedMutationRequest(t, tc.method, path, body, login.Token, login.CSRFToken)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("post-commit hook did not revoke ProjectEdit: status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) || !strings.Contains(response.Body.String(), `"retryable":false`) {
				t.Fatalf("revoked rebuild-plan %s returned status=%d body=%s", tc.name, response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), `"rebuild_plan_id"`) || strings.Contains(response.Body.String(), `"plan"`) {
				t.Fatalf("revoked rebuild-plan %s exposed committed details: %s", tc.name, response.Body.String())
			}
			stored, err := stageStore.GetExecutionEnvironmentRebuildPlan(ctx, environment.ID)
			if tc.name == "delete" {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("delete commit missing: stored=%+v err=%v", stored, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("committed plan missing: %v", err)
			}
			if tc.name == "update" && (len(stored.Plan.Steps) != 1 || stored.Plan.Steps[0].Notes != "Updated by operator") {
				t.Fatalf("committed update state=%+v", stored.Plan)
			}
		})
	}
}
