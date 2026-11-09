package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestQualifiedPluginAdministrationRequiresLiveOwner(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "A show"})
	if err != nil {
		t.Fatal(err)
	}
	objects, err := vault.Open(t.TempDir(), projects)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(objects, projects, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(projects, repository)
	if err != nil {
		t.Fatal(err)
	}
	technician, err := h.auth.CreateUser(ctx, "plugin-tech", h.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	tech, err := h.auth.Login(ctx, technician.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, technician.ID, project.ID,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}); err != nil {
		t.Fatal(err)
	}

	option := WithOperatorExtensionLibrary(h.auth, library, nil)
	legacy := New(option).Handler()
	scoped := New(option, WithOwnerPluginAdministration(h.auth)).Handler()
	request := func(handler http.Handler, credential userauth.Credential, method, path string, want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.RemoteAddr = "127.0.0.1:41000"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
		req.Header.Set(csrfHeader, credential.CSRFToken)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s returned %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
	}
	request(legacy, tech, "GET", "/api/v1/extensions", 200)
	request(scoped, tech, "GET", "/api/v1/extensions", 403)
	request(scoped, tech, "HEAD", "/api/v1/%65xtensions", 403)
	request(scoped, tech, "POST", "/api/v1/extensions/register", 403)
	request(scoped, owner, "GET", "/api/v1/extensions", 200)
	request(scoped, tech, "GET", "/health/live", 200)
	if err := h.auth.Logout(ctx, owner.Token); err != nil {
		t.Fatal(err)
	}
	request(scoped, owner, "GET", "/api/v1/extensions", 401)
}

func TestSC024AllRegisteredPluginNamespacesUseQualifiedGuard(t *testing.T) {
	matched := 0
	for _, route := range collectHTTPRouteRegistrations(t) {
		if !strings.HasPrefix(route.Source, "operator_extension") && route.Source != "security_operations.go" {
			continue
		}
		if route.Source == "security_operations.go" && !strings.Contains(route.Pattern, "/security/plugins") {
			continue
		}
		if !isHubPluginAdminPattern(route.Pattern) {
			t.Errorf("unqualified plugin route %s %s", route.Source, route.Pattern)
		}
		matched++
	}
	if matched != 28 {
		t.Fatalf("plugin inventory changed: %d", matched)
	}
}
