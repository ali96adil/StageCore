package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestDraftHistorySuppressesStoreErrorsAfterProjectGrantRevocation(t *testing.T) {
	cases := []struct {
		name, path, table, hookName string
	}{
		{"Cue", "cue-draft-history", "cue_draft_history_state", "cue-draft-history"},
		{"Route", "route-draft-history", "route_draft_history_state", "route-draft-history"},
		{"Input", "input-draft-history", "input_draft_history_state", "input-draft-history"},
		{"Output", "output-draft-history", "output_draft_history_state", "output-draft-history"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)
			ctx := context.Background()
			st := store.New(h.db.DB, clock.Real{})
			owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			tech, err := h.auth.CreateUser(ctx, "history-revoke-"+strings.ToLower(tc.name), h.password, userauth.RoleTechnician)
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, tech.Username, h.password, "127.0.0.2")
			if err != nil {
				t.Fatal(err)
			}
			access, err := projectaccess.New(h.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: tc.name + " history", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
				[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit, projectaccess.CueEdit}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.db.DB.ExecContext(ctx, "DROP TABLE "+tc.table); err != nil {
				t.Fatal(err)
			}
			revoked := false
			beforeError := func(route string) {
				if route != tc.hookName {
					t.Errorf("error hook route=%q want=%q", route, tc.hookName)
				}
				revoked = true
				if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
					[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
					t.Fatal(err)
				}
			}
			handler := New(
				withOperatorCuePublish(h.auth, st, publish.New(st, capability.NewRegistry()), cuePublishHooks{BeforeErrorResponse: beforeError}),
				withOperatorConfigurationHooks(h.auth, st, operatorConfigurationHooks{BeforeErrorResponse: beforeError}),
				WithProjectRouteQualification(access),
			).Handler()
			route := "/api/v1/projects/" + project.ID + "/" + tc.path
			request := authenticatedReadRequest(http.MethodGet, route, login.Token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("storage error hook did not revoke the Project grant: status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) {
				t.Fatalf("revoked history read returned status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "no such table") || strings.Contains(response.Body.String(), "history status") {
				t.Fatalf("storage error escaped after Project grant revocation: %s", response.Body.String())
			}
		})
	}
}

func TestProjectConfigurationSuppressesStoreErrorAfterProjectGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	tech, err := h.auth.CreateUser(ctx, "configuration-revoke-tech", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, tech.Username, h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Revoked configuration", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, "DROP TABLE output_definitions"); err != nil {
		t.Fatal(err)
	}
	revoked := false
	handler := New(
		withOperatorConfigurationHooks(h.auth, st, operatorConfigurationHooks{BeforeErrorResponse: func(route string) {
			if route != "configuration-outputs" {
				t.Errorf("error hook route=%q", route)
			}
			revoked = true
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, nil); err != nil {
				t.Fatal(err)
			}
		}}),
		WithProjectRouteQualification(access),
	).Handler()
	request := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/configuration", login.Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !revoked {
		t.Fatalf("storage error hook did not revoke the Project grant: status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) {
		t.Fatalf("revoked configuration read returned status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "CONFIGURATION_UNAVAILABLE") || strings.Contains(response.Body.String(), "no such table") {
		t.Fatalf("storage error escaped after Project grant revocation: %s", response.Body.String())
	}
}
