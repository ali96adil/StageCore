package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type assistantProposalHTTPHarness struct {
	t          *testing.T
	auth       *authHarness
	stageStore *store.Store
	audit      *securityaudit.Service
	handler    http.Handler
	owner      userauth.Credential
	project    domain.Project
	revision   domain.ProjectRevision
}

func newAssistantProposalHTTPHarness(t *testing.T) *assistantProposalHTTPHarness {
	t.Helper()
	h := newAuthHarness(t)
	ctx := context.Background()
	stageStore := store.New(h.db.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Assistant Proposal", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	audit, err := securityaudit.New(h.db.DB, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := h.auth.Login(ctx, "owner", h.password, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return &assistantProposalHTTPHarness{
		t: t, auth: h, stageStore: stageStore, audit: audit,
		handler: New(WithOperatorConfigurationDraft(h.auth, stageStore, audit)).Handler(),
		owner: owner, project: project, revision: revision,
	}
}

func (h *assistantProposalHTTPHarness) request(credential userauth.Credential, method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			h.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &payload)
	req.RemoteAddr = "127.0.0.1:19023"
	req.AddCookie(&http.Cookie{Name: browserSessionCookie, Value: credential.Token})
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set(csrfHeader, credential.CSRFToken)
	}
	res := httptest.NewRecorder()
	h.handler.ServeHTTP(res, req)
	return res
}

