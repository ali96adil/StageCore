package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/simulationcontrol"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/clock"
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
	if err != nil { t.Fatal(err) }
	access, err := projectaccess.New(h.db.DB)
	if err != nil { t.Fatal(err) }
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
	viewer, err := h.auth.CreateUser(ctx, "simulation-revoked-viewer", h.password, userauth.RoleViewer)
	if err != nil { t.Fatal(err) }
	login, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.1")
	if err != nil { t.Fatal(err) }
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
		if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, nil); err != nil { t.Fatal(err) }
		active := domain.Session{Type: domain.SessionSimulation, Status: domain.SessionActive}
		return simulationcontrol.Status{ProjectID: project.ID, Session: &active}, nil
	})
	w := httptest.NewRecorder()
	if active, ok := activeSimulationSession(w, request, reader); ok || !read || w.Code != http.StatusNotFound || active.ID != "" {
		t.Fatalf("revoked simulation status got active=%+v ok=%v read=%v status=%d body=%s", active, ok, read, w.Code, w.Body.String())
	}
}
