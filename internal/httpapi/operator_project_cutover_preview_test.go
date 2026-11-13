package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOwnerProjectCutoverPreviewRouteIsReadOnlyOwnerOnlyAndRechecksSession(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	projectA, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Preview show A"})
	if err != nil {
		t.Fatal(err)
	}
	projectB, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Preview show B"})
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
	operator, err := h.auth.CreateUser(ctx, "preview-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.CreateUser(ctx, "preview-viewer", h.password, userauth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	operatorLogin, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerLogin, err := h.auth.Login(ctx, viewer.Username, h.password, "127.0.0.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, projectA.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ShowGO}); err != nil {
		t.Fatal(err)
	}
	var grantsBefore int
	if err := h.db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_permission_grants`).Scan(&grantsBefore); err != nil {
		t.Fatal(err)
	}
	options := []Option{WithOwnerProjectCutoverPreview(h.auth, access), WithProjectRouteQualification(access)}
	handler := New(options...).Handler()
	request := func(login userauth.Credential) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, authenticatedReadRequest(http.MethodGet, "/api/v1/project-access/cutover-preview", login.Token))
		return recorder
	}
	ownerResponse := request(owner)
	if ownerResponse.Code != http.StatusOK || ownerResponse.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(ownerResponse.Body.String(), projectA.ID) || !strings.Contains(ownerResponse.Body.String(), projectB.Name) {
		t.Fatalf("Owner preview response=%d headers=%v body=%s", ownerResponse.Code, ownerResponse.Header(), ownerResponse.Body.String())
	}
	var preview projectaccess.CutoverImpactPreview
	if err := json.Unmarshal(ownerResponse.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.ReadOnly || preview.Enforced || preview.ReadyForCutover || preview.ProjectCount != 2 {
		t.Fatalf("Owner received an optimistic or mutating cutover preview: %+v", preview)
	}
	for _, login := range []userauth.Credential{operatorLogin, viewerLogin} {
		response := request(login)
		if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), projectA.ID) || strings.Contains(response.Body.String(), projectA.Name) {
			t.Fatalf("non-Owner received cutover impact data: HTTP %d %s", response.Code, response.Body.String())
		}
	}
	var grantsAfter int
	if err := h.db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_permission_grants`).Scan(&grantsAfter); err != nil {
		t.Fatal(err)
	}
	if grantsAfter != grantsBefore {
		t.Fatalf("read-only Owner preview changed grants: %d -> %d", grantsBefore, grantsAfter)
	}

	revokedHandler := New(withOwnerProjectCutoverPreview(h.auth, access, ownerProjectCutoverPreviewHooks{
		BeforeResponse: func() {
			if err := h.auth.Logout(ctx, owner.Token); err != nil {
				t.Errorf("revoke Owner session: %v", err)
			}
		},
	}), WithProjectRouteQualification(access)).Handler()
	revokedResponse := httptest.NewRecorder()
	revokedHandler.ServeHTTP(revokedResponse, authenticatedReadRequest(http.MethodGet, "/api/v1/project-access/cutover-preview", owner.Token))
	if revokedResponse.Code != http.StatusUnauthorized || strings.Contains(revokedResponse.Body.String(), projectA.ID) || strings.Contains(revokedResponse.Body.String(), projectA.Name) {
		t.Fatalf("revoked Owner received cutover impact data: HTTP %d %s", revokedResponse.Code, revokedResponse.Body.String())
	}
}

func TestOwnerProjectCutoverPreviewSuppressesErrorAfterSessionRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(withOwnerProjectCutoverPreview(h.auth, access, ownerProjectCutoverPreviewHooks{
		Preview: func(context.Context, string) (projectaccess.CutoverImpactPreview, error) {
			if err := h.auth.Logout(ctx, owner.Token); err != nil {
				t.Errorf("revoke Owner session while report fails: %v", err)
			}
			return projectaccess.CutoverImpactPreview{}, errors.New("preview database failure")
		},
	}), WithProjectRouteQualification(access)).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, "/api/v1/project-access/cutover-preview", owner.Token))
	if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), "PROJECT_ACCESS_PREVIEW_UNAVAILABLE") {
		t.Fatalf("revoked Owner received cutover preview failure after authority ended: HTTP %d %s", response.Code, response.Body.String())
	}
}