func (h *assistantProposalHTTPHarness) preview(credential userauth.Credential, proposal assistant.DraftProposal) assistant.DraftProposal {
	h.t.Helper()
	res := h.request(credential, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/preview", assistantProposalRequest{Proposal: proposal})
	if res.Code != http.StatusOK {
		h.t.Fatalf("preview status=%d body=%s", res.Code, res.Body.String())
	}
	var body struct {
		Proposal      assistant.DraftProposal `json:"proposal"`
		ApplyRequired bool                    `json:"apply_required"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		h.t.Fatal(err)
	}
	if !body.ApplyRequired || body.Proposal.ExpiresAt == nil || !body.Proposal.ExpiresAt.After(time.Now().UTC()) {
		h.t.Fatalf("preview=%+v", body)
	}
	return body.Proposal
}

func proposalWithPayload(revisionID string, kind assistant.ProposalOperationKind, summary, payload string) assistant.DraftProposal {
	return assistant.DraftProposal{
		BaseRevisionID: revisionID,
		Operations: []assistant.ProposalOperation{{Kind: kind, Summary: summary, Payload: json.RawMessage(payload)}},
	}
}

func TestAssistantProposalPreviewDoesNotMutateAndApplyRequiresExplicitConfirmation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalDeviceMappingDraft, "Add projector", `{"logical_name":"PROJECTOR","logical_type":"GENERIC","configuration":{}}`)
	sealed := h.preview(h.owner, proposal)
	aliases, err := h.stageStore.ListAliases(context.Background(), h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("preview mutated aliases=%v", aliases)
	}
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealed, Apply: false})
	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "ASSISTANT_PROPOSAL_CONFIRMATION_REQUIRED") {
		t.Fatalf("apply=false status=%d body=%s", res.Code, res.Body.String())
	}
	aliases, err = h.stageStore.ListAliases(context.Background(), h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("unconfirmed apply mutated aliases=%v", aliases)
	}
}

func TestAssistantProposalApplyUsesOperationSpecificRBAC(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	password := "operator correct horse battery staple"
	if _, err := h.auth.auth.CreateUser(ctx, "assistant-operator", password, userauth.RoleOperator); err != nil {
		t.Fatal(err)
	}
	operator, err := h.auth.auth.Login(ctx, "assistant-operator", password, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}

	note := proposalWithPayload(h.revision.ID, assistant.ProposalNoteDraft, "Add note", `{"category":"assistant","body":"Check projector focus"}`)
	sealedNote := h.preview(operator, note)
	noteRes := h.request(operator, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealedNote, Apply: true})
	if noteRes.Code != http.StatusOK {
		t.Fatalf("operator note apply status=%d body=%s", noteRes.Code, noteRes.Body.String())
	}
	notes, err := h.stageStore.ListNotes(ctx, h.project.ID, store.NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Body != "Check projector focus" {
		t.Fatalf("notes=%+v", notes)
	}

	mapping := proposalWithPayload(h.revision.ID, assistant.ProposalDeviceMappingDraft, "Add projector", `{"logical_name":"PROJECTOR","logical_type":"GENERIC","configuration":{}}`)
	sealedMapping := h.preview(operator, mapping)
	mappingRes := h.request(operator, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealedMapping, Apply: true})
	if mappingRes.Code != http.StatusForbidden {
		t.Fatalf("operator structural apply status=%d body=%s", mappingRes.Code, mappingRes.Body.String())
	}
	aliases, err := h.stageStore.ListAliases(ctx, h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("forbidden structural apply mutated aliases=%v", aliases)
	}
}

func TestAssistantProposalApplyRejectsMultiOperationBeforeMutation(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := assistant.DraftProposal{
		BaseRevisionID: h.revision.ID,
		Operations: []assistant.ProposalOperation{
			{Kind: assistant.ProposalDeviceMappingDraft, Summary: "One", Payload: json.RawMessage(`{"logical_name":"ONE","configuration":{}}`)},
			{Kind: assistant.ProposalDeviceMappingDraft, Summary: "Two", Payload: json.RawMessage(`{"logical_name":"TWO","configuration":{}}`)},
		},
	}
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "ASSISTANT_PROPOSAL_ATOMIC_BATCH_REQUIRED") {
		t.Fatalf("multi apply status=%d body=%s", res.Code, res.Body.String())
	}
	aliases, err := h.stageStore.ListAliases(context.Background(), h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("multi apply mutated aliases=%v", aliases)
	}
}

func TestAssistantProposalApplyRejectsStaleBaseline(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalDeviceMappingDraft, "Add projector", `{"logical_name":"PROJECTOR","configuration":{}}`)
	sealed := h.preview(h.owner, proposal)
	if err := h.stageStore.SetRevisionStatus(ctx, h.revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	forked, err := h.stageStore.EnsureProjectDraft(ctx, h.project.ID, h.owner.Session.User.ID, "stale proposal test")
	if err != nil {
		t.Fatal(err)
	}
	if forked.ID == h.revision.ID {
		t.Fatalf("expected successor Draft, got original revision %s", forked.ID)
	}
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "ASSISTANT_PROPOSAL_STALE") {
		t.Fatalf("stale apply status=%d body=%s", res.Code, res.Body.String())
	}
	aliases, err := h.stageStore.ListAliases(ctx, h.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Fatalf("stale apply mutated aliases=%v", aliases)
	}
}

func TestAssistantProposalStructuralApplyReusesShowConfigurationLock(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	ctx := context.Background()
	if _, err := h.stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: h.revision.ID, DisplayLabel: "1", Name: "Cue One", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.stageStore.SetRevisionStatus(ctx, h.revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(h.stageStore).Create(ctx, h.revision.ID, h.owner.Session.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "assistant show lock"); err != nil {
		t.Fatal(err)
	}
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalDeviceMappingDraft, "Add projector", `{"logical_name":"PROJECTOR","configuration":{}}`)
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusLocked || !strings.Contains(res.Body.String(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("SHOW apply status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestAssistantProposalTemplatePreviewDoesNotInventApplySurface(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalTemplateDraft, "Draft template", `{"name":"Template proposal"}`)
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "ASSISTANT_PROPOSAL_APPLY_UNSUPPORTED") {
		t.Fatalf("template apply status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestAssistantProposalSuccessfulApplyWritesAuditEvidence(t *testing.T) {
	h := newAssistantProposalHTTPHarness(t)
	proposal := proposalWithPayload(h.revision.ID, assistant.ProposalDeviceMappingDraft, "Add projector", `{"logical_name":"PROJECTOR","logical_type":"GENERIC","configuration":{}}`)
	sealed := h.preview(h.owner, proposal)
	res := h.request(h.owner, http.MethodPost, "/api/v1/projects/"+h.project.ID+"/assistant/proposals/apply", assistantProposalApplyRequest{Proposal: sealed, Apply: true})
	if res.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", res.Code, res.Body.String())
	}
	records, err := h.audit.List(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record.EventType == "assistant.proposal.apply" && record.ResourceID == h.project.ID && record.Result == securityaudit.ResultSuccess {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("assistant apply audit not found: %+v", records)
	}
}
