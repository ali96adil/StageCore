package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

func TestAssistantPreviewDoesNotReturnSealedProposalAfterProjectGrantRevocation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	access, err := projectaccess.New(h.auth.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.auth.CreateUser(ctx, "assistant-revoked-preview", h.auth.password, userauth.RoleTechnician)
	if err != nil {
		t.Fatal(err)
	}
	login, err := h.auth.auth.Login(ctx, operator.Username, h.auth.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := access.SetPermissions(ctx, h.owner.Session.User.ID, operator.ID, h.project.ID, []projectaccess.Permission{projectaccess.ProjectView, projectaccess.ProjectEdit}); err != nil {
		t.Fatal(err)
	}
	handler := New(WithProjectRouteQualification(access), WithOperatorConfigurationDraft(h.auth.auth, h.stageStore, h.audit)).Handler()
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalNoteDraft, "Preview private note", `{"category":"assistant","body":"Secret note"}`)
	payload, err := json.Marshal(assistantProposalRequest{Proposal: proposal})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/projects/" + h.project.ID + "/assistant/proposals/preview"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(payload)))
	req.RemoteAddr = "127.0.0.1:19031"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: login.Token})
	req.Header.Set(csrfHeader, login.CSRFToken)
	req.Header.Set("Content-Type", "application/json")
	revoked := false
	req.Body = io.NopCloser(revokeDuringRead{Reader: strings.NewReader(string(payload)), revoke: func() {
		if revoked {
			return
		}
		revoked = true
		if err := access.SetPermissions(ctx, h.owner.Session.User.ID, operator.ID, h.project.ID, nil); err != nil {
			t.Fatal(err)
		}
	}})
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if !revoked || res.Code != http.StatusNotFound || strings.Contains(res.Body.String(), "apply_required") ||
		strings.Contains(res.Body.String(), "Secret note") {
		t.Fatalf("revoked preview: revoked=%v status=%d body=%s", revoked, res.Code, res.Body.String())
	}
}

func revokeAssistantSessionAfterSuccessfulProposalAudit(t *testing.T, h *assistantProposalHTTPHarness) {
	t.Helper()
	_, err := h.auth.db.DB.ExecContext(context.Background(), `
		CREATE TRIGGER revoke_assistant_session_after_proposal_audit
		AFTER INSERT ON security_audit_records
		WHEN NEW.event_type = 'assistant.proposal.apply' AND NEW.result = 'SUCCESS'
		BEGIN
			UPDATE browser_sessions
			SET revoked_at_us = NEW.occurred_at_us
			WHERE user_id = NEW.actor_user_id AND revoked_at_us IS NULL;
		END`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAssistantProposalApplySuppressesResultAfterPostCommitRevocation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalNoteDraft, "Add private note", `{"category":"assistant","body":"private assistant note"}`)
	sealed := h.preview(h.owner, proposal)
	revokeAssistantSessionAfterSuccessfulProposalAudit(t, h)

	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(res.Body.String(), `"retryable":false`) || strings.Contains(res.Body.String(), `"applied":true`) ||
		strings.Contains(res.Body.String(), "private assistant note") {
		t.Fatalf("revoked proposal response disclosed its result or encouraged replay: status=%d body=%s", res.Code, res.Body.String())
	}
	notes, err := h.stageStore.ListNotes(context.Background(), h.project.ID, store.NoteFilter{})
	if err != nil || len(notes) != 1 || notes[0].Body != "private assistant note" {
		t.Fatalf("proposal should have committed before response revocation: notes=%+v err=%v", notes, err)
	}
}

func TestAssistantProposalBatchSuppressesResultsAfterPostCommitRevocation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalNoteDraft, Summary: "Add first note", Payload: json.RawMessage(`{"body":"private first note"}`)},
			{Kind: assistant.ProposalNoteDraft, Summary: "Add second note", Payload: json.RawMessage(`{"body":"private second note"}`)},
		},
	}
	sealed := h.preview(h.owner, proposal)
	revokeAssistantSessionAfterSuccessfulProposalAudit(t, h)

	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply-batch", assistantProposalBatchApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), `"effect_may_be_committed":true`) ||
		!strings.Contains(res.Body.String(), `"retryable":false`) || strings.Contains(res.Body.String(), `"results"`) ||
		strings.Contains(res.Body.String(), "private first note") || strings.Contains(res.Body.String(), "private second note") {
		t.Fatalf("revoked batch response disclosed results or encouraged replay: status=%d body=%s", res.Code, res.Body.String())
	}
	notes, err := h.stageStore.ListNotes(context.Background(), h.project.ID, store.NoteFilter{})
	if err != nil || len(notes) != 2 {
		t.Fatalf("proposal batch should have committed before response revocation: notes=%+v err=%v", notes, err)
	}
}
