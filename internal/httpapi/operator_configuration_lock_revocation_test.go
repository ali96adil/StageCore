package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestConfigurationLockStoreErrorsSuppressAfterGrantRevocation(t *testing.T) {
	cases := []struct {
		name, method, hook string
		permission         projectaccess.Permission
	}{
		{name: "read", method: http.MethodGet, permission: projectaccess.ProjectView, hook: "configuration-lock-read"},
		{name: "create-draft", method: http.MethodPost, permission: projectaccess.ProjectEdit, hook: "configuration-draft-lock"},
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
			operator, err := h.auth.CreateUser(ctx, "lock-revocation-"+tc.name, h.password, userauth.RoleTechnician)
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.2")
			if err != nil {
				t.Fatal(err)
			}
			project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Configuration lock revocation", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			access, err := projectaccess.New(h.db.DB)
			if err != nil {
				t.Fatal(err)
			}
			permissions := []projectaccess.Permission{tc.permission}
			if tc.permission == projectaccess.ProjectEdit {
				permissions = append(permissions, projectaccess.ProjectView)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, permissions); err != nil {
				t.Fatal(err)
			}
			if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE sessions RENAME COLUMN session_type TO session_type_fault"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := h.db.DB.ExecContext(ctx, "ALTER TABLE sessions RENAME COLUMN session_type_fault TO session_type"); err != nil {
					t.Errorf("restore sessions schema: %v", err)
				}
			}()

			revoked := false
			hooks := operatorConfigurationDraftHooks{BeforeErrorResponse: func(route string) {
				if route != tc.hook {
					t.Errorf("error hook route=%q want=%q", route, tc.hook)
				}
				revoked = true
				if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
					t.Errorf("revoke project grant: %v", err)
				}
			}}
			handler := New(
				withOperatorConfigurationDraft(h.auth, st, hooks),
				WithProjectRouteQualification(access),
			).Handler()
			path := "/api/v1/projects/" + project.ID + "/configuration/lock"
			if tc.method == http.MethodPost {
				path = "/api/v1/projects/" + project.ID + "/configuration/draft"
			}
			request := authenticatedReadRequest(tc.method, path, login.Token)
			if tc.method != http.MethodGet {
				request.Header.Set(csrfHeader, login.CSRFToken)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !revoked {
				t.Fatalf("store error hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) {
				t.Fatalf("revoked configuration response status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "session_type") || strings.Contains(response.Body.String(), "no such column") {
				t.Fatalf("store details escaped after revocation: %s", response.Body.String())
			}
		})
	}
}
