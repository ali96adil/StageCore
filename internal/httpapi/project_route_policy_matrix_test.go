package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestQualifiedProjectRoutePermissionMatrix(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "qualified-policy-matrix", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Qualified Policy Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "Unrelated Project"})
	if err != nil {
		t.Fatal(err)
	}

	registerPolicyProbes := func(server *Server) {
		for pattern := range projectRoutePermissions {
			server.mux.HandleFunc(pattern, withAuthenticatedSession(h.auth, isUnsafeMethod(methodFromPattern(pattern)),
				func(w http.ResponseWriter, r *http.Request, session userauth.Session, token string) {
					if _, ok := qualifyProjectRoute(w, r, h.auth, session, token); !ok {
						return
					}
					w.WriteHeader(http.StatusNoContent)
				}))
		}
	}
	handler := New(registerPolicyProbes, WithProjectRouteQualification(access)).Handler()

	routes := make([]string, 0, len(projectRoutePermissions))
	for pattern := range projectRoutePermissions {
		routes = append(routes, pattern)
	}
	sort.Strings(routes)
	if len(routes) == 0 {
		t.Fatal("project route permission map is empty")
	}
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := authenticatedExecutionEnvironmentRequest(t, login.Token, login.CSRFToken, method, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	withViewMembership := func(permissions []projectaccess.Permission) []projectaccess.Permission {
		grants := append([]projectaccess.Permission(nil), permissions...)
		for _, permission := range grants {
			if permission == projectaccess.ProjectView {
				return grants
			}
		}
		return append(grants, projectaccess.ProjectView)
	}
	concretePath := func(pattern string, projectID string) string {
		_, path, ok := strings.Cut(pattern, " ")
		if !ok {
			t.Fatalf("invalid permission-map route pattern %q", pattern)
		}
		for {
			start := strings.IndexByte(path, '{')
			if start < 0 {
				return path
			}
			end := strings.IndexByte(path[start:], '}')
			if end < 0 {
				t.Fatalf("unterminated path parameter in %q", pattern)
			}
			name := path[start+1 : start+end]
			value := "qualification-" + name
			if name == "project_id" {
				value = projectID
			}
			path = path[:start] + value + path[start+end+1:]
		}
	}

	for _, pattern := range routes {
		method, _, ok := strings.Cut(pattern, " ")
		if !ok {
			t.Fatalf("invalid permission-map route pattern %q", pattern)
		}
		permissions := projectRoutePermissions[pattern]
		if len(permissions) == 0 {
			t.Fatalf("route %q has no explicit Project permission", pattern)
		}
		path := concretePath(pattern, project.ID)
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, withViewMembership(permissions)); err != nil {
			t.Fatalf("grant %v for %s: %v", permissions, pattern, err)
		}
		if response := request(method, path); response.Code != http.StatusNoContent {
			t.Errorf("exact grant %v did not authorize %s %s: HTTP %d %s", permissions, method, path, response.Code, response.Body.String())
		}

		partial := withViewMembership(permissions[:len(permissions)-1])
		if permissions[len(permissions)-1] == projectaccess.ProjectView {
			partial = nil
		}
		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, partial); err != nil {
			t.Fatalf("reduce grant for %s: %v", pattern, err)
		}
		if response := request(method, path); response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "PROJECT_NOT_FOUND") {
			t.Errorf("partial grant %v leaked %s %s: HTTP %d %s", partial, method, path, response.Code, response.Body.String())
		}

		if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, withViewMembership(permissions)); err != nil {
			t.Fatalf("restore grant for cross-Project check %s: %v", pattern, err)
		}
		foreignPath := concretePath(pattern, foreign.ID)
		if response := request(method, foreignPath); response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "PROJECT_NOT_FOUND") {
			t.Errorf("Project grant crossed scope for %s %s: HTTP %d %s", method, foreignPath, response.Code, response.Body.String())
		}
	}
}

func methodFromPattern(pattern string) string {
	method, _, _ := strings.Cut(pattern, " ")
	return method
}
