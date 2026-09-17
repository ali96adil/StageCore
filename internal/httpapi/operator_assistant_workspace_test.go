package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type assistantWorkspaceEvidenceSource struct {
	collection assistant.EvidenceCollection
}

func (s assistantWorkspaceEvidenceSource) Collect(context.Context, assistant.EvidenceQuery) (assistant.EvidenceCollection, error) {
	return s.collection, nil
}

type assistantWorkspaceRedactor struct{}

func (assistantWorkspaceRedactor) RedactString(_ context.Context, value string) string { return value }

type assistantWorkspaceHTTPHarness struct {
	t          *testing.T
	auth       *authHarness
	stageStore *store.Store
	owner      userauth.Credential
	projectID  string
	revisionID string
}

func newAssistantWorkspaceHTTPHarness(t *testing.T) *assistantWorkspaceHTTPHarness {
	t.Helper()
	h := newAuthHarness(t)
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(context.Background(), store.CreateProjectParams{Name: "Assistant Workspace", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(context.Background(), "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return &assistantWorkspaceHTTPHarness{t: t, auth: h, stageStore: stageStore, owner: owner, projectID: project.ID, revisionID: revision.ID}
}

func (h *assistantWorkspaceHTTPHarness) handler(service *assistant.WorkspaceService) http.Handler {
	return New(
		WithAssistantWorkspace(service),
		WithOperatorConfigurationDraft(h.auth, h.stageStore),
	).Handler()
}

func (h *assistantWorkspaceHTTPHarness) request(handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			h.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	req.RemoteAddr = "127.0.0.1:19423"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: h.owner.Token})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set(csrfHeader, h.owner.CSRFToken)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func TestOperatorAssistantStatusStaysAvailableWhenProviderIsOffline(t *testing.T) {
	h := newAssistantWorkspaceHTTPHarness(t)
	res := h.request(h.handler(nil), http.MethodGet, "/api/v1/projects/"+h.projectID+"/assistant/status", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		ProviderState      string          `json:"provider_state"`
		Tasks              map[string]bool `json:"tasks"`
		NormalOperatorMode bool            `json:"normal_operator_mode"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ProviderState != "UNAVAILABLE" || !body.NormalOperatorMode || body.Tasks["EXPLAIN"] || body.Tasks["DIAGNOSE"] || body.Tasks["DRAFT"] {
		t.Fatalf("offline status=%+v", body)
	}
}

func TestOperatorAssistantExplainUsesAuthenticatedGroundedWorkspaceService(t *testing.T) {
	h := newAssistantWorkspaceHTTPHarness(t)
	provider := assistant.FakeProvider{Responses: map[string]assistant.Response{
		"explain-1": {
			ContractVersion: assistant.ContractVersion1,
			RequestID:       "explain-1",
			Authority:       assistant.AuthorityReadOnly,
			Answer:          "Preflight reports a blocker.",
			Evidence:        []assistant.EvidenceRef{{Kind: string(assistant.ContextPreflightFinding), ID: "preflight:blocker"}},
		},
	}}
	service := &assistant.WorkspaceService{
		Provider: provider,
		EvidenceSource: assistantWorkspaceEvidenceSource{collection: assistant.EvidenceCollection{Facts: []assistant.ContextFact{{
			Kind: assistant.ContextPreflightFinding, RefID: "preflight:blocker", Summary: "status=BLOCKER",
		}}}},
		DraftSource: assistant.CanonicalDraftContextSource{Store: h.stageStore},
		Redactor:    assistantWorkspaceRedactor{},
	}
	handler := h.handler(service)
	statusRes := h.request(handler, http.MethodGet, "/api/v1/projects/"+h.projectID+"/assistant/status", nil)
	if statusRes.Code != http.StatusOK || !bytes.Contains(statusRes.Body.Bytes(), []byte(`"EXPLAIN":true`)) || !bytes.Contains(statusRes.Body.Bytes(), []byte(`"DRAFT":true`)) {
		t.Fatalf("status=%d body=%s", statusRes.Code, statusRes.Body.String())
	}
	res := h.request(handler, http.MethodPost, "/api/v1/projects/"+h.projectID+"/assistant/tasks", assistantWorkspaceTaskRequest{
		RequestID: "explain-1", Kind: assistant.RequestExplain, Scope: assistant.EvidenceReadiness, Prompt: "Why is this Project not ready?",
	})
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("Preflight reports a blocker.")) || !bytes.Contains(res.Body.Bytes(), []byte("preflight:blocker")) {
		t.Fatalf("task status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestOperatorAssistantDraftReturnsUnsealedStructuredProposalOnly(t *testing.T) {
	h := newAssistantWorkspaceHTTPHarness(t)
	provider := assistant.FakeProvider{Responses: map[string]assistant.Response{
		"draft-1": {
			ContractVersion: assistant.ContractVersion1,
			RequestID:       "draft-1",
			Authority:       assistant.AuthorityDraftProposal,
			Answer:          "I prepared an operator note.",
			Proposal: &assistant.DraftProposal{
				BaseRevisionID: h.revisionID,
				Operations: []assistant.ProposalOperation{{
					Kind: assistant.ProposalNoteDraft, Summary: "Add focus note", Payload: json.RawMessage(`{"category":"assistant","body":"Check projector focus"}`),
				}},
			},
		},
	}}
	service := &assistant.WorkspaceService{
		Provider:    provider,
		DraftSource: assistant.CanonicalDraftContextSource{Store: h.stageStore},
		Redactor:    assistantWorkspaceRedactor{},
	}
	res := h.request(h.handler(service), http.MethodPost, "/api/v1/projects/"+h.projectID+"/assistant/tasks", assistantWorkspaceTaskRequest{
		RequestID: "draft-1", Kind: assistant.RequestDraft, Prompt: "Add a note to check projector focus",
	})
	if res.Code != http.StatusOK {
		t.Fatalf("draft status=%d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Response assistant.Response `json:"response"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Response.Proposal == nil || len(body.Response.Proposal.Operations) != 1 || body.Response.Proposal.ExpiresAt != nil {
		t.Fatalf("response=%+v", body.Response)
	}
}

func TestOperatorAssistantTaskFailsClosedWhenProviderIsUnavailable(t *testing.T) {
	h := newAssistantWorkspaceHTTPHarness(t)
	res := h.request(h.handler(nil), http.MethodPost, "/api/v1/projects/"+h.projectID+"/assistant/tasks", assistantWorkspaceTaskRequest{
		RequestID: "offline-1", Kind: assistant.RequestDraft, Prompt: "Draft something",
	})
	if res.Code != http.StatusServiceUnavailable || !bytes.Contains(res.Body.Bytes(), []byte("ASSISTANT_PROVIDER_UNAVAILABLE")) {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
