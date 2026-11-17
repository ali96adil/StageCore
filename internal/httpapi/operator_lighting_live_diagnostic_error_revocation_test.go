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

func TestLightingDiagnosticSuppressesProjectReadErrorAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.CreateUser(ctx, "lighting-diagnostic-error-viewer", h.password, userauth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting diagnostic error fence", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE projects RENAME COLUMN current_revision_id TO current_revision_fault"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE projects RENAME COLUMN current_revision_fault TO current_revision_id"); err != nil {
			t.Errorf("restore projects schema: %v", err)
		}
	}()
	revoked := false
	reader := lightingDiagnosticReadFunc(func(context.Context, string, string) livereconcile.BlockedSoftwareDiagnostic {
		t.Error("diagnostic reader ran after Project read failed")
		return livereconcile.BlockedSoftwareDiagnostic{Status: livereconcile.SoftwareDiagnosticUnknown}
	})
	handler := New(
		WithProjectRouteQualification(access),
		withOperatorLightingLiveDiagnostics(h.auth, st, reader, lightingLiveDiagnosticHooks{BeforeProjectReadError: func() {
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, viewer.ID, project.ID, nil); err != nil {
				t.Errorf("revoke ProjectView: %v", err)
			}
		}}),
	).Handler()
	path := "/api/v1/projects/" + project.ID + "/lighting-controller/nodes/node-1/live-diagnostic"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, path, login.Token))
	if !revoked {
		t.Fatalf("project Store error hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "PROJECTS_UNAVAILABLE") {
		t.Fatalf("revoked diagnostic returned status=%d body=%s", response.Code, response.Body.String())
	}
}
