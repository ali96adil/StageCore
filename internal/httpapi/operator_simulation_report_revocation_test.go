package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/simulationreport"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type simulationReportReadFunc func(context.Context, string) (simulationreport.Report, error)

func (f simulationReportReadFunc) Generate(ctx context.Context, sessionID string) (simulationreport.Report, error) {
	return f(ctx, sessionID)
}

func TestSimulationReportWithholdsDataWhenProjectGrantIsRevokedDuringRead(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "simulation access"})
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
	viewer, err := h.auth.CreateUser(ctx, "simulation-viewer", h.password, userauth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	read := false
	reports := simulationReportReadFunc(func(ctx context.Context, _ string) (simulationreport.Report, error) {
		read = true
		if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, nil); err != nil {
			t.Fatal(err)
		}
		return simulationreport.Report{ProjectID: project.ID}, nil
	})
	handler := New(WithProjectRouteQualification(access), withOperatorSimulationReportReader(h.auth, reports)).Handler()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/simulation/report/test", login.Token))
	if !read || res.Code != http.StatusNotFound || strings.Contains(res.Body.String(), project.ID) {
		t.Fatalf("read=%t status=%d body=%s", read, res.Code, res.Body)
	}
}
