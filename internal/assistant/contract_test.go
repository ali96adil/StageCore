package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type passRedactor struct{}

func (passRedactor) RedactString(_ context.Context, input string) string {
	return input
}

type replaceRedactor struct {
	secret string
}

func (r replaceRedactor) RedactString(_ context.Context, input string) string {
	return strings.ReplaceAll(input, r.secret, "[REDACTED]")
}

func testContext(t *testing.T) ContextBundle {
	t.Helper()
	bundle, err := NewContextBundle(context.Background(), passRedactor{}, "project-1", "revision-1", []ContextFact{
		{Kind: ContextCueSummary, RefID: "cue-42", Summary: "Cue 42 failed after a timeout"},
	})
	if err != nil {
		t.Fatalf("NewContextBundle() error = %v", err)
	}
	return bundle
}

func TestRequestValidationEnforcesAdvisoryAuthority(t *testing.T) {
	base := Request{
		ContractVersion: ContractVersion1,
		RequestID:       "request-1",
		ProjectID:       "project-1",
		RevisionID:      "revision-1",
		Kind:            RequestExplain,
		Authority:       AuthorityReadOnly,
		Prompt:          "Why did Cue 42 fail?",
		Context:         testContext(t),
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid read-only request rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Request)
	}{
		{
			name: "unknown live execution authority",
			mutate: func(request *Request) {
				request.Authority = AuthorityClass("LIVE_EXECUTION")
			},
		},
		{
			name: "read request cannot ask for draft authority",
			mutate: func(request *Request) {
				request.Authority = AuthorityDraftProposal
			},
		},
		{
			name: "draft requires proposal authority",
			mutate: func(request *Request) {
				request.Kind = RequestDraft
				request.Authority = AuthorityReadOnly
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := base
			tc.mutate(&request)
			if err := request.Validate(); !errors.Is(err, ErrInvalidContract) {
				t.Fatalf("Validate() error = %v, want ErrInvalidContract", err)
			}
		})
	}
}

func TestDraftProposalRejectsRuntimeCommandKinds(t *testing.T) {
	proposal := DraftProposal{
		BaseRevisionID: "revision-1",
		Operations: []ProposalOperation{
			{Kind: ProposalOperationKind("GO"), Summary: "Execute the next cue"},
		},
	}
	if err := proposal.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("Validate() error = %v, want ErrInvalidContract", err)
	}

	proposal.Operations[0] = ProposalOperation{Kind: ProposalOperationKind("BLACKOUT"), Summary: "Emergency blackout"}
	if err := proposal.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("Validate() error = %v, want ErrInvalidContract", err)
	}
}

func TestReadOnlyResponseCannotCarryMutationProposal(t *testing.T) {
	request := Request{
		ContractVersion: ContractVersion1,
		RequestID:       "request-1",
		ProjectID:       "project-1",
		RevisionID:      "revision-1",
		Kind:            RequestDiagnose,
		Authority:       AuthorityReadOnly,
		Prompt:          "Diagnose the cue failure",
		Context:         testContext(t),
	}
	response := Response{
		ContractVersion: ContractVersion1,
		RequestID:       request.RequestID,
		Authority:       AuthorityReadOnly,
		Answer:          "The recorded action timed out.",
		Proposal: &DraftProposal{
			BaseRevisionID: "revision-1",
			Operations: []ProposalOperation{
				{Kind: ProposalNoteDraft, Summary: "Add a troubleshooting note"},
			},
		},
	}
	if err := response.ValidateFor(request); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("ValidateFor() error = %v, want ErrInvalidContract", err)
	}
}

func TestContextBuilderRequiresRedactor(t *testing.T) {
	_, err := NewContextBundle(context.Background(), nil, "project-1", "revision-1", []ContextFact{
		{Kind: ContextDoctorFinding, RefID: "doctor-1", Summary: "Diagnostic evidence"},
	})
	if !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("NewContextBundle() error = %v, want ErrInvalidContract", err)
	}
}

func TestContextBuilderUsesRedactorAndRejectsNonAllowlistedKinds(t *testing.T) {
	bundle, err := NewContextBundle(context.Background(), replaceRedactor{secret: "super-secret-value"}, "project-1", "revision-1", []ContextFact{
		{Kind: ContextDoctorFinding, RefID: "doctor-1", Summary: "Credential value super-secret-value was detected in diagnostic text"},
	})
	if err != nil {
		t.Fatalf("NewContextBundle() error = %v", err)
	}
	if strings.Contains(bundle.Facts[0].Summary, "super-secret-value") {
		t.Fatal("context retained a redacted secret value")
	}
	if !strings.Contains(bundle.Facts[0].Summary, "[REDACTED]") {
		t.Fatalf("context summary = %q, want redaction marker", bundle.Facts[0].Summary)
	}

	bundle.Facts[0].Kind = ContextKind("SECRET_STORE_RECORD")
	if err := bundle.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("Validate() error = %v, want ErrInvalidContract", err)
	}
}

func TestFakeProviderIsDeterministicAndValidatesResponses(t *testing.T) {
	request := Request{
		ContractVersion: ContractVersion1,
		RequestID:       "request-1",
		ProjectID:       "project-1",
		RevisionID:      "revision-1",
		Kind:            RequestExplain,
		Authority:       AuthorityReadOnly,
		Prompt:          "Explain the current readiness state",
		Context:         testContext(t),
	}
	want := Response{
		ContractVersion: ContractVersion1,
		RequestID:       request.RequestID,
		Authority:       AuthorityReadOnly,
		Answer:          "The supplied evidence reports one cue timeout.",
		Evidence:        []EvidenceRef{{Kind: "FLIGHT_RECORDER", ID: "event-1"}},
	}
	provider := FakeProvider{Responses: map[string]Response{request.RequestID: want}}
	got, err := provider.Complete(context.Background(), request)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got.Answer != want.Answer {
		t.Fatalf("Complete() answer = %q, want %q", got.Answer, want.Answer)
	}

	missing := request
	missing.RequestID = "missing"
	if _, err := provider.Complete(context.Background(), missing); !errors.Is(err, ErrFakeResponseMissing) {
		t.Fatalf("Complete() error = %v, want ErrFakeResponseMissing", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Complete(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete() cancelled error = %v, want context.Canceled", err)
	}
}

func TestDraftResponseAcceptsOnlyStructuredProposal(t *testing.T) {
	request := Request{
		ContractVersion: ContractVersion1,
		RequestID:       "request-draft-1",
		ProjectID:       "project-1",
		RevisionID:      "revision-1",
		Kind:            RequestDraft,
		Authority:       AuthorityDraftProposal,
		Prompt:          "Draft a troubleshooting checklist",
		Context:         testContext(t),
	}
	response := Response{
		ContractVersion: ContractVersion1,
		RequestID:       request.RequestID,
		Authority:       AuthorityDraftProposal,
		Answer:          "Prepared an editable checklist proposal.",
		Proposal: &DraftProposal{
			BaseRevisionID: "revision-1",
			Operations: []ProposalOperation{
				{Kind: ProposalChecklistDraft, Summary: "Verify tablet readiness before Scene 3"},
			},
		},
	}
	if err := response.ValidateFor(request); err != nil {
		t.Fatalf("ValidateFor() error = %v", err)
	}
}
