package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorProjectCreateListOpenAndRoleDenial(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorProjects(h.auth, projectStore)).Handler()

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	createBody, _ := json.Marshal(map[string]string{"name": "Blindness", "description": "Main stage project"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(createBody))
	createReq.RemoteAddr = "127.0.0.1:10001"
	createReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	createReq.Header.Set(csrfHeader, owner.CSRFToken)
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		Project       projectView  `json:"project"`
		DraftRevision revisionView `json:"draft_revision"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Project.Name != "Blindness" || created.DraftRevision.Status != domain.RevisionDraft {
		t.Fatalf("unexpected create payload: %+v %+v", created.Project, created.DraftRevision)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	listReq.RemoteAddr = "127.0.0.1:10002"
	listReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), "Blindness") {
		t.Fatalf("list response=%d %s", listRes.Code, listRes.Body.String())
	}

	openReq := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+created.Project.ID, nil)
	openReq.RemoteAddr = "127.0.0.1:10003"
	openReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
	openRes := httptest.NewRecorder()
	handler.ServeHTTP(openRes, openReq)
	if openRes.Code != http.StatusOK || !strings.Contains(openRes.Body.String(), created.DraftRevision.ID) {
		t.Fatalf("open response=%d %s", openRes.Code, openRes.Body.String())
	}

	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000003', 'viewer2', ?, 'VIEWER', 1, 1, 1)
	`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "viewer2", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerReq := httptest.NewRequest(http.MethodPost, "/api/v1/projects", bytes.NewReader(createBody))
	viewerReq.RemoteAddr = "127.0.0.1:10004"
	viewerReq.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: viewer.Token})
	viewerReq.Header.Set(csrfHeader, viewer.CSRFToken)
	viewerRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerRes, viewerReq)
	if viewerRes.Code != http.StatusForbidden {
		t.Fatalf("VIEWER create status=%d, want 403", viewerRes.Code)
	}
}

func TestDashboardDraftPublishedAndActiveCueState(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	handler := New(WithOperatorProjects(h.auth, projectStore)).Handler()
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Dashboard Test", CreatedBy: owner.Session.User.ID})
	if err != nil {
		t.Fatal(err)
	}

	requestDashboard := func() dashboardView {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/dashboard", nil)
		req.RemoteAddr = "127.0.0.1:12000"
		req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: owner.Token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("dashboard status=%d body=%s", res.Code, res.Body.String())
		}
		var view dashboardView
		if err := json.Unmarshal(res.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}

	initial := requestDashboard()
	if initial.PublicationState != "NOT_PUBLISHED" || !initial.UnpublishedChanges || initial.Mode != "EDIT" {
		t.Fatalf("unexpected initial dashboard: %+v", initial)
	}

	first, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1,
		Enabled: true, Criticality: "NORMAL",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Second", OrderIndex: 2,
		Enabled: true, Criticality: "NORMAL",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshot, err := projectStore.CreateRuntimeSnapshot(ctx, revision.ID, owner.Session.User.ID, strings.Repeat("a", 64), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	session, err := projectStore.CreateSession(ctx, snapshot.ID, domain.SessionRehearsal, "Rehearsal 1")
	if err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetSessionCurrentCue(ctx, session.ID, first.ID); err != nil {
		t.Fatal(err)
	}

	active := requestDashboard()
	if active.PublicationState != "PUBLISHED" || active.UnpublishedChanges {
		t.Fatalf("unexpected publication state: %+v", active)
	}
	if active.Mode != "REHEARSAL" || active.ActiveSession == nil || active.ActiveSession.ID != session.ID {
		t.Fatalf("unexpected active session: %+v", active)
	}
	if active.CurrentCue == nil || active.CurrentCue.ID != first.ID {
		t.Fatalf("current cue=%+v, want %s", active.CurrentCue, first.ID)
	}
	if active.NextCue == nil || active.NextCue.ID != second.ID {
		t.Fatalf("next cue=%+v, want %s", active.NextCue, second.ID)
	}
}
