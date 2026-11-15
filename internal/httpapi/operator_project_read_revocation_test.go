package httpapi

import (
	"context"
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

func TestProjectReadsSuppressStoreErrorsAfterProjectGrantRevocation(t *testing.T) {
	cases := []struct {
		name, path, table, hookName string
	}{
		{"detail-revision", "{project_id}", "project_revisions", "project-read"},
		{"dashboard-snapshot", "{project_id}/dashboard", "runtime_snapshots", "project-dashboard"},
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
			tech, err := h.auth.CreateUser(ctx, "project-read-"+tc.name, h.password, userauth.RoleTechnician)
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
			project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Project read fence", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
				[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
				t.Fatal(err)
			}
			if tc.table == "project_revisions" {
				conn, err := h.db.DB.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
					_ = conn.Close()
					t.Fatal(err)
				}
				_, dropErr := conn.ExecContext(ctx, "DROP TABLE "+tc.table)
				_, pragmaErr := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON")
				_ = conn.Close()
				if dropErr != nil {
					t.Fatal(dropErr)
				}
				if pragmaErr != nil {
					t.Fatal(pragmaErr)
				}
			} else if _, err := h.db.DB.ExecContext(ctx, "DROP TABLE "+tc.table); err != nil {
				t.Fatal(err)
			}
			revoked := false
			revokeAtError := func(route string) {
				if route != tc.hookName {
					t.Errorf("error hook route=%q want=%q", route, tc.hookName)
				}
				revoked = true
				if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, nil); err != nil {
					t.Fatal(err)
				}
				if err := access.CheckAll(ctx, tech.ID, project.ID, projectaccess.ProjectView); !errors.Is(err, projectaccess.ErrDenied) {
					t.Errorf("Project grant remained active after revocation: %v", err)
				}
			}
			handler := New(
				withOperatorProjectsHooks(h.auth, st, access, operatorProjectHooks{BeforeErrorResponse: revokeAtError}),
				WithProjectRouteQualification(access),
			).Handler()
			path := "/api/v1/projects/" + strings.ReplaceAll(tc.path, "{project_id}", project.ID)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, path, login.Token))
			if !revoked {
				t.Fatalf("storage error hook did not revoke the Project grant: status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) {
				t.Fatalf("revoked Project read returned status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "UNAVAILABLE") {
				t.Fatalf("storage error escaped after Project grant revocation: %s", response.Body.String())
			}
		})
	}
}
