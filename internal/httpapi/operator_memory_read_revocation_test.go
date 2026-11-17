package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestSessionMemoryAndNoteReadsSuppressStoreErrorsAfterGrantRevocation(t *testing.T) {
	cases := []struct {
		name, path, table, hookName string
	}{
		{"sessions", "sessions", "sessions", "project-sessions"},
		{"session-detail", "sessions/missing-session", "sessions", "project-session-detail"},
		{"notes", "notes", "operator_notes", "project-notes"},
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
			tech, err := h.auth.CreateUser(ctx, "memory-read-"+tc.name, h.password, userauth.RoleTechnician)
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
			project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Session Memory read fence", CreatedBy: owner.Session.User.ID})
			if err != nil {
				t.Fatal(err)
			}
			if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID,
				[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.db.DB.ExecContext(ctx, "DROP TABLE "+tc.table); err != nil {
				t.Fatal(err)
			}
			revoked := false
			handler := New(
				withOperatorMemoryHooks(h.auth, st, sessionmemory.New(st), operatorMemoryHooks{BeforeErrorResponse: func(route string) {
					if route != tc.hookName {
						t.Errorf("error hook route=%q want=%q", route, tc.hookName)
					}
					revoked = true
					if err := access.SetPermissions(ctx, owner.Session.User.ID, tech.ID, project.ID, nil); err != nil {
						t.Fatal(err)
					}
				}}),
				WithProjectRouteQualification(access),
			).Handler()
			path := "/api/v1/projects/" + project.ID + "/" + tc.path
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedReadRequest(http.MethodGet, path, login.Token))
			if !revoked {
				t.Fatalf("store error hook did not revoke grant: status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"error_code":"PROJECT_NOT_FOUND"`) {
				t.Fatalf("revoked memory read returned status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "no such table") {
				t.Fatalf("store error escaped after grant revocation: %s", response.Body.String())
			}
		})
	}
}
