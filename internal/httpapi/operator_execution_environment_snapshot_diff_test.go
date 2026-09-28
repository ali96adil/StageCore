package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorExecutionEnvironmentSnapshotDiffIsReadOnlyAndScoped(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorExecutionEnvironments(h.auth, stageStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Snapshot diff API", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("video-main"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	makeSnapshot := func(value float64) executionenv.Snapshot {
		metadata, err := json.Marshal(map[string]any{
			"path": "/fader",
			"value": value,
		})
		if err != nil {
			t.Fatal(err)
		}
		return executionenv.Snapshot{
			SchemaVersion:        executionenv.SnapshotSchemaVersion,
			EnvironmentKey:       environment.Manifest.EnvironmentKey,
			AdapterKey:           environment.Manifest.AdapterKey,
			SourceManifestSHA256: environment.ContentSHA256,
			CaptureStatus:        executionenv.SnapshotPartial,
			Items: []executionenv.SnapshotItem{{
				Key:             "vdmx-oscquery",
				Name:            "VDMX OSCQuery",
				Kind:            executionenv.SnapshotControlState,
				Provenance:      executionenv.ProvenanceOSCQuery,
				ProvenanceClass: executionenv.SnapshotProvenanceObserved,
				Capture:         executionenv.ItemObserved,
				Portability:     executionenv.SnapshotDescriptiveOnly,
				Metadata:        metadata,
			}},
		}
	}

	before, err := stageStore.CreateExecutionEnvironmentSnapshot(
		ctx, environment.ID, makeSnapshot(0.5), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	after, err := stageStore.CreateExecutionEnvironmentSnapshot(
		ctx, environment.ID, makeSnapshot(0.75), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	listPath := "/api/v1/projects/" + project.ID +
		"/revisions/" + revision.ID +
		"/execution-environments/" + environment.ID +
		"/snapshots"
	listReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodGet, listPath, nil,
	)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("snapshot list status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	var listed struct {
		ExecutionEnvironmentID string                                    `json:"execution_environment_id"`
		Snapshots              []executionEnvironmentSnapshotSummaryView `json:"snapshots"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.ExecutionEnvironmentID != environment.ID || len(listed.Snapshots) != 2 {
		t.Fatalf("snapshot list=%+v", listed)
	}
	if listed.Snapshots[0].SnapshotID != before.ID ||
		listed.Snapshots[1].SnapshotID != after.ID ||
		listed.Snapshots[0].CaptureStatus != executionenv.SnapshotPartial ||
		listed.Snapshots[0].ContentSHA256 == "" {
		t.Fatalf("snapshot summaries=%+v", listed.Snapshots)
	}

	base := "/api/v1/projects/" + project.ID +
		"/revisions/" + revision.ID +
		"/execution-environments/" + environment.ID +
		"/snapshots/diff"

	unauthenticated := httptest.NewRequest(
		http.MethodGet,
		base+"?before_snapshot_id="+before.ID+"&after_snapshot_id="+after.ID,
		nil,
	)
	unauthenticated.RemoteAddr = "127.0.0.1:14010"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticatedRes.Code, unauthenticatedRes.Body.String())
	}

	missingIDs := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodGet, base, nil,
	)
	missingIDsRes := httptest.NewRecorder()
	handler.ServeHTTP(missingIDsRes, missingIDs)
	if missingIDsRes.Code != http.StatusBadRequest {
		t.Fatalf("missing IDs status=%d body=%s", missingIDsRes.Code, missingIDsRes.Body.String())
	}

	req := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodGet,
		base+"?before_snapshot_id="+before.ID+"&after_snapshot_id="+after.ID,
		nil,
	)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("diff status=%d body=%s", res.Code, res.Body.String())
	}
	var view executionEnvironmentSnapshotDiffView
	if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.ExecutionEnvironmentID != environment.ID ||
		view.BeforeSnapshotID != before.ID ||
		view.AfterSnapshotID != after.ID ||
		view.Diff.Identical {
		t.Fatalf("view=%+v", view)
	}
	want := []executionenv.SnapshotItemDiff{{
		Key: "vdmx-oscquery", ChangedFields: []string{"metadata"},
	}}
	if !reflect.DeepEqual(view.Diff.ChangedItems, want) {
		t.Fatalf("changed=%+v want=%+v", view.Diff.ChangedItems, want)
	}

	otherEnvironment, err := stageStore.CreateExecutionEnvironmentManifest(
		ctx, revision.ID, testVDMXExecutionEnvironmentManifest("video-secondary"), owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	otherSnapshot := makeSnapshot(0.9)
	otherSnapshot.EnvironmentKey = otherEnvironment.Manifest.EnvironmentKey
	otherSnapshot.SourceManifestSHA256 = otherEnvironment.ContentSHA256
	cross, err := stageStore.CreateExecutionEnvironmentSnapshot(
		ctx, otherEnvironment.ID, otherSnapshot, owner.Session.User.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	crossReq := authenticatedExecutionEnvironmentRequest(
		t, owner.Token, owner.CSRFToken, http.MethodGet,
		base+"?before_snapshot_id="+before.ID+"&after_snapshot_id="+cross.ID,
		nil,
	)
	crossRes := httptest.NewRecorder()
	handler.ServeHTTP(crossRes, crossReq)
	if crossRes.Code != http.StatusNotFound {
		t.Fatalf("cross-environment status=%d body=%s", crossRes.Code, crossRes.Body.String())
	}
}
