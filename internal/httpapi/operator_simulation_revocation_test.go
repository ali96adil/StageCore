package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/simulationcontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type simulationStatusReaderFunc func(context.Context, string) (simulationcontrol.Status, error)

func (f simulationStatusReaderFunc) Status(ctx context.Context, projectID string) (simulationcontrol.Status, error) {
	return f(ctx, projectID)
}

func TestActiveSimulationSessionRejectsGrantRevokedDuringStatusRead(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "revoke simulation during read"})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.CreateUser(ctx, "simulation-revoked-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView, projectaccess.RehearsalControl}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/simulation/go", nil)
	request.SetPathValue("project_id", project.ID)
	request = request.WithContext(context.WithValue(request.Context(), projectRouteGrantKey{}, projectRouteGrant{
		access: access, auth: h.auth, token: login.Token, user: login.Session,
		needed: []projectaccess.Permission{projectaccess.RehearsalControl},
	}))
	if _, err := currentProjectRouteGrant(request); err != nil {
		t.Fatalf("expected authorized simulation control before read: %v", err)
	}
	read := false
	reader := simulationStatusReaderFunc(func(ctx context.Context, projectID string) (simulationcontrol.Status, error) {
		read = true
		if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, nil); err != nil {
			t.Fatal(err)
		}
		active := domain.Session{Type: domain.SessionSimulation, Status: domain.SessionActive}
		return simulationcontrol.Status{ProjectID: project.ID, Session: &active}, nil
	})
	w := httptest.NewRecorder()
	if active, ok := activeSimulationSession(w, request, reader); ok || !read || w.Code != http.StatusNotFound || active.ID != "" {
		t.Fatalf("revoked simulation status got active=%+v ok=%v read=%v status=%d body=%s", active, ok, read, w.Code, w.Body.String())
	}
}

func TestSimulationStartRejectsProjectGrantRevokedWhileReadingBody(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "simulation request read boundary"})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "simulation-body-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView, projectaccess.RehearsalControl}); err != nil {
		t.Fatal(err)
	}
	twin := simulator.NewDigitalTwin()
	engine := cueengine.NewWithExecutor(st, simulator.NewSessionExecutorWithDigitalTwin(st, simulator.Adapter{}, twin))
	control := simulationcontrol.New(st, engine, twin)
	handler := New(WithProjectRouteQualification(access), WithOperatorSimulation(h.auth, control)).Handler()
	body := `{"request_id":"00000000-0000-7000-8000-000000000731","name":"should-not-create"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/simulation/start", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:28123"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	request.Header.Set(csrfHeader, login.CSRFToken)
	request.Header.Set("Content-Type", "application/json")
	revoked := false
	request.Body = io.NopCloser(revokeDuringRead{Reader: strings.NewReader(body), revoke: func() {
		if revoked {
			return
		}
		revoked = true
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
			t.Fatal(err)
		}
	}})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if !revoked || w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "should-not-create") {
		t.Fatalf("revoked simulation start: revoked=%v status=%d body=%s", revoked, w.Code, w.Body.String())
	}
	if status, err := control.Status(ctx, project.ID); err != nil || status.Session != nil {
		t.Fatalf("revoked simulation created session: status=%+v err=%v", status, err)
	}
}

func TestSimulationStartSuppressesResultAfterProjectGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name:      "simulation response revocation boundary",
		CreatedBy: owner.Session.User.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, owner.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "simulation-response-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	permissions := []projectaccess.Permission{projectaccess.ProjectView, projectaccess.RehearsalControl}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, permissions); err != nil {
		t.Fatal(err)
	}
	twin := simulator.NewDigitalTwin()
	engine := cueengine.NewWithExecutor(stageStore, simulator.NewSessionExecutorWithDigitalTwin(stageStore, simulator.Adapter{}, twin))
	control := simulationcontrol.New(stageStore, engine, twin)
	revoked := false
	handler := New(
		WithProjectRouteQualification(access),
		withOperatorSimulation(h.auth, control, operatorSimulationHooks{BeforeResponse: func() {
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Fatal(err)
			}
		}}),
	).Handler()
	body := `{"request_id":"00000000-0000-7000-8000-000000000732","name":"suppressed-after-revocation","start_kind":"BEGINNING"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/simulation/start", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:28124"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	request.Header.Set(csrfHeader, login.CSRFToken)
	request.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if !revoked {
		t.Fatal("response boundary hook did not revoke the project grant")
	}
	if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "suppressed-after-revocation") {
		t.Fatalf("revoked simulation start exposed result: status=%d body=%s", w.Code, w.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["effect_may_be_committed"] != true || response["retryable"] != false {
		t.Fatalf("revoked simulation response lost commit uncertainty: %v", response)
	}
	status, err := control.Status(ctx, project.ID)
	if err != nil || status.Session == nil || status.Session.Status != domain.SessionActive {
		t.Fatalf("simulation start effect was not retained truthfully: status=%+v err=%v", status, err)
	}
}
