package assistant

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDraftProposalBatchApplyAllowsBoundedAtomicOperations(t *testing.T) {
	now := time.Date(2026, 9, 17, 18, 0, 0, 0, time.UTC)
	proposal := DraftProposal{
		BaseRevisionID: "revision-1",
		Operations: []ProposalOperation{
			{Kind: ProposalCueDraft, Ref: "cue-a", Summary: "Add cue", Payload: json.RawMessage(`{"name":"A"}`)},
			{Kind: ProposalNoteDraft, Summary: "Add note", Payload: json.RawMessage(`{"body":"note"}`)},
		},
	}
	sealed, err := SealProposal(proposal, now.Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.ValidateForApply(now); !errors.Is(err, ErrProposalAtomicBatchRequired) {
		t.Fatalf("legacy apply error=%v, want ErrProposalAtomicBatchRequired", err)
	}
	if err := sealed.ValidateForBatchApply(now); err != nil {
		t.Fatalf("atomic batch should validate: %v", err)
	}
}

func TestDraftProposalRejectsDuplicateOperationRefs(t *testing.T) {
	proposal := DraftProposal{
		BaseRevisionID: "revision-1",
		Operations: []ProposalOperation{
			{Kind: ProposalCueDraft, Ref: "same-ref", Summary: "One", Payload: json.RawMessage(`{"name":"One"}`)},
			{Kind: ProposalCueDraft, Ref: "same-ref", Summary: "Two", Payload: json.RawMessage(`{"name":"Two"}`)},
		},
	}
	if err := proposal.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("duplicate ref error=%v, want ErrInvalidContract", err)
	}
}
