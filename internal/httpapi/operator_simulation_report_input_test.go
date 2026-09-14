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
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/simulationinput"
	"github.com/ali96adil/StageCore/internal/simulationreport"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type simulationAPIPhysicalProbe struct {
	calls int
}

func (p *simulationAPIPhysicalProbe) Execute(context.Context, capability.Request) capability.Result {
	p.calls++
	return capability.Result{
		Result:   domain.ExecutionCompleted,
		AckLevel: contracts.AckDevice,
	}
}

func TestOperatorSimulationReportAndInputRBACPreserveNoRealOutput(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	deviceRepository, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{
		Name:      "Simulation API Acceptance",
		CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:   project.ID,
		LogicalName: "SIM-TARGET",
		LogicalType: "GENERIC",
	}); err != nil {
		t.Fatal(err)
	}
	input, err := projectStore.CreateInput(ctx, domain.InputDefinition{
		RevisionID:  revision.ID,
		Name:        "SIM-SENSOR",
		SourceRef:   "simulation:test",
		EventType:   "sensor.value",
		ValueSchema: json.RawMessage(`{"type":"number"}`),
		Enabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := projectStore.CreateOutput(ctx, domain.OutputDefinition{
		RevisionID:    revision.ID,
		Name:          "SIM-OUTPUT",
		TargetRef:     "SIM-TARGET",
		CapabilityKey: "stage.test.physical",
		ValueSchema:   json.RawMessage(`{"type":"object"}`),
		Criticality:   "NORMAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	outputID := output.ID
	if _, err := projectStore.CreateRouteWithActions(ctx, domain.Route{
		RevisionID:          revision.ID,
		Name:                "SIM-SENSOR -> SIM-OUTPUT",
		InputID:             input.ID,
		ConditionDefinition: json.RawMessage(`{"operator":"equals","value":1}`),
		TransformDefinition: json.RawMessage(`null`),
		PriorityClass:       domain.PriorityP1,
		ErrorPolicy:         json.RawMessage(`{"on_error":"STOP_ROUTE"}`),
		Enabled:             true,
	}, []domain.RouteAction{{
		OrderIndex: 0,
		OutputID:   &outputID,
		Parameters: json.RawMessage(`{}`),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(projectStore).Create(ctx, revision.ID, owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := projectStore.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID:  published.ID,
		SessionType: domain.SessionSimulation,
		Name:        "Simulation API",
		StartPosition: domain.SessionStartPosition{
			Version:  domain.SessionContractVersion1,
			Kind:     domain.SessionStartBeginning,
			Metadata: json.RawMessage(`{}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	twin := simulator.NewDigitalTwin()
	physical := &simulationAPIPhysicalProbe{}
	inputService := simulationinput.New(
		projectStore,
		routing.NewSimulationSafeWithDigitalTwin(projectStore, physical, twin),
	)
	reportService := simulationreport.New(projectStore, deviceRepository, twin)
	handler := New(
		WithOperatorSimulationInputs(h.auth, inputService),
		WithOperatorSimulationReport(h.auth, reportService),
	).Handler()

	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000099', 'sim-viewer', ?, 'VIEWER', 1, 1, 1)
	`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "sim-viewer", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/simulation/report/"+session.ID, nil)
	unauthenticated.RemoteAddr = "127.0.0.1:1234"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated report status=%d, want 401", unauthenticatedRes.Code)
	}

	viewerReportReq := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/simulation/report/"+session.ID, viewer.Token)
	viewerReportRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerReportRes, viewerReportReq)
	if viewerReportRes.Code != http.StatusOK {
		t.Fatalf("VIEWER report status=%d body=%s", viewerReportRes.Code, viewerReportRes.Body.String())
	}
	var reportPayload struct {
		Report simulationreport.Report `json:"report"`
	}
	if err := json.Unmarshal(viewerReportRes.Body.Bytes(), &reportPayload); err != nil {
		t.Fatal(err)
	}
	if reportPayload.Report.SessionID != session.ID || reportPayload.Report.ProjectID != project.ID {
		t.Fatalf("unexpected report authority: %+v", reportPayload.Report)
	}

	injectBody, _ := json.Marshal(simulationInputInjectRequest{
		RequestID: "00000000-0000-7000-8000-000000000701",
		InputID:   input.ID,
		Value:     json.RawMessage(`1`),
	})
	viewerInjectReq := authenticatedMutationRequest(
		t,
		http.MethodPost,
		"/api/v1/projects/"+project.ID+"/simulation/inputs/inject",
		injectBody,
		viewer.Token,
		viewer.CSRFToken,
	)
	viewerInjectRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerInjectRes, viewerInjectReq)
	if viewerInjectRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER inject status=%d, want 403 body=%s", viewerInjectRes.Code, viewerInjectRes.Body.String())
	}

	ownerInjectReq := authenticatedMutationRequest(
		t,
		http.MethodPost,
		"/api/v1/projects/"+project.ID+"/simulation/inputs/inject",
		injectBody,
		owner.Token,
		owner.CSRFToken,
	)
	ownerInjectRes := httptest.NewRecorder()
	handler.ServeHTTP(ownerInjectRes, ownerInjectReq)
	if ownerInjectRes.Code != http.StatusOK {
		t.Fatalf("OWNER inject status=%d body=%s", ownerInjectRes.Code, ownerInjectRes.Body.String())
	}
	var injectResponse struct {
		Result contracts.CommandResult `json:"result"`
		Scope  string                  `json:"scope"`
		Source string                  `json:"source"`
	}
	if err := json.Unmarshal(ownerInjectRes.Body.Bytes(), &injectResponse); err != nil {
		t.Fatal(err)
	}
	if injectResponse.Result.Status != contracts.CommandCompleted || injectResponse.Scope != "SIMULATION_ONLY" || injectResponse.Source != "TEST" {
		t.Fatalf("unexpected Simulation inject response: %+v", injectResponse)
	}
	if physical.calls != 0 {
		t.Fatalf("Simulation input reached physical executor %d time(s)", physical.calls)
	}

	events, err := projectStore.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundSimulationExecution := false
	for _, event := range events {
		if event.EventType == "simulation.execution.completed" {
			foundSimulationExecution = true
			break
		}
	}
	if !foundSimulationExecution {
		t.Fatalf("Simulation input did not leave canonical execution evidence: %+v", events)
	}
}
