package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/livereconcile"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type lightingDiagnosticReadFunc func(context.Context, string, string) livereconcile.BlockedSoftwareDiagnostic

func (f lightingDiagnosticReadFunc) Read(ctx context.Context, projectID, deviceID string) livereconcile.BlockedSoftwareDiagnostic {
	return f(ctx, projectID, deviceID)
}

func TestLightingLiveDiagnosticWithholdsDataAfterInFlightProjectRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "lighting diagnostic grant revocation"})
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
	viewer, err := h.auth.CreateUser(ctx, "lighting-diagnostic-viewer", h.password, userauth.RoleViewer)
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
	reader := lightingDiagnosticReadFunc(func(ctx context.Context, projectID, deviceID string) livereconcile.BlockedSoftwareDiagnostic {
		read = true
		if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, nil); err != nil {
			t.Fatal(err)
		}
		return livereconcile.BlockedSoftwareDiagnostic{
			Status: livereconcile.SoftwareDiagnosticUnsafe,
			ProjectID: projectID, CueID: "private-cue-id", Reason: "private-lighting-detail",
			DesiredSlots: map[int]uint8{1: 180},
		}
	})
	handler := New(WithProjectRouteQualification(access),
		WithOperatorLightingLiveDiagnostics(h.auth, st, reader)).Handler()
	path := "/api/v1/projects/" + project.ID + "/lighting-controller/nodes/node-1/live-diagnostic"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, authenticatedReadRequest(http.MethodGet, path, login.Token))
	if !read || res.Code != http.StatusNotFound ||
		strings.Contains(res.Body.String(), "private-cue-id") ||
		strings.Contains(res.Body.String(), "private-lighting-detail") ||
		strings.Contains(res.Body.String(), "180") {
		t.Fatalf("revoked lighting diagnostic leaked: read=%t status=%d body=%s", read, res.Code, res.Body)
	}
}
