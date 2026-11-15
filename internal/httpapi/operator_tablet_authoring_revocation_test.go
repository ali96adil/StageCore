package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestTabletSceneReadSuppressesStoreErrorAfterGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "tablet-scene-error-operator", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Tablet scene read fence", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, "DROP TABLE cues"); err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(
		withOperatorTabletAuthoring(h.auth, devices, st, operatorTabletAuthoringHooks{BeforeErrorResponse: func(route string) {
			if route != "tablet-scenes-cues" {
				t.Errorf("error hook route=%q", route)
			}
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Errorf("revoke ProjectView: %v", err)
			}
		}}),
		WithProjectRouteQualification(access),
	).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/tablet-controller/scenes", login.Token))
	if !revoked {
		t.Fatalf("Store error boundary did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) || strings.Contains(response.Body.String(), "TABLET_SCENES_UNAVAILABLE") {
		t.Fatalf("revoked Tablet Scene read returned status=%d body=%s", response.Code, response.Body.String())
	}
}
