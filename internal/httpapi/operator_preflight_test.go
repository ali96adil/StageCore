package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestAuthenticatedPreflightReportAndShowUseSameGate(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Preflight Web", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(projectStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}

	registry := capability.NewRegistry()
	root := t.TempDir()
	policy := storagehealth.NewPolicyWithProbe(1, 10, func(string) (storagehealth.Filesystem, error) {
		return storagehealth.Filesystem{TotalBytes: 1_000_000, FreeBytes: 900_000}, nil
	})
	monitor := storagehealth.NewMonitor(policy, root, root)
	preflightService := preflight.New(projectStore, registry, monitor)
	runtime := runtimecontrol.New(projectStore, registry, runtimecontrol.WithShowGate(preflightService.ShowGate))
	handler := New(
		WithOperatorPreflight(h.auth, preflightService),
		WithOperatorRuntime(h.auth, projectStore, runtime),
	).Handler()

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/preflight", nil)
	unauthenticated.RemoteAddr = "127.0.0.1:18001"
	unauthenticatedRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRes, unauthenticated)
	if unauthenticatedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated Preflight status=%d, want 401", unauthenticatedRes.Code)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	preflightReq := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/preflight", owner.Token)
	preflightRes := httptest.NewRecorder()
	handler.ServeHTTP(preflightRes, preflightReq)
	if preflightRes.Code != http.StatusOK {
		t.Fatalf("Preflight status=%d body=%s", preflightRes.Code, preflightRes.Body.String())
	}
	if body := preflightRes.Body.String(); !containsAll(body, `"status":"PASS"`, published.ID, `"storage"`) {
		t.Fatalf("unexpected Preflight payload: %s", body)
	}

	other, _, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Other project"})
	if err != nil {
		t.Fatal(err)
	}
	for _, snapshotID := range []string{published.ID, "unknown"} {
		req := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+other.ID+"/preflight?runtime_snapshot_id="+snapshotID, owner.Token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusNotFound || strings.Contains(res.Body.String(), published.ID) {
			t.Fatalf("foreign or unknown Snapshot disclosed details: %d %s", res.Code, res.Body.String())
		}
	}

	showBody := []byte(`{"mode":"SHOW","name":"Operator Show","request_id":"00000000-0000-7000-8000-000000000701"}`)
	showReq := authenticatedMutationRequest(t, http.MethodPost, "/api/v1/projects/"+project.ID+"/runtime/start", showBody, owner.Token, owner.CSRFToken)
	showRes := httptest.NewRecorder()
	handler.ServeHTTP(showRes, showReq)
	if showRes.Code != http.StatusCreated {
		t.Fatalf("SHOW start status=%d body=%s", showRes.Code, showRes.Body.String())
	}
	active, err := projectStore.ActiveSessionForProject(ctx, project.ID)
	if err != nil || active == nil || active.Type != domain.SessionShow {
		t.Fatalf("SHOW did not become active: session=%+v err=%v", active, err)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func TestPreflightOperatorUXLinksBlockingChecksToRelevantWorkspace(t *testing.T) {
	handler := New(WithOperatorWeb()).Handler()
	req := httptest.NewRequest(http.MethodGet, "/preflight.js", nil)
	req.RemoteAddr = "127.0.0.1:18002"
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("preflight.js status=%d body=%s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	for _, required := range []string{
		"preflightDestination",
		"Open Stage Devices",
		"Open Network Cockpit",
		"Open Live Video",
		"Open Machine Roles",
		"Open Environments",
		"Open Cues / Publish",
		"bindPreflightFixButtons",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("preflight UX missing %q", required)
		}
	}
}

type preflightEvaluatorFunc func(context.Context, string, string) (preflight.Report, error)

func (f preflightEvaluatorFunc) Evaluate(ctx context.Context, projectID, snapshotID string) (preflight.Report, error) {
	return f(ctx, projectID, snapshotID)
}

func TestQualifiedPreflightFencesReportAndRedactsMediaWithoutGrant(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	st := store.New(h.db.DB, clock.Real{})
	access, err := projectaccess.New(h.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	project, _, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "preflight scope"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	user, err := h.auth.CreateUser(ctx, "preflight viewer", h.password, userauth.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.Login(ctx, user.Username, h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	set := func(permissions ...projectaccess.Permission) {
		t.Helper()
		if err := access.SetPermissions(ctx, owner.Session.User.ID, user.ID, project.ID, permissions); err != nil {
			t.Fatal(err)
		}
	}
	report := preflight.Report{
		Status: preflight.Warn, ProjectID: project.ID,
		Checks: []preflight.Check{
			{Category: "media", Detail: "sensitive hash"},
			{Category: "clock", Detail: "clock warning"},
		},
		Media: []preflight.MediaStatus{{ContentHash: "sensitive hash"}},
	}
	revoke := false
	evaluate := preflightEvaluatorFunc(func(context.Context, string, string) (preflight.Report, error) {
		if revoke {
			set()
		}
		return report, nil
	})
	handler := New(WithProjectRouteQualification(access), WithOperatorPreflight(h.auth, evaluate)).Handler()
	read := func() *httptest.ResponseRecorder {
		req := authenticatedReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/preflight", login.Token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	set(projectaccess.ProjectView)
	if res := read(); res.Code != 200 || strings.Contains(res.Body.String(), "sensitive hash") || !strings.Contains(res.Body.String(), "clock warning") {
		t.Fatalf("media redaction failed: %d %s", res.Code, res.Body.String())
	}
	set(projectaccess.ProjectView, projectaccess.MediaRead)
	if res := read(); res.Code != 200 || !strings.Contains(res.Body.String(), "sensitive hash") {
		t.Fatalf("authorized media absent: %d %s", res.Code, res.Body.String())
	}
	revoke = true
	if res := read(); res.Code != 404 || strings.Contains(res.Body.String(), "clock warning") {
		t.Fatalf("revoked report escaped: %d %s", res.Code, res.Body.String())
	}
}
