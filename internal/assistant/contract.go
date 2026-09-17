package assistant

import (
	"errors"
	"fmt"
	"strings"
)

const ContractVersion1 = 1

const (
	MaxPromptBytes = 16 * 1024
	MaxAnswerBytes = 64 * 1024
)

type AuthorityClass string

const (
	AuthorityReadOnly      AuthorityClass = "READ_ONLY"
	AuthorityDraftProposal AuthorityClass = "DRAFT_PROPOSAL"
)

type RequestKind string

const (
	RequestExplain  RequestKind = "EXPLAIN"
	RequestDiagnose RequestKind = "DIAGNOSE"
	RequestDraft    RequestKind = "DRAFT"
)

type ProposalOperationKind string

const (
	ProposalCueDraft           ProposalOperationKind = "CUE_DRAFT"
	ProposalRoutingDraft       ProposalOperationKind = "ROUTING_DRAFT"
	ProposalDeviceMappingDraft ProposalOperationKind = "DEVICE_MAPPING_DRAFT"
	ProposalChecklistDraft     ProposalOperationKind = "CHECKLIST_DRAFT"
	ProposalNoteDraft          ProposalOperationKind = "NOTE_DRAFT"
	ProposalTemplateDraft      ProposalOperationKind = "TEMPLATE_DRAFT"
)

var ErrInvalidContract = errors.New("invalid assistant contract")

type EvidenceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type ProposalOperation struct {
	Kind     ProposalOperationKind `json:"kind"`
	TargetID string                `json:"target_id,omitempty"`
	Summary  string                `json:"summary"`
}

type DraftProposal struct {
	BaseRevisionID string              `json:"base_revision_id"`
	Operations     []ProposalOperation `json:"operations"`
}

type Request struct {
	ContractVersion int            `json:"contract_version"`
	RequestID       string         `json:"request_id"`
	ProjectID       string         `json:"project_id"`
	RevisionID      string         `json:"revision_id,omitempty"`
	Kind            RequestKind    `json:"kind"`
	Authority       AuthorityClass `json:"authority"`
	Prompt          string         `json:"prompt"`
	Context         ContextBundle  `json:"context"`
}

type Response struct {
	ContractVersion int            `json:"contract_version"`
	RequestID       string         `json:"request_id"`
	Authority       AuthorityClass `json:"authority"`
	Answer          string         `json:"answer"`
	Evidence        []EvidenceRef  `json:"evidence,omitempty"`
	Proposal        *DraftProposal `json:"proposal,omitempty"`
	Assumptions     []string       `json:"assumptions,omitempty"`
	MissingContext  []string       `json:"missing_context,omitempty"`
}

func (a AuthorityClass) Valid() bool {
	return a == AuthorityReadOnly || a == AuthorityDraftProposal
}

func (k RequestKind) Valid() bool {
	return k == RequestExplain || k == RequestDiagnose || k == RequestDraft
}

func (k ProposalOperationKind) Valid() bool {
	switch k {
	case ProposalCueDraft, ProposalRoutingDraft, ProposalDeviceMappingDraft, ProposalChecklistDraft, ProposalNoteDraft, ProposalTemplateDraft:
		return true
	default:
		return false
	}
}

func (r Request) Validate() error {
	if r.ContractVersion != ContractVersion1 {
		return invalid("unsupported contract_version %d", r.ContractVersion)
	}
	if err := validateID("request_id", r.RequestID, true); err != nil {
		return err
	}
	if err := validateID("project_id", r.ProjectID, true); err != nil {
		return err
	}
	if err := validateID("revision_id", r.RevisionID, false); err != nil {
		return err
	}
	if !r.Kind.Valid() {
		return invalid("unsupported request kind %q", r.Kind)
	}
	if !r.Authority.Valid() {
		return invalid("unsupported authority %q", r.Authority)
	}
	if strings.TrimSpace(r.Prompt) == "" || strings.TrimSpace(r.Prompt) != r.Prompt || len(r.Prompt) > MaxPromptBytes {
		return invalid("prompt must be non-empty, trimmed, and <= %d bytes", MaxPromptBytes)
	}
	if r.Kind == RequestDraft {
		if r.Authority != AuthorityDraftProposal {
			return invalid("DRAFT requests require DRAFT_PROPOSAL authority")
		}
	} else if r.Authority != AuthorityReadOnly {
		return invalid("EXPLAIN and DIAGNOSE requests require READ_ONLY authority")
	}
	if err := r.Context.Validate(); err != nil {
		return err
	}
	if r.Context.ProjectID != r.ProjectID {
		return invalid("context project_id must match request project_id")
	}
	if r.RevisionID != "" && r.Context.RevisionID != "" && r.Context.RevisionID != r.RevisionID {
		return invalid("context revision_id must match request revision_id")
	}
	return nil
}

func (r Response) ValidateFor(request Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if r.ContractVersion != ContractVersion1 {
		return invalid("unsupported response contract_version %d", r.ContractVersion)
	}
	if r.RequestID != request.RequestID {
		return invalid("response request_id does not match request")
	}
	if r.Authority != request.Authority {
		return invalid("response authority does not match request")
	}
	if len(r.Answer) > MaxAnswerBytes || strings.TrimSpace(r.Answer) != r.Answer {
		return invalid("answer must be trimmed and <= %d bytes", MaxAnswerBytes)
	}
	for _, evidence := range r.Evidence {
		if err := validateID("evidence.kind", evidence.Kind, true); err != nil {
			return err
		}
		if err := validateID("evidence.id", evidence.ID, true); err != nil {
			return err
		}
	}
	if request.Kind != RequestDraft && r.Proposal != nil {
		return invalid("read-only responses cannot include a draft proposal")
	}
	if r.Proposal != nil {
		if err := r.Proposal.Validate(); err != nil {
			return err
		}
		if request.RevisionID != "" && r.Proposal.BaseRevisionID != request.RevisionID {
			return invalid("proposal base_revision_id does not match request revision_id")
		}
	}
	return nil
}

func (p DraftProposal) Validate() error {
	if err := validateID("base_revision_id", p.BaseRevisionID, true); err != nil {
		return err
	}
	if len(p.Operations) == 0 {
		return invalid("proposal requires at least one operation")
	}
	for i, operation := range p.Operations {
		if !operation.Kind.Valid() {
			return invalid("proposal operation %d has unsupported kind %q", i, operation.Kind)
		}
		if operation.TargetID != "" {
			if err := validateID("proposal target_id", operation.TargetID, false); err != nil {
				return err
			}
		}
		if strings.TrimSpace(operation.Summary) == "" || strings.TrimSpace(operation.Summary) != operation.Summary {
			return invalid("proposal operation %d summary must be non-empty and trimmed", i)
		}
	}
	return nil
}

func validateID(name, value string, required bool) error {
	if value == "" && !required {
		return nil
	}
	if value == "" || strings.TrimSpace(value) != value || len(value) > 256 {
		return invalid("%s must be non-empty, trimmed, and <= 256 bytes", name)
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidContract, fmt.Sprintf(format, args...))
}
