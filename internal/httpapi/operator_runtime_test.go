package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestRehearsalStartSuppressesResultAfterGrantRevocationAtCommit(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.RehearsalControl}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.h.db.DB.ExecContext(ctx, fmt.Sprintf(`
		CREATE TRIGGER revoke_rehearsal_grant_after_session_start
		AFTER INSERT ON sessions
		WHEN NEW.session_type = 'REHEARSAL' AND NEW.project_id = '%s'
		BEGIN
			DELETE FROM project_permission_grants
			WHERE user_id = '%s' AND project_id = NEW.project_id AND permission = 'rehearsal.control';
		END`, f.projectID, f.operatorID)); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(f.projects, capability.NewRegistry())
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+f.projectID+"/runtime/start",
		[]byte(`{"mode":"REHEARSAL","name":"Revoked at commit","request_id":"018f5b34-0f47-7cc0-9000-0a0000000199"}`),
		f.operator.Token, f.operator.CSRFToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), `"session"`) ||
		!strings.Contains(response.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(response.Body.String(), `"effect_may_have_been_committed":true`) ||
		!strings.Contains(response.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked operator received committed rehearsal result or retryable response: HTTP %d %s", response.Code, response.Body.String())
	}
	if err := f.access.Check(ctx, f.operatorID, f.projectID, projectaccess.ProjectView); err != nil {
		t.Fatalf("test must preserve project.view: %v", err)
	}
	active, err := f.projects.ActiveSessionForProject(ctx, f.projectID)
	if err != nil || active == nil || active.Type != domain.SessionRehearsal {
		t.Fatalf("committed rehearsal state was not retained truthfully: %+v err=%v", active, err)
	}
}

