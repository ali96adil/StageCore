package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/routing"
	"github.com/ali96adil/StageCore/internal/simulationinput"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestSimulationInputInjectionRejectsRevokedGrantWhileReadingBody(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "simulation inject grant"})
	if err != nil { t.Fatal(err) }
	access, err := projectaccess.New(h.db.DB)
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	operator, err := h.auth.CreateUser(ctx, "simulation-input-operator", h.password, userauth.RoleOperator)
	if err != nil { t.Fatal(err) }
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.RehearsalControl}); err != nil { t.Fatal(err) }
	inputs := simulationinput.New(st, routing.NewSimulationSafe(st, simulator.Adapter{}))
	handler := New(WithProjectRouteQualification(access), WithOperatorSimulationInputs(h.auth, inputs)).Handler()
	body := `{"request_id":"00000000-0000-7000-8000-000000000741","input_id":"missing","value":1}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/simulation/inputs/inject", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:28124"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	request.Header.Set(csrfHeader, login.CSRFToken)
	request.Header.Set("Content-Type", "application/json")
	revoked := false
	request.Body = io.NopCloser(revokeDuringRead{Reader: strings.NewReader(body), revoke: func() {
		if revoked { return }
		revoked = true
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil { t.Fatal(err) }
	}})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, request)
	if !revoked || res.Code != http.StatusNotFound {
		t.Fatalf("revoked simulation input injected: revoked=%v status=%d body=%s", revoked, res.Code, res.Body)
	}
}
