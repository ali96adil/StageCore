package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/simulationcontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

func TestSimulationOperatorStatusAndFaultInjectionUseActiveSessionAndSnakeCaseCues(t *testing.T) {
	f := setupProjectAccessAPI(t)
	ctx := context.Background()
	project, err := f.projects.GetProject(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.access.SetPermissions(ctx, f.owner.Session.User.ID, f.operatorID, project.ID, []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.RehearsalControl,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.projects.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: f.projectID, LogicalName: "SIM", LogicalType: "GENERIC"}); err != nil {
		t.Fatal(err)
	}
	cue, err := f.projects.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: project.CurrentRevisionID, DisplayLabel: "1", Name: "Fault acceptance cue",
		OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "SIM", CapabilityKey: "sim.test",
		Parameters: json.RawMessage(`{}`), TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.projects.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(f.projects).Create(ctx, project.CurrentRevisionID, f.owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	physical := capability.NewRegistry()
	if err := physical.Register("sim.test", simulator.Adapter{}); err != nil {
		t.Fatal(err)
	}
	twin := simulator.NewDigitalTwin()
	simulationExecutor := simulator.NewSessionExecutorWithDigitalTwin(f.projects, physical, twin)
	var managedOutputCalls atomic.Int32
	runtime := runtimecontrol.New(f.projects, physical,
		runtimecontrol.WithSimulationExecutor(simulationExecutor),
		runtimecontrol.WithEmergencySafety(func(context.Context, domain.Session, contracts.CommandEnvelope, bool) (json.RawMessage, error) {
			managedOutputCalls.Add(1)
			return json.RawMessage(`{"managed_output":"called"}`), nil
		}),
	)
	control := simulationcontrol.New(f.projects, cueengine.NewWithExecutor(f.projects, simulationExecutor), twin, runtime)
	handler := New(WithOperatorRuntime(f.h.auth, f.projects, runtime), WithProjectRouteQualification(f.access), WithOperatorSimulation(f.h.auth, control)).Handler()
	startBody, _ := json.Marshal(simulationStartRequest{
		RequestID: "00000000-0000-7000-8000-000000009911", Name: "Fault acceptance",
		StartKind: string(domain.SessionStartBeginning),
	})
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/simulation/start", startBody, f.operator.Token, f.operator.CSRFToken))
	if start.Code != http.StatusCreated {
		t.Fatalf("Simulation start status=%d body=%s", start.Code, start.Body.String())
	}
	var started struct {
		Session struct {
			ID string `json:"session_id"`
		} `json:"session"`
		Result contracts.CommandResult `json:"result"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil || started.Session.ID == "" || started.Result.Status != contracts.CommandCompleted {
		t.Fatalf("Simulation start response=%+v err=%v body=%s", started, err, start.Body.String())
	}
	schedulingBody := func(requestID string) []byte {
		body, _ := json.Marshal(runtimeCommandRequest{ExpectedSessionID: &started.Session.ID, RequestID: requestID})
		return body
	}
	pause := httptest.NewRecorder()
	handler.ServeHTTP(pause, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/runtime/pause-cue-scheduling",
		schedulingBody("00000000-0000-7000-8000-000000009917"), f.operator.Token, f.operator.CSRFToken))
	if pause.Code != http.StatusOK || !containsJSON(pause.Body.Bytes(), `"scope":"NEW_GO_AND_JUMP_ADMISSION_ONLY"`) {
		t.Fatalf("Simulation Pause API status=%d body=%s", pause.Code, pause.Body.String())
	}
	pausedGO := httptest.NewRecorder()
	handler.ServeHTTP(pausedGO, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/runtime/go",
		schedulingBody("00000000-0000-7000-8000-000000009918"), f.operator.Token, f.operator.CSRFToken))
	if pausedGO.Code != http.StatusConflict || !containsJSON(pausedGO.Body.Bytes(), `"error_code":"CUE_SCHEDULING_PAUSED"`) {
		t.Fatalf("Simulation admitted GO during Pause: HTTP %d %s", pausedGO.Code, pausedGO.Body.String())
	}
	resume := httptest.NewRecorder()
	handler.ServeHTTP(resume, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/runtime/resume-cue-scheduling",
		schedulingBody("00000000-0000-7000-8000-000000009919"), f.operator.Token, f.operator.CSRFToken))
	if resume.Code != http.StatusOK || !containsJSON(resume.Body.Bytes(), `"cue_scheduling_paused":false`) {
		t.Fatalf("Simulation Resume API status=%d body=%s", resume.Code, resume.Body.String())
	}
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+f.projectID+"/simulation", f.operator.Token))
	if status.Code != http.StatusOK || !containsJSON(status.Body.Bytes(), `"session_id":"`+started.Session.ID+`"`) ||
		!containsJSON(status.Body.Bytes(), `"next_cue_id":"`+cue.ID+`"`) {
		t.Fatalf("Simulation status did not provide UI cue/session fields: HTTP %d %s", status.Code, status.Body.String())
	}
	faultBody := []byte(`{"target_ref":"SIM","capability":"sim.test","behavior":"FAIL","uses":1,"error_code":"PHASE2_FAULT"}`)
	fault := httptest.NewRecorder()
	handler.ServeHTTP(fault, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/simulation/faults", faultBody, f.operator.Token, f.operator.CSRFToken))
	if fault.Code != http.StatusCreated || !containsJSON(fault.Body.Bytes(), `"scope":"SIMULATION_ONLY"`) {
		t.Fatalf("valid active-session fault injection failed: HTTP %d %s", fault.Code, fault.Body.String())
	}
	faultGo := httptest.NewRecorder()
	handler.ServeHTTP(faultGo, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/simulation/go",
		[]byte(`{"request_id":"00000000-0000-7000-8000-000000009914"}`),
		f.operator.Token, f.operator.CSRFToken))
	if faultGo.Code != http.StatusOK {
		t.Fatalf("GO after configuring Simulation fault returned HTTP %d %s", faultGo.Code, faultGo.Body.String())
	}
	var faultObserved bool
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		faultStatus := httptest.NewRecorder()
		handler.ServeHTTP(faultStatus, authenticatedReadRequest(http.MethodGet,
			"/api/v1/projects/"+f.projectID+"/simulation", f.operator.Token))
		if faultStatus.Code != http.StatusOK {
			t.Fatalf("Simulation status after faulted GO returned HTTP %d %s", faultStatus.Code, faultStatus.Body.String())
		}
		if containsJSON(faultStatus.Body.Bytes(), `"result":"FAILED"`) && containsJSON(faultStatus.Body.Bytes(), "PHASE2_FAULT") {
			faultObserved = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !faultObserved {
		t.Fatal("configured Simulation fault did not produce a visible terminal failed CueExecution")
	}
	if managedOutputCalls.Load() != 0 {
		t.Fatalf("Simulation faulted GO reached managed outputs %d time(s)", managedOutputCalls.Load())
	}

	blackout := httptest.NewRecorder()
	handler.ServeHTTP(blackout, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/runtime/emergency-blackout",
		[]byte(`{"request_id":"00000000-0000-7000-8000-000000009915","enabled":true,"confirm":"BLACKOUT"}`),
		f.operator.Token, f.operator.CSRFToken))
	if blackout.Code != http.StatusOK || !containsJSON(blackout.Body.Bytes(), `"scope":"SIMULATION_ONLY"`) ||
		!containsJSON(blackout.Body.Bytes(), `"physical_dispatch":false`) || managedOutputCalls.Load() != 0 {
		t.Fatalf("Simulation Emergency Blackout crossed managed-output boundary: HTTP %d managed calls=%d body=%s", blackout.Code, managedOutputCalls.Load(), blackout.Body.String())
	}
	latched, err := f.projects.SessionManagedOutputBlackout(ctx, started.Session.ID)
	if err != nil || !latched {
		t.Fatalf("Simulation Emergency Blackout latch=%t err=%v", latched, err)
	}
	clear := httptest.NewRecorder()
	handler.ServeHTTP(clear, authenticatedMutationRequest(t, http.MethodPost,
		"/api/v1/projects/"+f.projectID+"/runtime/emergency-blackout",
		[]byte(`{"request_id":"00000000-0000-7000-8000-000000009916","enabled":false,"confirm":"CLEAR"}`),
		f.operator.Token, f.operator.CSRFToken))
	if clear.Code != http.StatusOK || !containsJSON(clear.Body.Bytes(), `"scope":"SIMULATION_ONLY"`) || managedOutputCalls.Load() != 0 {
		t.Fatalf("Simulation Emergency Blackout clear crossed managed-output boundary: HTTP %d managed calls=%d body=%s", clear.Code, managedOutputCalls.Load(), clear.Body.String())
	}
	latched, err = f.projects.SessionManagedOutputBlackout(ctx, started.Session.ID)
	if err != nil || latched {
		t.Fatalf("cleared Simulation Emergency Blackout latch=%t err=%v", latched, err)
	}
}
