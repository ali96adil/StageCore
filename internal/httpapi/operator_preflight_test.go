package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
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
