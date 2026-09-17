package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type workspaceRedactor struct{}

func (workspaceRedactor) RedactString(_ context.Context, value string) string {
	return strings.ReplaceAll(value, "secret-value", "[REDACTED]")
}

type workspaceDraftSource struct {
	collection EvidenceCollection
}

func (s workspaceDraftSource) CollectDraftContext(context.Context, string, string) (EvidenceCollection, error) {
	return s.collection, nil
}

type captureWorkspaceProvider struct {
	request  Request
	response Response
	err      error
}

func (p *captureWorkspaceProvider) Complete(_ context.Context, request Request) (Response, error) {
	p.request = request
	if p.err != nil {
		return Response{}, p.err
	}
	return p.response, nil
}

func TestWorkspaceAvailabilityReflectsConfiguredSafeSurfaces(t *testing.T) {
	service := WorkspaceService{}
	if got := service.Availability(); got.Provider || got.Explain || got.Diagnose || got.Draft {
		t.Fatalf("empty availability=%+v", got)
	}
	provider := &captureWorkspaceProvider{}
	service = WorkspaceService{Provider: provider, Redactor: workspaceRedactor{}, DraftSource: workspaceDraftSource{}}
	got := service.Availability()
	if !got.Provider || got.Explain || got.Diagnose || !got.Draft {
		t.Fatalf("draft-only availability=%+v", got)
	}
}

func TestWorkspaceDraftUsesCanonicalRedactedContextAndRequiresStructuredProposal(t *testing.T) {
	provider := &captureWorkspaceProvider{response: Response{
		ContractVersion: ContractVersion1,
		RequestID:       "request-draft",
		Authority:       AuthorityDraftProposal,
		Answer:          "Draft prepared.",
		Evidence:        []EvidenceRef{{Kind: string(ContextProjectSummary), ID: "project-1"}},
		Proposal: &DraftProposal{
			BaseRevisionID: "revision-1",
			Operations: []ProposalOperation{{
				Kind: ProposalNoteDraft, Summary: "Add note", Payload: json.RawMessage(`{"body":"Check focus"}`),
			}},
		},
	}}
	service := WorkspaceService{
		Provider: provider,
		Redactor: workspaceRedactor{},
		DraftSource: workspaceDraftSource{collection: EvidenceCollection{
			Facts:          []ContextFact{{Kind: ContextProjectSummary, RefID: "project-1", Summary: "project secret-value"}},
			MissingContext: []string{"no device health evidence"},
		}},
	}
	response, err := service.Respond(context.Background(), WorkspaceTaskInput{
		RequestID: "request-draft", ProjectID: "project-1", RevisionID: "revision-1",
		Kind: RequestDraft, Prompt: "Add a focus note",
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.request.Authority != AuthorityDraftProposal || provider.request.Kind != RequestDraft {
		t.Fatalf("request=%+v", provider.request)
	}
	if got := provider.request.Context.Facts[0].Summary; strings.Contains(got, "secret-value") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("context was not redacted: %q", got)
	}
	if response.Proposal == nil || response.Proposal.ExpiresAt != nil {
		t.Fatalf("proposal=%+v", response.Proposal)
	}
	if len(response.MissingContext) != 1 || response.MissingContext[0] != "no device health evidence" {
		t.Fatalf("missing_context=%v", response.MissingContext)
	}
}

func TestWorkspaceDraftRejectsUngroundedEvidence(t *testing.T) {
	provider := &captureWorkspaceProvider{response: Response{
		ContractVersion: ContractVersion1,
		RequestID:       "request-draft",
		Authority:       AuthorityDraftProposal,
		Evidence:        []EvidenceRef{{Kind: string(ContextCueSummary), ID: "invented-cue"}},
		Proposal: &DraftProposal{
			BaseRevisionID: "revision-1",
			Operations:     []ProposalOperation{{Kind: ProposalNoteDraft, Summary: "Add note", Payload: json.RawMessage(`{"body":"Check focus"}`)}},
		},
	}}
	service := WorkspaceService{
		Provider: provider, Redactor: workspaceRedactor{},
		DraftSource: workspaceDraftSource{collection: EvidenceCollection{Facts: []ContextFact{{Kind: ContextProjectSummary, RefID: "project-1", Summary: "project"}}}},
	}
	_, err := service.Respond(context.Background(), WorkspaceTaskInput{
		RequestID: "request-draft", ProjectID: "project-1", RevisionID: "revision-1", Kind: RequestDraft, Prompt: "Draft a note",
	})
	if !errors.Is(err, ErrUngroundedEvidence) {
		t.Fatalf("error=%v, want ErrUngroundedEvidence", err)
	}
}

func TestWorkspaceFailsClosedWithoutProvider(t *testing.T) {
	_, err := (WorkspaceService{}).Respond(context.Background(), WorkspaceTaskInput{Kind: RequestDraft})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("error=%v, want ErrProviderUnavailable", err)
	}
}
