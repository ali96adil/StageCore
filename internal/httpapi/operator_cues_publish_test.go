package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorCuePublishCreatesImmutableSnapshotAndNextEditForksDraft(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{Result: domain.ExecutionCompleted}
	})); err != nil {
		t.Fatal(err)
	}
	publisher := publish.New(projectStore, registry)
	handler := New(
		WithOperatorProjects(h.auth, projectStore),
		WithOperatorCuePublish(h.auth, projectStore, publisher),
	).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, originalRevision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Publish Test", CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC",
	}); err != nil {
		t.Fatal(err)
	}

	createBody := cueWriteRequest{
		DisplayLabel: "1", Name: "Opening", OrderIndex: 1, CueType: "STANDARD",
		Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		Actions: []actionWriteRequest{{
			OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM",
			CapabilityKey: "sim.test", Parameters: json.RawMessage(`{"value":"first"}`),
			TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: "P1", Enabled: true,
		}},
	}
	createPayload, _ := json.Marshal(createBody)
	createReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/cues", createPayload, owner.Token, owner.CSRFToken)
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create cue status=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		Revision revisionView `json:"revision"`
		Cue      cueView      `json:"cue"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Revision.ID != originalRevision.ID || created.Cue.Name != "Opening" {
		t.Fatalf("unexpected created cue payload: %+v %+v", created.Revision, created.Cue)
	}

	validationReq := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/validation", owner.Token)
	validationRes := httptest.NewRecorder()
	handler.ServeHTTP(validationRes, validationReq)
	if validationRes.Code != http.StatusOK {
		t.Fatalf("validation status=%d body=%s", validationRes.Code, validationRes.Body.String())
	}
	var validation publish.Report
	if err := json.Unmarshal(validationRes.Body.Bytes(), &validation); err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || len(validation.Findings) != 0 {
		t.Fatalf("validation should pass: %+v", validation)
	}

	publishReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/publish", nil, owner.Token, owner.CSRFToken)
	publishRes := httptest.NewRecorder()
	handler.ServeHTTP(publishRes, publishReq)
	if publishRes.Code != http.StatusCreated {
		t.Fatalf("publish status=%d body=%s", publishRes.Code, publishRes.Body.String())
	}
	var published struct {
		RuntimeSnapshot snapshotView   `json:"runtime_snapshot"`
		Validation      publish.Report `json:"validation"`
	}
	if err := json.Unmarshal(publishRes.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if published.RuntimeSnapshot.ID == "" || published.RuntimeSnapshot.RevisionID != originalRevision.ID || !published.Validation.Valid {
		t.Fatalf("unexpected publish payload: %+v", published)
	}
	storedBeforeEdit, err := projectStore.GetRuntimeSnapshot(ctx, published.RuntimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	manifestBeforeEdit := append([]byte(nil), storedBeforeEdit.Manifest...)
	hashBeforeEdit := storedBeforeEdit.ContentHash
	validatedRevision, err := projectStore.GetRevision(ctx, originalRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if validatedRevision.Status != domain.RevisionValidated {
		t.Fatalf("published revision status=%s, want VALIDATED", validatedRevision.Status)
	}

	editBody := createBody
	editBody.Name = "Opening Revised"
	editBody.Actions[0].Parameters = json.RawMessage(`{"value":"second"}`)
	editPayload, _ := json.Marshal(editBody)
	editReq := authenticatedMutationRequest(t, http.MethodPut, "/api/v1/projects/"+project.ID+"/cues/"+created.Cue.ID, editPayload, owner.Token, owner.CSRFToken)
	editRes := httptest.NewRecorder()
	handler.ServeHTTP(editRes, editReq)
	if editRes.Code != http.StatusOK {
		t.Fatalf("edit after publish status=%d body=%s", editRes.Code, editRes.Body.String())
	}
	var edited struct {
		Revision revisionView `json:"revision"`
		Cue      cueView      `json:"cue"`
	}
	if err := json.Unmarshal(editRes.Body.Bytes(), &edited); err != nil {
		t.Fatal(err)
	}
	if edited.Revision.ID == originalRevision.ID || edited.Revision.Status != domain.RevisionDraft {
		t.Fatalf("edit did not fork new Draft: %+v", edited.Revision)
	}
	if edited.Cue.ID == created.Cue.ID || edited.Cue.RevisionID != edited.Revision.ID || edited.Cue.Name != "Opening Revised" {
		t.Fatalf("unexpected forked cue: %+v", edited.Cue)
	}

	storedAfterEdit, err := projectStore.GetRuntimeSnapshot(ctx, published.RuntimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedAfterEdit.ContentHash != hashBeforeEdit || !bytes.Equal(storedAfterEdit.Manifest, manifestBeforeEdit) {
		t.Fatal("published Runtime Snapshot changed after editing successor Draft")
	}
	oldCue, err := projectStore.GetCue(ctx, created.Cue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if oldCue.Name != "Opening" || oldCue.RevisionID != originalRevision.ID {
		t.Fatalf("published source cue was mutated: %+v", oldCue)
	}
	currentProject, err := projectStore.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if currentProject.CurrentRevisionID != edited.Revision.ID {
		t.Fatalf("current revision=%s, want successor Draft %s", currentProject.CurrentRevisionID, edited.Revision.ID)
	}

	dashboardReq := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/dashboard", owner.Token)
	dashboardRes := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRes, dashboardReq)
	if dashboardRes.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", dashboardRes.Code, dashboardRes.Body.String())
	}
	var dashboard dashboardView
	if err := json.Unmarshal(dashboardRes.Body.Bytes(), &dashboard); err != nil {
		t.Fatal(err)
	}
	if !dashboard.UnpublishedChanges || dashboard.PublishedSnapshot == nil || dashboard.PublishedSnapshot.ID != published.RuntimeSnapshot.ID {
		t.Fatalf("dashboard did not distinguish published Snapshot from successor Draft: %+v", dashboard)
	}
}

func TestOperatorPublishValidationBlockLeavesDraftEditable(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	registry := capability.NewRegistry()
	publisher := publish.New(projectStore, registry)
	handler := New(WithOperatorCuePublish(h.auth, projectStore, publisher)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Blocked Publish", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}

	body := cueWriteRequest{
		DisplayLabel: "1", Name: "Broken Target", OrderIndex: 1, CueType: "STANDARD",
		Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		Actions: []actionWriteRequest{{
			OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "MISSING",
			CapabilityKey: "sim.test", Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`),
			ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true,
		}},
	}
	payload, _ := json.Marshal(body)
	createReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/cues", payload, owner.Token, owner.CSRFToken)
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create invalid cue status=%d body=%s", createRes.Code, createRes.Body.String())
	}

	publishReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/publish", nil, owner.Token, owner.CSRFToken)
	publishRes := httptest.NewRecorder()
	handler.ServeHTTP(publishRes, publishReq)
	if publishRes.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blocked publish status=%d body=%s", publishRes.Code, publishRes.Body.String())
	}
	var blocked struct {
		ErrorCode  string         `json:"error_code"`
		Validation publish.Report `json:"validation"`
	}
	if err := json.Unmarshal(publishRes.Body.Bytes(), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.ErrorCode != "VALIDATION_BLOCKED" || blocked.Validation.Valid || len(blocked.Validation.Findings) == 0 {
		t.Fatalf("unexpected blocked validation: %+v", blocked)
	}
	stillDraft, err := projectStore.GetRevision(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillDraft.Status != domain.RevisionDraft {
		t.Fatalf("blocked publish froze revision as %s", stillDraft.Status)
	}
	latest, err := projectStore.LatestPublishedRuntimeSnapshotForProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest != nil {
		t.Fatalf("blocked publish created snapshot: %+v", latest)
	}
}

func authenticatedMutationRequest(t *testing.T, method, path string, body []byte, token, csrf string) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "127.0.0.1:18000"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
	req.Header.Set(csrfHeader, csrf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func authenticatedReadRequest(method, path, token string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:18001"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
	return req
}
