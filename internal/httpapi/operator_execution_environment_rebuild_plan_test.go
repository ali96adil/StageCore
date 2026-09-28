package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorExecutionEnvironmentRebuildPlanLifecycle(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorExecutionEnvironments(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Editable rebuild API", CreatedBy: owner.Session.User.ID,
	})
	if err != nil { t.Fatal(err) }
	environment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("video-main"), owner.Session.User.ID,
	)
	if err != nil { t.Fatal(err) }

	snapshotValue := executionenv.Snapshot{
		SchemaVersion:              executionenv.SnapshotSchemaVersion,
		EnvironmentKey:             environment.Manifest.EnvironmentKey,
		AdapterKey:                 environment.Manifest.AdapterKey,
		SourceManifestSHA256:       environment.ContentSHA256,
		CaptureStatus:              executionenv.SnapshotPartial,
		RebuildPlanVersion:         executionenv.SnapshotRebuildPlanVersion,
		ReconstructionFingerprint: strings.Repeat("c", 64),
		RebuildPlan: []executionenv.SnapshotRebuildStep{
			{
				Step: 1, Action: "Open VDMX", Status: "OBSERVED",
				ProvenanceClass: executionenv.SnapshotProvenanceObserved,
			},
			{
				Step: 2, Action: "Recreate unsupported internals manually", Status: "MANUAL",
				ProvenanceClass: executionenv.SnapshotProvenanceUnsupported,
			},
		},
		Items: []executionenv.SnapshotItem{{
			Key: "vdmx-app", Name: "VDMX", Kind: executionenv.SnapshotOther,
			Provenance: executionenv.ProvenanceAdapterObservation,
			ProvenanceClass: executionenv.SnapshotProvenanceObserved,
			Capture: executionenv.ItemObserved,
			Portability: executionenv.SnapshotDescriptiveOnly,
		}},
	}
	snapshot, err := stageStore.CreateExecutionEnvironmentSnapshot(
		ctx, environment.ID, snapshotValue, owner.Session.User.ID,
	)
	if err != nil { t.Fatal(err) }

	path := "/api/v1/projects/" + project.ID +
		"/revisions/" + revision.ID +
		"/execution-environments/" + environment.ID +
		"/rebuild-plan"

	unauth := httptest.NewRequest(http.MethodGet, path, nil)
	unauth.RemoteAddr = "127.0.0.1:14100"
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthRes.Code, unauthRes.Body.String())
	}

	missingReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodGet, path, nil,
	)
	missingRes := httptest.NewRecorder()
	handler.ServeHTTP(missingRes, missingReq)
	if missingRes.Code != http.StatusNotFound {
		t.Fatalf("missing plan status=%d body=%s", missingRes.Code, missingRes.Body.String())
	}

	seedBody, _ := json.Marshal(map[string]any{"source_snapshot_id": snapshot.ID})
	seedReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodPost, path+"/seed", seedBody,
	)
	seedRes := httptest.NewRecorder()
	handler.ServeHTTP(seedRes, seedReq)
	if seedRes.Code != http.StatusCreated {
		t.Fatalf("seed status=%d body=%s", seedRes.Code, seedRes.Body.String())
	}
	var seeded executionEnvironmentRebuildPlanView
	if err := json.Unmarshal(seedRes.Body.Bytes(), &seeded); err != nil { t.Fatal(err) }
	if seeded.ExecutionEnvironmentID != environment.ID ||
		seeded.SourceSnapshotID != snapshot.ID ||
		len(seeded.Plan.Steps) != 2 ||
		seeded.Plan.Steps[0].SourceStep == nil ||
		seeded.Plan.Steps[0].ProvenanceClass != executionenv.SnapshotProvenanceObserved {
		t.Fatalf("seeded=%+v", seeded)
	}

	getReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodGet, path, nil,
	)
	getRes := httptest.NewRecorder()
	handler.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRes.Code, getRes.Body.String())
	}

	edited := seeded.Plan
	edited.Steps[0].Action = "Open VDMX and choose the show workspace"
	invalidBody, _ := json.Marshal(executionEnvironmentRebuildPlanUpdateRequest{
		SourceSnapshotID: snapshot.ID,
		Plan:             edited,
	})
	invalidReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodPut, path, invalidBody,
	)
	invalidRes := httptest.NewRecorder()
	handler.ServeHTTP(invalidRes, invalidReq)
	if invalidRes.Code != http.StatusBadRequest {
		t.Fatalf("invalid edit status=%d body=%s", invalidRes.Code, invalidRes.Body.String())
	}

	edited.Steps[0].ProvenanceClass = executionenv.SnapshotProvenanceUserDeclared
	edited.Steps = append(edited.Steps, executionenv.AssistedRebuildStep{
		Step: 3, Action: "Verify projector output manually", Status: "MANUAL",
		ProvenanceClass: executionenv.SnapshotProvenanceUserDeclared,
	})
	updateBody, _ := json.Marshal(executionEnvironmentRebuildPlanUpdateRequest{
		SourceSnapshotID: snapshot.ID,
		Plan:             edited,
	})
	updateReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodPut, path, updateBody,
	)
	updateRes := httptest.NewRecorder()
	handler.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateRes.Code, updateRes.Body.String())
	}
	var updated executionEnvironmentRebuildPlanView
	if err := json.Unmarshal(updateRes.Body.Bytes(), &updated); err != nil { t.Fatal(err) }
	if updated.RebuildPlanID != seeded.RebuildPlanID ||
		len(updated.Plan.Steps) != 3 ||
		updated.Plan.Steps[0].ProvenanceClass != executionenv.SnapshotProvenanceUserDeclared {
		t.Fatalf("updated=%+v", updated)
	}

	deleteNoConfirmReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodDelete, path, nil,
	)
	deleteNoConfirmRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteNoConfirmRes, deleteNoConfirmReq)
	if deleteNoConfirmRes.Code != http.StatusBadRequest {
		t.Fatalf("delete without confirm status=%d body=%s", deleteNoConfirmRes.Code, deleteNoConfirmRes.Body.String())
	}

	deleteReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodDelete, path+"?confirm=true", nil,
	)
	deleteRes := httptest.NewRecorder()
	handler.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRes.Code, deleteRes.Body.String())
	}

	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	seedAfterFreezeReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodPost, path+"/seed", seedBody,
	)
	seedAfterFreezeRes := httptest.NewRecorder()
	handler.ServeHTTP(seedAfterFreezeRes, seedAfterFreezeReq)
	if seedAfterFreezeRes.Code != http.StatusConflict {
		t.Fatalf("seed frozen status=%d body=%s", seedAfterFreezeRes.Code, seedAfterFreezeRes.Body.String())
	}
	var frozenError map[string]any
	if err := json.Unmarshal(seedAfterFreezeRes.Body.Bytes(), &frozenError); err != nil { t.Fatal(err) }
	if frozenError["error_code"] != "REVISION_NOT_DRAFT" {
		t.Fatalf("seed frozen error=%v", frozenError)
	}
}
