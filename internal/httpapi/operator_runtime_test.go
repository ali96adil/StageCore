package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestAuthenticatedOperatorRuntimeRehearsalGoJumpAndStop(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(projectStore, registry)
	handler := New(WithOperatorRuntime(h.auth, projectStore, runtime)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Runtime Web", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectStore.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	first, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Second", OrderIndex: 2,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(projectStore).Create(ctx, revision.ID, owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}

	initialReq := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/runtime", owner.Token)
	initialRes := httptest.NewRecorder()
	handler.ServeHTTP(initialRes, initialReq)
	if initialRes.Code != http.StatusOK {
		t.Fatalf("initial runtime status=%d body=%s", initialRes.Code, initialRes.Body.String())
	}
	var initial runtimeStatusView
	if err := json.Unmarshal(initialRes.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Mode != "EDIT" || initial.Snapshot == nil || initial.Snapshot.ID != published.ID || initial.Session != nil {
		t.Fatalf("unexpected initial runtime status: %+v", initial)
	}

	startPayload, _ := json.Marshal(runtimeStartRequest{Mode: "REHEARSAL", Name: "Web Rehearsal", RequestID: "00000000-0000-7000-8000-000000000401"})
	startReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/start", startPayload, owner.Token, owner.CSRFToken)
	startRes := httptest.NewRecorder()
	handler.ServeHTTP(startRes, startReq)
	if startRes.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", startRes.Code, startRes.Body.String())
	}
	var start struct {
		Result  contracts.CommandResult `json:"result"`
		Session sessionView             `json:"session"`
	}
	if err := json.Unmarshal(startRes.Body.Bytes(), &start); err != nil {
		t.Fatal(err)
	}
	if start.Result.Status != contracts.CommandCompleted || start.Session.ID == "" {
		t.Fatalf("unexpected start response: %+v", start)
	}

	goPayload, _ := json.Marshal(runtimeCommandRequest{RequestID: "00000000-0000-7000-8000-000000000402"})
	goReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/go", goPayload, owner.Token, owner.CSRFToken)
	goRes := httptest.NewRecorder()
	handler.ServeHTTP(goRes, goReq)
	if goRes.Code != http.StatusOK {
		t.Fatalf("GO status=%d body=%s", goRes.Code, goRes.Body.String())
	}
	var goResponse struct {
		Result contracts.CommandResult `json:"result"`
	}
	if err := json.Unmarshal(goRes.Body.Bytes(), &goResponse); err != nil {
		t.Fatal(err)
	}
	if goResponse.Result.Status != contracts.CommandCompleted {
		t.Fatalf("GO result=%+v", goResponse.Result)
	}

	statusReq := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/runtime", owner.Token)
	statusRes := httptest.NewRecorder()
	handler.ServeHTTP(statusRes, statusReq)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", statusRes.Code, statusRes.Body.String())
	}
	var status runtimeStatusView
	if err := json.Unmarshal(statusRes.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Mode != "REHEARSAL" || status.Session == nil || status.CurrentCue == nil || status.CurrentCue.ID != first.ID {
		t.Fatalf("unexpected running status: %+v", status)
	}
	if status.NextCue == nil || status.NextCue.ID != second.ID || status.LatestExecution == nil || status.LatestExecution.Result != domain.ExecutionCompleted {
		t.Fatalf("runtime did not expose next/latest result: %+v", status)
	}

	unconfirmedPayload, _ := json.Marshal(runtimeJumpRequest{
		RequestID: "00000000-0000-7000-8000-000000000403", CueID: second.ID, Confirm: false,
	})
	unconfirmedReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/jump", unconfirmedPayload, owner.Token, owner.CSRFToken)
	unconfirmedRes := httptest.NewRecorder()
	handler.ServeHTTP(unconfirmedRes, unconfirmedReq)
	if unconfirmedRes.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed Jump status=%d, want 400", unconfirmedRes.Code)
	}

	expected := first.ID
	jumpPayload, _ := json.Marshal(runtimeJumpRequest{
		RequestID: "00000000-0000-7000-8000-000000000404", CueID: second.ID,
		ExpectedCurrentCueID: &expected, Confirm: true,
	})
	jumpReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/jump", jumpPayload, owner.Token, owner.CSRFToken)
	jumpRes := httptest.NewRecorder()
	handler.ServeHTTP(jumpRes, jumpReq)
	if jumpRes.Code != http.StatusOK {
		t.Fatalf("Jump status=%d body=%s", jumpRes.Code, jumpRes.Body.String())
	}
	var jumpResponse struct {
		Result contracts.CommandResult `json:"result"`
	}
	if err := json.Unmarshal(jumpRes.Body.Bytes(), &jumpResponse); err != nil {
		t.Fatal(err)
	}
	if jumpResponse.Result.Status != contracts.CommandCompleted {
		t.Fatalf("Jump result=%+v", jumpResponse.Result)
	}
	active, err := projectStore.ActiveSessionForProject(ctx, project.ID)
	if err != nil || active == nil || active.CurrentCueID == nil || *active.CurrentCueID != second.ID {
		t.Fatalf("Jump did not move current Cue: active=%+v err=%v", active, err)
	}

	noRunStopPayload, _ := json.Marshal(runtimeCommandRequest{RequestID: "00000000-0000-7000-8000-000000000405"})
	noRunStopReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/stop", noRunStopPayload, owner.Token, owner.CSRFToken)
	noRunStopRes := httptest.NewRecorder()
	handler.ServeHTTP(noRunStopRes, noRunStopReq)
	if noRunStopRes.Code != http.StatusConflict {
		t.Fatalf("STOP without running Cue status=%d body=%s", noRunStopRes.Code, noRunStopRes.Body.String())
	}

	stopSessionPayload, _ := json.Marshal(runtimeCommandRequest{RequestID: "00000000-0000-7000-8000-000000000406"})
	stopSessionReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/stop-session", stopSessionPayload, owner.Token, owner.CSRFToken)
	stopSessionRes := httptest.NewRecorder()
	handler.ServeHTTP(stopSessionRes, stopSessionReq)
	if stopSessionRes.Code != http.StatusOK {
		t.Fatalf("stop Session status=%d body=%s", stopSessionRes.Code, stopSessionRes.Body.String())
	}

	showPayload, _ := json.Marshal(runtimeStartRequest{Mode: "SHOW", Name: "Blocked Show", RequestID: "00000000-0000-7000-8000-000000000407"})
	showReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/start", showPayload, owner.Token, owner.CSRFToken)
	showRes := httptest.NewRecorder()
	handler.ServeHTTP(showRes, showReq)
	if showRes.Code != http.StatusConflict {
		t.Fatalf("SHOW before S3 status=%d body=%s", showRes.Code, showRes.Body.String())
	}
	var showResponse struct {
		Result contracts.CommandResult `json:"result"`
	}
	if err := json.Unmarshal(showRes.Body.Bytes(), &showResponse); err != nil {
		t.Fatal(err)
	}
	if showResponse.Result.Error == nil || showResponse.Result.Error.ErrorCode != "SHOW_PREFLIGHT_REQUIRED" {
		t.Fatalf("unexpected SHOW gate result: %+v", showResponse.Result)
	}
}
