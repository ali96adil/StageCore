package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type EvidenceScope string

const (
	EvidenceExecution EvidenceScope = "EXECUTION"
	EvidenceReadiness EvidenceScope = "READINESS"
	EvidenceTiming    EvidenceScope = "TIMING"
)

var (
	ErrEvidenceUnavailable = errors.New("assistant evidence unavailable")
	ErrUngroundedEvidence  = errors.New("assistant response references ungrounded evidence")
)

type EvidenceQuery struct {
	Scope             EvidenceScope
	ProjectID         string
	RevisionID        string
	RuntimeSnapshotID string
	SessionID         string
	CueExecutionID    string
}

type EvidenceCollection struct {
	Facts          []ContextFact
	MissingContext []string
}

type EvidenceSource interface {
	Collect(context.Context, EvidenceQuery) (EvidenceCollection, error)
}

type DiagnosticInput struct {
	RequestID         string
	ProjectID         string
	RevisionID        string
	RuntimeSnapshotID string
	SessionID         string
	CueExecutionID    string
	Kind              RequestKind
	Scope             EvidenceScope
	Prompt            string
}

type DiagnosticService struct {
	Provider Provider
	Source   EvidenceSource
	Redactor Redactor
}

func (s DiagnosticService) Respond(ctx context.Context, input DiagnosticInput) (Response, error) {
	if s.Provider == nil {
		return Response{}, ErrProviderUnavailable
	}
	if s.Source == nil {
		return Response{}, fmt.Errorf("%w: evidence source is not configured", ErrEvidenceUnavailable)
	}
	if s.Redactor == nil {
		return Response{}, invalid("diagnostic redactor is required")
	}
	if input.Kind != RequestExplain && input.Kind != RequestDiagnose {
		return Response{}, invalid("read-only diagnostics require EXPLAIN or DIAGNOSE request kind")
	}
	if !input.Scope.Valid() {
		return Response{}, invalid("unsupported evidence scope %q", input.Scope)
	}

	collection, err := s.Source.Collect(ctx, EvidenceQuery{
		Scope:             input.Scope,
		ProjectID:         input.ProjectID,
		RevisionID:        input.RevisionID,
		RuntimeSnapshotID: input.RuntimeSnapshotID,
		SessionID:         input.SessionID,
		CueExecutionID:    input.CueExecutionID,
	})
	if err != nil {
		return Response{}, err
	}
	if len(collection.Facts) == 0 {
		detail := strings.Join(normalizeContextList(collection.MissingContext), "; ")
		if detail == "" {
			detail = "no canonical StageCore evidence matched the request"
		}
		return Response{}, fmt.Errorf("%w: %s", ErrEvidenceUnavailable, detail)
	}

	bundle, err := NewContextBundle(ctx, s.Redactor, input.ProjectID, input.RevisionID, collection.Facts)
	if err != nil {
		return Response{}, err
	}
	request := Request{
		ContractVersion: ContractVersion1,
		RequestID:       input.RequestID,
		ProjectID:       input.ProjectID,
		RevisionID:      input.RevisionID,
		Kind:            input.Kind,
		Authority:       AuthorityReadOnly,
		Prompt:          input.Prompt,
		Context:         bundle,
	}
	if err := request.Validate(); err != nil {
		return Response{}, err
	}

	response, err := s.Provider.Complete(ctx, request)
	if err != nil {
		return Response{}, err
	}
	if err := response.ValidateFor(request); err != nil {
		return Response{}, err
	}
	if err := validateResponseGrounding(response, bundle); err != nil {
		return Response{}, err
	}
	response.MissingContext = mergeContextLists(response.MissingContext, collection.MissingContext)
	return response, nil
}

func (s EvidenceScope) Valid() bool {
	return s == EvidenceExecution || s == EvidenceReadiness || s == EvidenceTiming
}

func validateResponseGrounding(response Response, bundle ContextBundle) error {
	allowed := make(map[string]struct{}, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		allowed[string(fact.Kind)+"\x00"+fact.RefID] = struct{}{}
	}
	for _, evidence := range response.Evidence {
		if _, ok := allowed[evidence.Kind+"\x00"+evidence.ID]; !ok {
			return fmt.Errorf("%w: %s/%s was not supplied in canonical context", ErrUngroundedEvidence, evidence.Kind, evidence.ID)
		}
	}
	if len(response.Evidence) == 0 && len(normalizeContextList(response.MissingContext)) == 0 {
		return fmt.Errorf("%w: read-only answers must cite supplied evidence or declare missing context", ErrUngroundedEvidence)
	}
	return nil
}

func mergeContextLists(primary, secondary []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(primary)+len(secondary))
	for _, list := range [][]string{primary, secondary} {
		for _, item := range normalizeContextList(list) {
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func normalizeContextList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
