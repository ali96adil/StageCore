package assistant

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDraftProposalSliceAContractRemainsValidWithoutPayloadOrExpiry(t *testing.T) {
	proposal := DraftProposal{
		BaseRevisionID: "revision-1",
		Operations:     []ProposalOperation{{Kind: ProposalCueDraft, Summary: "Add cue"}},
	}
	if err := proposal.Validate(); err != nil {
		t.Fatalf("Slice A proposal should remain valid: %v", err)
	}
	if err := proposal.ValidateForPreview(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("preview without payload error=%v, want ErrInvalidContract", err)
	}
}

func TestDraftProposalPreviewSealAndApplyLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	proposal := DraftProposal{
		BaseRevisionID: "revision-1",
		Operations: []ProposalOperation{{
			Kind: ProposalNoteDraft, Summary: "Add operator note",
			Payload: json.RawMessage(`{"category":"assistant","body":"Check projector"}`),
		}},
	}
	sealed, err := SealProposal(proposal, now.Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ExpiresAt == nil || !sealed.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("expires_at=%v", sealed.ExpiresAt)
	}
	if err := sealed.ValidateForApply(now); err != nil {
		t.Fatalf("sealed proposal should apply: %v", err)
	}
	if err := sealed.ValidateForApply(now.Add(16 * time.Minute)); !errors.Is(err, ErrProposalExpired) {
		t.Fatalf("expired proposal error=%v, want ErrProposalExpired", err)
	}
}

func TestDraftProposalApplyRequiresAtomicSingleOperation(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	proposal := DraftProposal{
		BaseRevisionID: "revision-1",
		Operations: []ProposalOperation{
			{Kind: ProposalNoteDraft, Summary: "First", Payload: json.RawMessage(`{"body":"one"}`)},
			{Kind: ProposalNoteDraft, Summary: "Second", Payload: json.RawMessage(`{"body":"two"}`)},
		},
	}
	sealed, err := SealProposal(proposal, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.ValidateForApply(now); !errors.Is(err, ErrProposalAtomicBatchRequired) {
		t.Fatalf("multi-operation apply error=%v, want ErrProposalAtomicBatchRequired", err)
	}
}

func TestDraftProposalRejectsInvalidStructuredPayload(t *testing.T) {
	proposal := DraftProposal{
		BaseRevisionID: "revision-1",
		Operations:     []ProposalOperation{{Kind: ProposalCueDraft, Summary: "Add cue", Payload: json.RawMessage(`{"broken":`)}},
	}
	if err := proposal.Validate(); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("invalid payload error=%v, want ErrInvalidContract", err)
	}
}
