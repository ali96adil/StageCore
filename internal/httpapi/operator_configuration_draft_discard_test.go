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
	"github.com/ali96adil/StageCore/internal/store"
)

func TestOperatorCanDiscardDraftAndRestoreValidatedRevision(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
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
	handler := New(WithOperatorConfigurationDraft(h.auth, stageStore)).Handler()
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
}
