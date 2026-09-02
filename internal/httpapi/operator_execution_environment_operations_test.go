package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

type fakeExecutionEnvironmentOperationRuntime struct {
	calls    int
	request  companionchannel.EnvironmentOperationRequest
	status   companionchannel.EnvironmentOperationStatus
	errorCode string
	summary  string
	snapshot *executionenv.Snapshot
}

func (f *fakeExecutionEnvironmentOperationRuntime) OperateExecutionEnvironment(_ context.Context, request companionchannel.EnvironmentOperationRequest) companionchannel.EnvironmentOperationResult {
	f.calls++
	f.request = request
	return companionchannel.EnvironmentOperationResult{
		OperationID:     request.OperationID,
		Kind:            request.Kind,
		Status:          f.status,
		ErrorCode:       f.errorCode,
		ResponseSummary: f.summary,
		Snapshot:        f.snapshot,
	}
}

func TestOperatorExecutionEnvironmentOperationIsRuntimeControlledAndBounded(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	runtime := &fakeExecutionEnvironmentOperationRuntime{
		status:  companionchannel.EnvironmentOperationCompleted,
		summary: "VDMX opened the declared execution-environment launch target",
	}
	handler := New(WithOperatorExecutionEnvironmentOperations(h.auth, stageStore, runtime)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Execution Environment Operations", CreatedBy: owner.Session.User.ID,
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
	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/operations"
	validBody := []byte(`{"operation_id":"operation-open-1","kind":"OPEN","timeout_ms":5000}`)

	unauthenticated := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(validBody)))
	unauthenticated.RemoteAddr = "127.0.0.1:15000"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized || runtime.calls != 0 {
		t.Fatalf("unauthenticated status=%d calls=%d body=%s", unauthenticatedRes.Code, runtime.calls, unauthenticatedRes.Body.String())
	}

	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000209', 'viewer-operations', ?, 'VIEWER', 1, 1, 1)
	`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "viewer-operations", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerReq := authenticatedExecutionEnvironmentRequest(t, viewer.Token, viewer.CSRFToken, http.MethodPost, path, validBody)
	viewerRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden || runtime.calls != 0 {
		t.Fatalf("VIEWER status=%d calls=%d body=%s", viewerRes.Code, runtime.calls, viewerRes.Body.String())
	}

	injectionBody := []byte(`{"operation_id":"operation-injection","kind":"OPEN","timeout_ms":5000,"capability":"local.echo"}`)
	injectionReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, path, injectionBody)
	injectionRes := httptest.NewRecorder()
	handler.ServeHTTP(injectionRes, injectionReq)
	if injectionRes.Code != http.StatusBadRequest || runtime.calls != 0 || !strings.Contains(injectionRes.Body.String(), `"INVALID_REQUEST"`) {
		t.Fatalf("injection status=%d calls=%d body=%s", injectionRes.Code, runtime.calls, injectionRes.Body.String())
	}

	invalidKindReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, path, []byte(`{"operation_id":"operation-kind","kind":"SHELL","timeout_ms":5000}`))
	invalidKindRes := httptest.NewRecorder()
	handler.ServeHTTP(invalidKindRes, invalidKindReq)
	if invalidKindRes.Code != http.StatusBadRequest || runtime.calls != 0 {
		t.Fatalf("invalid kind status=%d calls=%d body=%s", invalidKindRes.Code, runtime.calls, invalidKindRes.Body.String())
	}

	invalidTimeoutReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, path, []byte(`{"operation_id":"operation-timeout","kind":"OPEN","timeout_ms":30001}`))
	invalidTimeoutRes := httptest.NewRecorder()
	handler.ServeHTTP(invalidTimeoutRes, invalidTimeoutReq)
	if invalidTimeoutRes.Code != http.StatusBadRequest || runtime.calls != 0 {
		t.Fatalf("invalid timeout status=%d calls=%d body=%s", invalidTimeoutRes.Code, runtime.calls, invalidTimeoutRes.Body.String())
	}

	validReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, path, validBody)
	validRes := httptest.NewRecorder()
	handler.ServeHTTP(validRes, validReq)
	if validRes.Code != http.StatusOK {
		t.Fatalf("valid operation status=%d body=%s", validRes.Code, validRes.Body.String())
	}
	if runtime.calls != 1 || runtime.request.OperationID != "operation-open-1" || runtime.request.EnvironmentManifestID != environment.ID || runtime.request.Kind != companionchannel.EnvironmentOperationOpen || runtime.request.TimeoutMS != 5000 {
		t.Fatalf("runtime calls=%d request=%+v", runtime.calls, runtime.request)
	}
	var response executionEnvironmentOperationView
	if err := json.Unmarshal(validRes.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != companionchannel.EnvironmentOperationCompleted || response.Kind != companionchannel.EnvironmentOperationOpen || response.ResponseSummary == "" {
		t.Fatalf("operation response=%+v", response)
	}
}

func TestOperatorExecutionEnvironmentOperationScopesEnvironmentAndReturnsSnapshot(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Environment Snapshot Operation", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := stageStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, testVDMXExecutionEnvironmentManifest("video-main"), owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherProject, otherRevision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Other Revision", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	_ = otherProject

	snapshot := &executionenv.Snapshot{
		SchemaVersion:        executionenv.SnapshotSchemaVersion,
		EnvironmentKey:       environment.Manifest.EnvironmentKey,
		AdapterKey:           environment.Manifest.AdapterKey,
		SourceManifestSHA256: environment.ContentSHA256,
		CaptureStatus:        executionenv.SnapshotPartial,
		Notes:                "truthful partial snapshot",
	}
	runtime := &fakeExecutionEnvironmentOperationRuntime{
		status:   companionchannel.EnvironmentOperationCompleted,
		summary:  "VDMX partial execution-environment snapshot captured",
		snapshot: snapshot,
	}
	handler := New(WithOperatorExecutionEnvironmentOperations(h.auth, stageStore, runtime)).Handler()

	wrongPath := "/api/v1/projects/" + project.ID + "/revisions/" + otherRevision.ID + "/execution-environments/" + environment.ID + "/operations"
	wrongReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, wrongPath, []byte(`{"operation_id":"capture-wrong","kind":"CAPTURE_SNAPSHOT","timeout_ms":5000}`))
	wrongRes := httptest.NewRecorder()
	handler.ServeHTTP(wrongRes, wrongReq)
	if wrongRes.Code != http.StatusNotFound || runtime.calls != 0 {
		t.Fatalf("cross-revision status=%d calls=%d body=%s", wrongRes.Code, runtime.calls, wrongRes.Body.String())
	}

	path := "/api/v1/projects/" + project.ID + "/revisions/" + revision.ID + "/execution-environments/" + environment.ID + "/operations"
	captureReq := authenticatedExecutionEnvironmentRequest(t, owner.Token, owner.CSRFToken, http.MethodPost, path, []byte(`{"operation_id":"capture-1","kind":"CAPTURE_SNAPSHOT","timeout_ms":5000}`))
	captureRes := httptest.NewRecorder()
	handler.ServeHTTP(captureRes, captureReq)
	if captureRes.Code != http.StatusOK {
		t.Fatalf("capture status=%d body=%s", captureRes.Code, captureRes.Body.String())
	}
	var payload executionEnvironmentOperationView
	if err := json.Unmarshal(captureRes.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 1 || payload.Snapshot == nil || payload.Snapshot.CaptureStatus != executionenv.SnapshotPartial || payload.Snapshot.EnvironmentKey != environment.Manifest.EnvironmentKey {
		t.Fatalf("capture calls=%d payload=%+v", runtime.calls, payload)
	}
}
