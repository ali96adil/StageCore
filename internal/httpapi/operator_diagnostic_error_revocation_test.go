package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/simulationreport"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type preflightErrorRevocationFunc func(context.Context, string, string) (preflight.Report, error)
func (f preflightErrorRevocationFunc) Evaluate(ctx context.Context, projectID, snapshotID string) (preflight.Report, error) {
	return f(ctx, projectID, snapshotID)
}

func TestPreflightFailureDoesNotRevealProjectErrorAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "secret preflight"})
	if err != nil { t.Fatal(err) }
	access, err := projectaccess.New(h.db.DB)
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	viewer, err := h.auth.CreateUser(ctx, "preflight-revoked-viewer", h.password, userauth.RoleViewer)
	if err != nil { t.Fatal(err) }
	login, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil { t.Fatal(err) }
	called := false
	evaluator := preflightErrorRevocationFunc(func(ctx context.Context, _, _ string) (preflight.Report, error) {
		called = true
		if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, nil); err != nil { t.Fatal(err) }
		return preflight.Report{}, errors.New("PRIVATE_DIAGNOSTIC_DO_NOT_LEAK")
	})
	handler := New(WithProjectRouteQualification(access), WithOperatorPreflight(h.auth, evaluator)).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/preflight", login.Token))
	if !called || res.Code != http.StatusNotFound || strings.Contains(res.Body.String(), "PRIVATE_DIAGNOSTIC_DO_NOT_LEAK") {
		t.Fatalf("revoked preflight error exposed: called=%t status=%d body=%s", called, res.Code, res.Body.String())
	}
}

func TestSimulationReportFailureDoesNotRevealProjectErrorAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	called := false
	reports := simulationReportReadFunc(func(ctx context.Context, _ string) (simulationreport.Report, error) {
		called = true
		if err := h.auth.Logout(ctx, owner.Token); err != nil { t.Fatal(err) }
		return simulationreport.Report{}, errors.New("PRIVATE_SIMULATION_TRACE_DO_NOT_LEAK")
	})
	handler := New(withOperatorSimulationReportReader(h.auth, reports)).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/project/simulation/report/hidden", owner.Token))
	if !called || res.Code != http.StatusUnauthorized || strings.Contains(res.Body.String(), "PRIVATE_SIMULATION_TRACE_DO_NOT_LEAK") {
		t.Fatalf("revoked report error exposed: called=%t status=%d body=%s", called, res.Code, res.Body.String())
	}
}
