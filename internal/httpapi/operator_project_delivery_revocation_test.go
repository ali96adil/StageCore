package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOperatorProjectReadsSuppressResultsAfterSessionRevocation(t *testing.T) {
	for _, route := range []struct {
		name   string
		method string
		path   func(string) string
	}{
		{name: "list", method: http.MethodGet, path: func(string) string { return "/api/v1/projects" }},
		{name: "detail", method: http.MethodGet, path: func(id string) string { return "/api/v1/projects/" + id }},
		{name: "dashboard", method: http.MethodGet, path: func(id string) string { return "/api/v1/projects/" + id + "/dashboard" }},
	} {
		t.Run(route.name, func(t *testing.T) {
			h := newAuthHarness(t)
			ctx := context.Background()
			projects := store.New(h.db.DB, clock.Real{})
			project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "private-project-name"})
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			handler := New(withOperatorProjectsHooks(h.auth, projects, nil, operatorProjectHooks{BeforeResponse: func() {
				if err := h.auth.Logout(ctx, login.Token); err != nil {
					t.Errorf("revoke Owner session: %v", err)
				}
			}})).Handler()
			request := httptest.NewRequest(route.method, route.path(project.ID), nil)
			request.RemoteAddr = "127.0.0.1:19201"
			request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || strings.Contains(response.Body.String(), project.ID) || strings.Contains(response.Body.String(), project.Name) {
				t.Fatalf("revoked session received project data: HTTP %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestOperatorProjectDetailSuppressesResultAfterProjectGrantRevocation(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "scoped-private-project"})
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "project-delivery-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithProjectRouteQualification(access),
		withOperatorProjectsHooks(h.auth, projects, access, operatorProjectHooks{BeforeResponse: func() {
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Errorf("revoke project view grant: %v", err)
			}
		}}),
	).Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID, nil)
	request.RemoteAddr = "127.0.0.1:19202"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), project.Name) || strings.Contains(response.Body.String(), project.ID) {
		t.Fatalf("revoked project grant received detail: HTTP %d %s", response.Code, response.Body.String())
	}
}

func TestOperatorProjectListRechecksGrantsAtDelivery(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projects := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "revoked-list-project"})
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.CreateUser(ctx, "project-list-operator", h.password, userauth.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, []projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, operator.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(
		WithProjectRouteQualification(access),
		withOperatorProjectsHooks(h.auth, projects, access, operatorProjectHooks{BeforeResponse: func() {
			if err := access.SetPermissions(ctx, owner.Session.User.ID, operator.ID, project.ID, nil); err != nil {
				t.Errorf("revoke project view grant: %v", err)
			}
		}}),
	).Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	request.RemoteAddr = "127.0.0.1:19203"
	request.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), project.Name) || strings.Contains(response.Body.String(), project.ID) {
		t.Fatalf("revoked project grant remained in list: HTTP %d %s", response.Code, response.Body.String())
	}
}

func TestOperatorProjectMutationsReportCommittedResultAfterSessionRevocation(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		method string
		path   func(string) string
		body   string
	}{
		{name: "create", method: http.MethodPost, path: func(string) string { return "/api/v1/projects" }, body: `{"name":"created-before-revocation"}`},
		{name: "duplicate", method: http.MethodPost, path: func(id string) string { return "/api/v1/projects/" + id + "/duplicate" }, body: `{"name":"duplicate-before-revocation"}`},
		{name: "rename", method: http.MethodPatch, path: func(id string) string { return "/api/v1/projects/" + id }, body: `{"name":"renamed-before-revocation"}`},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			h := newAuthHarness(t)
			ctx := context.Background()
			projects := store.New(h.db.DB, clock.Real{})
			project, _, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "original-project"})
			if err != nil {
				t.Fatal(err)
			}
			login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			handler := New(withOperatorProjectsHooks(h.auth, projects, nil, operatorProjectHooks{BeforeResponse: func() {
				if err := h.auth.Logout(ctx, login.Token); err != nil {
					t.Errorf("revoke Owner session: %v", err)
				}
			}})).Handler()
			request := authenticatedMutationRequest(t, mutation.method, mutation.path(project.ID), []byte(mutation.body), login.Token, login.CSRFToken)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("parse revocation response: %v body=%s", err, response.Body.String())
			}
			if response.Code != http.StatusForbidden || result["effect_may_be_committed"] != true || result["effect_may_have_been_committed"] != true || result["retryable"] != false {
				t.Fatalf("revoked Owner received mutation result or retryable response: HTTP %d %+v", response.Code, result)
			}
			items, err := projects.ListProjects(ctx)
			if err != nil {
				t.Fatal(err)
			}
			containsName := func(name string) bool {
				for _, item := range items {
					if item.Name == name {
						return true
					}
				}
				return false
			}
			switch mutation.name {
			case "create":
				if len(items) != 2 || !containsName("created-before-revocation") {
					t.Fatalf("committed project create was lost: %+v", items)
				}
			case "duplicate":
				if len(items) != 2 || !containsName("duplicate-before-revocation") {
					t.Fatalf("committed project duplicate was lost: %+v", items)
				}
			case "rename":
				stored, err := projects.GetProject(ctx, project.ID)
				if err != nil || stored.Name != "renamed-before-revocation" {
					t.Fatalf("committed project rename was lost: project=%+v err=%v", stored, err)
				}
			}
		})
	}
}

func TestOperatorConfigurationDraftSuppressesResultsAfterSessionRevocation(t *testing.T) {
	for _, operation := range []string{"ensure", "discard"} {
		t.Run(operation, func(t *testing.T) {
			h := newAuthHarness(t)
			ctx := context.Background()
			projects := store.New(h.db.DB, clock.Real{})
			project, revision, err := projects.CreateProject(ctx, store.CreateProjectParams{Name: "draft-revocation"})
			if err != nil {
				t.Fatal(err)
			}
			if err := projects.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
				t.Fatal(err)
			}
			if operation == "discard" {
				if _, err := projects.EnsureProjectDraft(ctx, project.ID, "owner", "prepare discard test"); err != nil {
					t.Fatal(err)
				}
			}
			login, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			handler := New(withOperatorConfigurationDraft(h.auth, projects, operatorConfigurationDraftHooks{BeforeResponse: func() {
				if err := h.auth.Logout(ctx, login.Token); err != nil {
					t.Errorf("revoke Owner session: %v", err)
				}
			}})).Handler()
			method, body := http.MethodPost, ""
			if operation == "discard" {
				method, body = http.MethodDelete, `{"reason":"discard before revocation"}`
			}
			request := authenticatedMutationRequest(t, method, "/api/v1/projects/"+project.ID+"/configuration/draft", []byte(body), login.Token, login.CSRFToken)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var result map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("parse revocation response: %v body=%s", err, response.Body.String())
			}
			if response.Code != http.StatusForbidden || result["effect_may_be_committed"] != true || result["effect_may_have_been_committed"] != true || result["retryable"] != false {
				t.Fatalf("revoked Owner received draft result or retryable response: HTTP %d %+v", response.Code, result)
			}
			stored, err := projects.GetProject(ctx, project.ID)
			if err != nil {
				t.Fatal(err)
			}
			if operation == "ensure" && stored.CurrentRevisionID == revision.ID {
				t.Fatal("draft creation did not commit before response suppression")
			}
			if operation == "discard" && stored.CurrentRevisionID != revision.ID {
				t.Fatalf("draft discard did not restore the validated revision: %+v", stored)
			}
		})
	}
}