func TestAuthenticatedOperatorRuntimeRehearsalGoJumpAndStop(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	tabletStopSessionID := ""
	runtime := runtimecontrol.New(projectStore, registry,
		runtimecontrol.WithRehearsalTabletStop(func(_ context.Context, session domain.Session, _ contracts.CommandEnvelope) error {
			tabletStopSessionID = session.ID
			return nil
		}),
		runtimecontrol.WithEmergencySafety(func(_ context.Context, _ domain.Session, command contracts.CommandEnvelope, enabled bool) (json.RawMessage, error) {
			if command.Priority != "P0" {
				t.Fatalf("Emergency Blackout priority=%s", command.Priority)
			}
			return json.RawMessage(`{"enabled":` + map[bool]string{true: "true", false: "false"}[enabled] + `,"lighting":{"status":"NOT_CONFIGURED"},"tablets":{"status":"NOT_CONFIGURED"},"native_visual":{"status":"NOT_CONFIGURED"},"audio":{"status":"UNCHANGED_BY_DESIGN"},"external_adapters":{"status":"UNCHANGED_BY_DESIGN"}}`), nil
		}),
	)
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorRuntime(h.auth, projectStore, runtime), WithOperatorMemory(h.auth, projectStore, sessionmemory.New(projectStore)), WithProjectRouteQualification(access)).Handler()
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

	for _, route := range []string{"go", "jump"} {
		body := []byte(`{"request_id":"00000000-0000-7000-8000-000000000499","expected_session_id":"previous-session"}`)
		if route == "jump" {
			body = []byte(`{"request_id":"00000000-0000-7000-8000-000000000499","cue_id":"some-cue","confirm":true,"expected_session_id":"previous-session"}`)
		}
		req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/"+route, body, owner.Token, owner.CSRFToken)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "RUNTIME_SESSION_CHANGED") {
			t.Fatalf("stale %s returned %d: %s", route, res.Code, res.Body)
		}
	}
	if runs, err := projectStore.ListCueExecutions(ctx, start.Session.ID); err != nil || len(runs) != 0 {
		t.Fatalf("stale commands dispatched cues: %+v, %v", runs, err)
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

	badEmergencyBody, _ := json.Marshal(runtimeEmergencyBlackoutRequest{
		RequestID: "00000000-0000-7000-8000-000000000408", Enabled: true, Confirm: "",
	})
	badEmergencyReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/emergency-blackout", badEmergencyBody, owner.Token, owner.CSRFToken)
	badEmergencyRes := httptest.NewRecorder()
	handler.ServeHTTP(badEmergencyRes, badEmergencyReq)
	if badEmergencyRes.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed Emergency Blackout status=%d body=%s", badEmergencyRes.Code, badEmergencyRes.Body.String())
	}

	emergencyBody, _ := json.Marshal(runtimeEmergencyBlackoutRequest{
		RequestID: "00000000-0000-7000-8000-000000000409", Enabled: true, Confirm: "BLACKOUT",
	})
	emergencyReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/emergency-blackout", emergencyBody, owner.Token, owner.CSRFToken)
	emergencyRes := httptest.NewRecorder()
	handler.ServeHTTP(emergencyRes, emergencyReq)
	if emergencyRes.Code != http.StatusOK {
		t.Fatalf("Emergency Blackout status=%d body=%s", emergencyRes.Code, emergencyRes.Body.String())
	}

	blackoutStatusReq := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/runtime", owner.Token)
	blackoutStatusRes := httptest.NewRecorder()
	handler.ServeHTTP(blackoutStatusRes, blackoutStatusReq)
	var blackoutStatus runtimeStatusView
	if blackoutStatusRes.Code != http.StatusOK || json.Unmarshal(blackoutStatusRes.Body.Bytes(), &blackoutStatus) != nil || !blackoutStatus.ManagedOutputBlackout {
		t.Fatalf("Runtime did not expose active Emergency Blackout: %d %s", blackoutStatusRes.Code, blackoutStatusRes.Body.String())
	}

	blockedGoBody, _ := json.Marshal(runtimeCommandRequest{RequestID: "00000000-0000-7000-8000-000000000410"})
	blockedGoReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/go", blockedGoBody, owner.Token, owner.CSRFToken)
	blockedGoRes := httptest.NewRecorder()
	handler.ServeHTTP(blockedGoRes, blockedGoReq)
	if blockedGoRes.Code != http.StatusConflict || !strings.Contains(blockedGoRes.Body.String(), "EMERGENCY_BLACKOUT_ACTIVE") {
		t.Fatalf("GO during Emergency Blackout=%d %s", blockedGoRes.Code, blockedGoRes.Body.String())
	}

	clearBody, _ := json.Marshal(runtimeEmergencyBlackoutRequest{
		RequestID: "00000000-0000-7000-8000-000000000411", Enabled: false, Confirm: "CLEAR",
	})
	clearReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/emergency-blackout", clearBody, owner.Token, owner.CSRFToken)
	clearRes := httptest.NewRecorder()
	handler.ServeHTTP(clearRes, clearReq)
	if clearRes.Code != http.StatusOK {
		t.Fatalf("clear Emergency Blackout=%d %s", clearRes.Code, clearRes.Body.String())
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

	staleSession := "a-different-rehearsal"
	staleCheck, _ := json.Marshal(runtimeJumpRequest{
		RequestID: "00000000-0000-7000-8000-000000000413", CueID: second.ID,
		Confirm: true, RehearsalSessionID: &staleSession,
	})
	staleReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/jump", staleCheck, owner.Token, owner.CSRFToken)
	staleRes := httptest.NewRecorder()
	handler.ServeHTTP(staleRes, staleReq)
	if staleRes.Code != http.StatusConflict || !strings.Contains(staleRes.Body.String(), "CUE_CHECK_SESSION_CHANGED") {
		t.Fatalf("stale Cue Check status=%d body=%s", staleRes.Code, staleRes.Body.String())
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

	trace := httptest.NewRecorder()
	handler.ServeHTTP(trace, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/sessions/"+start.Session.ID, owner.Token))
	if trace.Code != http.StatusOK {
		t.Fatalf("trace status=%d body=%s", trace.Code, trace.Body)
	}
	var history struct {
		CueExecutions []struct {
			ID string `json:"cue_execution_id"`
		} `json:"cue_executions"`
	}
	if err := json.Unmarshal(trace.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	for _, result := range []contracts.CommandResult{goResponse.Result, jumpResponse.Result} {
		var receipt struct {
			ID string `json:"cue_execution_id"`
		}
		if err := json.Unmarshal(result.Payload, &receipt); err != nil || receipt.ID == "" {
			t.Fatalf("missing execution in receipt: %s err=%v", result.Payload, err)
		}
		count := 0
		for _, execution := range history.CueExecutions {
			if execution.ID == receipt.ID {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("receipt %s resolved to %d persisted traces", receipt.ID, count)
		}
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

	badForcePayload, _ := json.Marshal(runtimeCommandRequest{
		RequestID: "00000000-0000-7000-8000-000000000412",
		Force:     true,
	})
	badForceReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/stop-session", badForcePayload, owner.Token, owner.CSRFToken)
	badForceRes := httptest.NewRecorder()
	handler.ServeHTTP(badForceRes, badForceReq)
	if badForceRes.Code != http.StatusBadRequest || !strings.Contains(badForceRes.Body.String(), "CONFIRMATION_REQUIRED") {
		t.Fatalf("unconfirmed Force Exit status=%d body=%s", badForceRes.Code, badForceRes.Body.String())
	}
	activeAfterBadForce, err := projectStore.ActiveSessionForProject(ctx, project.ID)
	if err != nil || activeAfterBadForce == nil || activeAfterBadForce.ID != start.Session.ID {
		t.Fatalf("unconfirmed Force Exit changed active Session: active=%+v err=%v", activeAfterBadForce, err)
	}

	stopSessionPayload, _ := json.Marshal(runtimeStopSessionRequest{
		runtimeCommandRequest:        runtimeCommandRequest{RequestID: "00000000-0000-7000-8000-000000000406"},
		RehearsalTabletStopSessionID: start.Session.ID,
	})
	stopSessionReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/stop-session", stopSessionPayload, owner.Token, owner.CSRFToken)
	stopSessionRes := httptest.NewRecorder()
	handler.ServeHTTP(stopSessionRes, stopSessionReq)
	if stopSessionRes.Code != http.StatusOK || tabletStopSessionID != start.Session.ID {
		t.Fatalf("stop Session status=%d body=%s", stopSessionRes.Code, stopSessionRes.Body.String())
	}

	badEditBlackoutBody, _ := json.Marshal(runtimeEmergencyBlackoutRequest{
		RequestID: "00000000-0000-7000-8000-000000000413", Enabled: true, Confirm: "",
	})
	badEditBlackoutReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/project-blackout", badEditBlackoutBody, owner.Token, owner.CSRFToken)
	badEditBlackoutRes := httptest.NewRecorder()
	handler.ServeHTTP(badEditBlackoutRes, badEditBlackoutReq)
	if badEditBlackoutRes.Code != http.StatusBadRequest || !strings.Contains(badEditBlackoutRes.Body.String(), "CONFIRMATION_REQUIRED") {
		t.Fatalf("unconfirmed EDIT Blackout status=%d body=%s", badEditBlackoutRes.Code, badEditBlackoutRes.Body.String())
	}

	editBlackoutBody, _ := json.Marshal(runtimeEmergencyBlackoutRequest{
		RequestID: "00000000-0000-7000-8000-000000000414", Enabled: true, Confirm: "BLACKOUT",
	})
	editBlackoutReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/project-blackout", editBlackoutBody, owner.Token, owner.CSRFToken)
	editBlackoutRes := httptest.NewRecorder()
	handler.ServeHTTP(editBlackoutRes, editBlackoutReq)
	if editBlackoutRes.Code != http.StatusOK || !strings.Contains(editBlackoutRes.Body.String(), "\"sessionless\":true") {
		t.Fatalf("EDIT Blackout status=%d body=%s", editBlackoutRes.Code, editBlackoutRes.Body.String())
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

func TestShowAdmissionRechecksGrantAfterPreflight(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	grants := []projectaccess.Permission{projectaccess.ProjectView, projectaccess.ShowEnterExit}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID, grants); err != nil {
		t.Fatal(err)
	}
	gateReached := false
	runtime := runtimecontrol.New(f.projects, capability.NewRegistry(), runtimecontrol.WithShowGate(
		func(_ context.Context, projectID, _ string) (bool, string, error) {
			gateReached = true
			return true, "", f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, projectID, nil)
		},
	))
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+f.projectID+"/runtime/start",
		[]byte(`{"mode":"SHOW","request_id":"018f5b34-0f47-7cc0-9000-0a0000000109"}`), f.operator.Token, f.operator.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !gateReached || res.Code != http.StatusNotFound {
		t.Fatalf("SHOW after grant revocation returned %d (gate=%t): %s", res.Code, gateReached, res.Body)
	}
	active, err := f.projects.ActiveSessionForProject(ctx, f.projectID)
	if err != nil || active != nil {
		t.Fatalf("revoked SHOW changed runtime state: %+v, %v", active, err)
	}
}

func TestQualifiedDelayedCueStopsDispatchAfterShowGrantRevocation(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.projects.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.projects.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: project.CurrentRevisionID, DisplayLabel: "1", Name: "Revocable cue",
		OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		ExecutionPolicy: json.RawMessage(`{"action_delay_ms":25}`),
	}, []domain.Action{
		{
			OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.revoke",
			Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		},
		{
			OrderIndex: 1, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.after-revoke",
			Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
			PriorityClass: domain.PriorityP1, Enabled: true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.RehearsalControl}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.projects.StartOperationalSession(ctx, published.ID, domain.SessionRehearsal, "revocation rehearsal", ""); err != nil {
		t.Fatal(err)
	}
	var sentBeforeRevocation, sentAfterRevocation atomic.Int32
	registry := capability.NewRegistry()
	if err := registry.Register("sim.revoke", capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		sentBeforeRevocation.Add(1)
		if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, project.ID,
			[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
			t.Errorf("revoke SHOW GO grant: %v", err)
		}
		// This simulated action has already crossed its adapter boundary. The Hub
		// cannot retract it; the following delayed action must remain unsent.
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
	})); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("sim.after-revoke", capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		sentAfterRevocation.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
	})); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(f.projects, registry)
	handler := New(
		WithScopedOperatorProjects(f.h.auth, f.projects, f.access),
		WithOperatorRuntime(f.h.auth, f.projects, runtime),
		WithProjectRouteQualification(f.access),
	).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/go",
		[]byte(`{"request_id":"018f5b34-0f47-7cc0-9000-0a0000000125"}`), f.operator.Token, f.operator.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if sentBeforeRevocation.Load() != 1 || sentAfterRevocation.Load() != 0 {
		t.Fatalf("dispatch counts before/after grant revocation = %d/%d, want 1/0; response=%d %s",
			sentBeforeRevocation.Load(), sentAfterRevocation.Load(), res.Code, res.Body.String())
	}
	if err := f.access.Check(ctx, f.operator.Session.User.ID, project.ID, projectaccess.ProjectView); err != nil {
		t.Fatalf("test must retain project.view while revoking runtime control: %v", err)
	}
	if res.Code != http.StatusNotFound || strings.Contains(res.Body.String(), "cue_execution_id") ||
		!strings.Contains(res.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(res.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked runtime intent received cue result or retryable response: HTTP %d %s", res.Code, res.Body.String())
	}
}

func TestShowExitRechecksGrantAfterStopSafety(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := f.projects.StartOperationalSession(ctx, published.ID, domain.SessionShow, "SHOW", "")
	if err != nil {
		t.Fatal(err)
	}
	grants := []projectaccess.Permission{projectaccess.ProjectView, projectaccess.ShowEnterExit}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, f.projectID, grants); err != nil {
		t.Fatal(err)
	}
	safetyReached := false
	runtime := runtimecontrol.New(f.projects, capability.NewRegistry(), runtimecontrol.WithSessionStopSafety(
		func(_ context.Context, stopped domain.Session, _ contracts.CommandEnvelope) error {
			safetyReached = true
			return f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, stopped.ProjectID, nil)
		},
	))
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+f.projectID+"/runtime/stop-session",
		[]byte(`{"request_id":"018f5b34-0f47-7cc0-9000-0a0000000112"}`), f.operator.Token, f.operator.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !safetyReached || res.Code != http.StatusForbidden ||
		!strings.Contains(res.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(res.Body.String(), `"retryable":false`) {
		t.Fatalf("SHOW exit after grant revocation returned %d (safety=%t): %s", res.Code, safetyReached, res.Body)
	}
	current, err := f.projects.ActiveSessionForProject(ctx, f.projectID)
	if err != nil || current == nil || current.ID != active.ID {
		t.Fatalf("revoked SHOW exit changed active Session: %+v, %v", current, err)
	}
}

func TestShowExitSuppressesResultAfterSessionRevocationDuringSafety(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := f.projects.StartOperationalSession(ctx, published.ID, domain.SessionShow, "SHOW", "")
	if err != nil {
		t.Fatal(err)
	}
	safetyReached := false
	runtime := runtimecontrol.New(f.projects, capability.NewRegistry(), runtimecontrol.WithSessionStopSafety(
		func(_ context.Context, _ domain.Session, _ contracts.CommandEnvelope) error {
			safetyReached = true
			return f.h.auth.Logout(ctx, f.owner.Token)
		},
	))
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+f.projectID+"/runtime/stop-session",
		[]byte(`{"request_id":"018f5b34-0f47-7cc0-9000-0a0000000127"}`), f.owner.Token, f.owner.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !safetyReached || res.Code != http.StatusForbidden || strings.Contains(res.Body.String(), "show.exited") ||
		strings.Contains(res.Body.String(), `"session_id"`) ||
		!strings.Contains(res.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(res.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked Owner received committed SHOW-exit result or retryable response: HTTP %d %s", res.Code, res.Body.String())
	}
	current, err := f.projects.GetSession(ctx, active.ID)
	if err != nil || current.Status != domain.SessionActive {
		t.Fatalf("SHOW exit must not commit after authority is revoked before its transaction: %+v err=%v", current, err)
	}
}

func TestEmergencyBlackoutClearKeepsLatchAfterBrowserRevocation(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := f.projects.StartOperationalSession(ctx, published.ID, domain.SessionShow, "SHOW", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetSessionManagedOutputBlackout(ctx, active.ID, true, f.owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	safetyReached := false
	runtime := runtimecontrol.New(f.projects, capability.NewRegistry(), runtimecontrol.WithEmergencySafety(
		func(_ context.Context, _ domain.Session, _ contracts.CommandEnvelope, enabled bool) (json.RawMessage, error) {
			safetyReached = !enabled
			return json.RawMessage(`{}`), f.h.auth.Logout(ctx, f.owner.Token)
		},
	))
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+f.projectID+"/runtime/emergency-blackout",
		[]byte(`{"enabled":false,"confirm":"CLEAR","request_id":"018f5b34-0f47-7cc0-9000-0a0000000113"}`), f.owner.Token, f.owner.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !safetyReached || res.Code != http.StatusForbidden ||
		!strings.Contains(res.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(res.Body.String(), `"retryable":false`) {
		t.Fatalf("blackout clear after session revocation returned %d (safety=%t): %s", res.Code, safetyReached, res.Body)
	}
	latched, err := f.projects.SessionManagedOutputBlackout(ctx, active.ID)
	if err != nil || !latched {
		t.Fatalf("revoked clear released GO latch: %t, %v", latched, err)
	}
}

func TestEmergencyBlackoutSuppressesResultAfterOwnerRevocation(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := f.projects.StartOperationalSession(ctx, published.ID, domain.SessionShow, "SHOW", "")
	if err != nil {
		t.Fatal(err)
	}
	safetyApplied := false
	runtime := runtimecontrol.New(f.projects, capability.NewRegistry(), runtimecontrol.WithEmergencySafety(
		func(context.Context, domain.Session, contracts.CommandEnvelope, bool) (json.RawMessage, error) {
			safetyApplied = true
			return json.RawMessage(`{"managed_output_blackout":true}`), f.h.auth.Logout(ctx, f.owner.Token)
		},
	))
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+f.projectID+"/runtime/emergency-blackout",
		[]byte(`{"enabled":true,"confirm":"BLACKOUT","request_id":"018f5b34-0f47-7cc0-9000-0a0000000126"}`), f.owner.Token, f.owner.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !safetyApplied {
		t.Fatal("test did not cross the mocked safety-output boundary")
	}
	if res.Code != http.StatusForbidden || strings.Contains(res.Body.String(), "managed_output_blackout") ||
		!strings.Contains(res.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(res.Body.String(), `"retryable":false`) {
		t.Fatalf("revoked Owner received Emergency Blackout result or retryable response: HTTP %d %s", res.Code, res.Body.String())
	}
	latched, err := f.projects.SessionManagedOutputBlackout(ctx, active.ID)
	if err != nil || !latched {
		t.Fatalf("revoked response undid the latched Emergency Blackout: latched=%t err=%v", latched, err)
	}
}

func TestLegacyBrowserCueRevocationSuppressesDispatchAndReply(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.projects.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.projects.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: project.CurrentRevisionID, Name: "Revocation Cue", OrderIndex: 1, Enabled: true,
	}, []domain.Action{{
		ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	crossedBoundary := false
	registry := capability.NewRegistry()
	if err := registry.Register("sim.test", capability.ExecutorFunc(func(ctx context.Context, _ capability.Request) capability.Result {
		if err := f.h.auth.Logout(ctx, f.owner.Token); err != nil {
			t.Fatal(err)
		}
		crossedBoundary = true
		if !errors.Is(capability.CheckDispatch(ctx), capability.ErrDispatchAuthorityRevoked) {
			t.Error("revoked browser session still authorized the adapter write")
		}
		return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: capability.DispatchAuthorityRevokedCode}
	})); err != nil {
		t.Fatal(err)
	}
	runtime := runtimecontrol.New(f.projects, registry)
	_, started := runtime.StartSession(ctx, runtimecontrol.StartRequest{
		ProjectID: project.ID, Mode: domain.SessionRehearsal, Issuer: f.owner.Session.User.ID,
		RequestID: "00000000-0000-7000-8000-000000000995",
	})
	if started.Status != contracts.CommandCompleted {
		t.Fatalf("start rehearsal: %+v", started)
	}
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime)).Handler()
	req := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/go",
		[]byte(`{"request_id":"00000000-0000-7000-8000-000000000996"}`), f.owner.Token, f.owner.CSRFToken)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !crossedBoundary || res.Code != http.StatusUnauthorized || strings.Contains(res.Body.String(), "cue_execution_id") {
		t.Fatalf("boundary=%t status=%d body=%s", crossedBoundary, res.Code, res.Body)
	}
}
