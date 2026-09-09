package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestOperatorCanDiscardDraftAndRestoreValidatedRevision(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	audit, err := securityaudit.New(h.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	project, validated, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Discard UX", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, validated.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	draft, err := stageStore.EnsureProjectDraft(ctx, project.ID, "owner", "operator edit")
	if err != nil {
		t.Fatal(err)
	}

	credential, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorConfigurationDraft(h.auth, stageStore, audit)).Handler()
	body := bytes.NewBufferString(`{"reason":"cancelled from Operator"}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+project.ID+"/configuration/draft", body)
	req.RemoteAddr = "127.0.0.1:18003"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("discard status=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Discarded bool `json:"discarded"`
		Current   struct {
			ID string `json:"revision_id"`
		} `json:"current_revision"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Discarded || response.Current.ID != validated.ID {
		t.Fatalf("discard response=%+v", response)
	}
	abandoned, err := stageStore.GetRevision(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if abandoned.Status != domain.RevisionSuperseded {
		t.Fatalf("draft status=%s want SUPERSEDED", abandoned.Status)
	}
	records, err := audit.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].EventType != "project.draft.discard" || records[0].Result != securityaudit.ResultSuccess || records[0].ResourceID != project.ID {
		t.Fatalf("audit records=%+v", records)
	}
}

func TestTechnicianCannotDiscardProjectDraft(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	audit, err := securityaudit.New(h.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	project, validated, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Owner-only discard", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, validated.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	draft, err := stageStore.EnsureProjectDraft(ctx, project.ID, "owner", "operator edit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.auth.CreateUser(ctx, "tech", "technician password 123", userauth.RoleTechnician); err != nil {
		t.Fatal(err)
	}
	credential, err := h.auth.Login(ctx, "tech", "technician password 123", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	handler := New(WithOperatorConfigurationDraft(h.auth, stageStore, audit)).Handler()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+project.ID+"/configuration/draft", bytes.NewBufferString(`{"reason":"technician attempt"}`))
	req.RemoteAddr = "127.0.0.2:18004"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, credential.CSRFToken)
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || !bytes.Contains(res.Body.Bytes(), []byte("OWNER_REQUIRED")) {
		t.Fatalf("technician discard status=%d body=%s", res.Code, res.Body.String())
	}
	current, err := stageStore.GetRevision(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.RevisionDraft {
		t.Fatalf("technician changed draft status=%s", current.Status)
	}
	records, err := audit.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].EventType != "project.draft.discard" || records[0].Result != securityaudit.ResultRejected || records[0].ActorUsername != "tech" {
		t.Fatalf("rejection audit=%+v", records)
	}
}
