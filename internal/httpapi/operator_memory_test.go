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
	"github.com/ali96adil/StageCore/internal/sessionmemory"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorNotesAndSessionMemoryAPI(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	projectStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Memory API", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := projectStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(projectStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	session, err := projectStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Dress")
	if err != nil {
		t.Fatal(err)
	}
	memory := sessionmemory.New(projectStore)
	handler := New(WithOperatorMemory(h.auth, projectStore, memory)).Handler()

	unauth := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/sessions", nil)
	unauth.RemoteAddr = "127.0.0.1:19001"
	unauthRes := httptest.NewRecorder()
	handler.ServeHTTP(unauthRes, unauth)
	if unauthRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated sessions status=%d, want 401", unauthRes.Code)
	}

	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	listReq := memoryReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/sessions", owner.Token)
	listRes := httptest.NewRecorder()
	handler.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK || !strings.Contains(listRes.Body.String(), session.ID) || !strings.Contains(listRes.Body.String(), `"session_type":"REHEARSAL"`) {
		t.Fatalf("sessions status=%d body=%s", listRes.Code, listRes.Body.String())
	}

	createPayload, _ := json.Marshal(map[string]any{
		"session_id": session.ID, "cue_id": cue.ID, "category": "video", "body": "Check blackout timing",
	})
	createReq := memoryMutationRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/notes", createPayload, owner.Token, owner.CSRFToken)
	createRes := httptest.NewRecorder()
	handler.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create note status=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		Note noteView `json:"note"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Note.NoteID == "" || created.Note.Status != domain.NoteOpen || created.Note.CreatedBy != "owner" {
		t.Fatalf("created note=%+v", created.Note)
	}

	filterPath := "/api/v1/projects/" + project.ID + "/notes?status=OPEN&category=video&session_id=" + session.ID + "&cue_id=" + cue.ID
	filterReq := memoryReadRequest(http.MethodGet, filterPath, owner.Token)
	filterRes := httptest.NewRecorder()
	handler.ServeHTTP(filterRes, filterReq)
	if filterRes.Code != http.StatusOK || !strings.Contains(filterRes.Body.String(), created.Note.NoteID) {
		t.Fatalf("filtered notes status=%d body=%s", filterRes.Code, filterRes.Body.String())
	}

	updatePayload, _ := json.Marshal(map[string]any{"category": "timing", "body": "Check blackout after GO"})
	updateReq := memoryMutationRequest(http.MethodPatch, "/api/v1/projects/"+project.ID+"/notes/"+created.Note.NoteID, updatePayload, owner.Token, owner.CSRFToken)
	updateRes := httptest.NewRecorder()
	handler.ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusOK || !strings.Contains(updateRes.Body.String(), "Check blackout after GO") {
		t.Fatalf("update note status=%d body=%s", updateRes.Code, updateRes.Body.String())
	}

	resolveReq := memoryMutationRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/notes/"+created.Note.NoteID+"/resolve", nil, owner.Token, owner.CSRFToken)
	resolveRes := httptest.NewRecorder()
	handler.ServeHTTP(resolveRes, resolveReq)
	if resolveRes.Code != http.StatusOK || !strings.Contains(resolveRes.Body.String(), `"status":"RESOLVED"`) {
		t.Fatalf("resolve note status=%d body=%s", resolveRes.Code, resolveRes.Body.String())
	}
	reopenReq := memoryMutationRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/notes/"+created.Note.NoteID+"/reopen", nil, owner.Token, owner.CSRFToken)
	reopenRes := httptest.NewRecorder()
	handler.ServeHTTP(reopenRes, reopenReq)
	if reopenRes.Code != http.StatusOK || !strings.Contains(reopenRes.Body.String(), `"status":"OPEN"`) {
		t.Fatalf("reopen note status=%d body=%s", reopenRes.Code, reopenRes.Body.String())
	}

	detailReq := memoryReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/sessions/"+session.ID, owner.Token)
	detailRes := httptest.NewRecorder()
	handler.ServeHTTP(detailRes, detailReq)
	if detailRes.Code != http.StatusOK || !strings.Contains(detailRes.Body.String(), session.ID) || !strings.Contains(detailRes.Body.String(), `"cue_executions":[]`) {
		t.Fatalf("session detail status=%d body=%s", detailRes.Code, detailRes.Body.String())
	}

	var passwordHash string
	if err := h.db.DB.QueryRowContext(ctx, `SELECT password_hash FROM local_users WHERE username = 'owner'`).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.DB.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, enabled, created_at_us, updated_at_us)
		VALUES ('00000000-0000-7000-8000-000000000090', 'viewer-memory', ?, 'VIEWER', 1, 1, 1)`, passwordHash); err != nil {
		t.Fatal(err)
	}
	viewer, err := h.auth.Login(ctx, "viewer-memory", h.password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	viewerRead := memoryReadRequest(http.MethodGet, "/api/v1/projects/"+project.ID+"/notes", viewer.Token)
	viewerReadRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerReadRes, viewerRead)
	if viewerReadRes.Code != http.StatusOK {
		t.Fatalf("viewer note read status=%d", viewerReadRes.Code)
	}
	viewerWrite := memoryMutationRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/notes", createPayload, viewer.Token, viewer.CSRFToken)
	viewerWriteRes := httptest.NewRecorder()
	handler.ServeHTTP(viewerWriteRes, viewerWrite)
	if viewerWriteRes.Code != http.StatusForbidden {
		t.Fatalf("viewer note write status=%d, want 403", viewerWriteRes.Code)
	}
}

func TestNoteWritePermissionRoles(t *testing.T) {
	for _, role := range []string{"OWNER", "TECHNICIAN", "OPERATOR"} {
		if err := userauth.Authorize(role, userauth.PermissionNoteWrite); err != nil {
			t.Fatalf("role %s note write denied: %v", role, err)
		}
	}
	if err := userauth.Authorize("VIEWER", userauth.PermissionNoteWrite); err == nil {
		t.Fatal("VIEWER unexpectedly has note write permission")
	}
}

func memoryReadRequest(method, path, token string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:19002"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
	return req
}

func memoryMutationRequest(method, path string, body []byte, token, csrf string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:19003"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: token})
	req.Header.Set(csrfHeader, csrf)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}
