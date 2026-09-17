package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

type DraftContextSource interface {
	CollectDraftContext(context.Context, string, string) (EvidenceCollection, error)
}

type DraftContextStore interface {
	GetProject(context.Context, string) (domain.Project, error)
	GetRevision(context.Context, string) (domain.ProjectRevision, error)
	ListCues(context.Context, string) ([]domain.Cue, error)
	ListRoutes(context.Context, string) ([]domain.Route, error)
	ListAliases(context.Context, string) ([]domain.ProjectDeviceAlias, error)
}

type CanonicalDraftContextSource struct {
	Store DraftContextStore
}

type WorkspaceTaskInput struct {
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

type WorkspaceAvailability struct {
	Provider bool `json:"provider"`
	Explain  bool `json:"explain"`
	Diagnose bool `json:"diagnose"`
	Draft    bool `json:"draft"`
}

type WorkspaceService struct {
	Provider       Provider
	EvidenceSource EvidenceSource
	DraftSource    DraftContextSource
	Redactor       Redactor
}

func (s WorkspaceService) Availability() WorkspaceAvailability {
	provider := s.Provider != nil
	readOnly := provider && s.EvidenceSource != nil && s.Redactor != nil
	draft := provider && s.DraftSource != nil && s.Redactor != nil
	return WorkspaceAvailability{Provider: provider, Explain: readOnly, Diagnose: readOnly, Draft: draft}
}

func (s WorkspaceService) Respond(ctx context.Context, input WorkspaceTaskInput) (Response, error) {
	if s.Provider == nil {
		return Response{}, ErrProviderUnavailable
	}
	switch input.Kind {
	case RequestExplain, RequestDiagnose:
		return DiagnosticService{Provider: s.Provider, Source: s.EvidenceSource, Redactor: s.Redactor}.Respond(ctx, DiagnosticInput{
			RequestID:         input.RequestID,
			ProjectID:         input.ProjectID,
			RevisionID:        input.RevisionID,
			RuntimeSnapshotID: input.RuntimeSnapshotID,
			SessionID:         input.SessionID,
			CueExecutionID:    input.CueExecutionID,
			Kind:              input.Kind,
			Scope:             input.Scope,
			Prompt:            input.Prompt,
		})
	case RequestDraft:
		return s.respondDraft(ctx, input)
	default:
		return Response{}, invalid("workspace requires EXPLAIN, DIAGNOSE or DRAFT request kind")
	}
}

func (s WorkspaceService) respondDraft(ctx context.Context, input WorkspaceTaskInput) (Response, error) {
	if s.DraftSource == nil {
		return Response{}, fmt.Errorf("%w: draft context source is not configured", ErrEvidenceUnavailable)
	}
	if s.Redactor == nil {
		return Response{}, invalid("workspace redactor is required")
	}
	collection, err := s.DraftSource.CollectDraftContext(ctx, input.ProjectID, input.RevisionID)
	if err != nil {
		return Response{}, err
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
		Kind:            RequestDraft,
		Authority:       AuthorityDraftProposal,
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
	if response.Proposal == nil {
		return Response{}, invalid("DRAFT response requires a structured proposal")
	}
	if err := response.Proposal.ValidateForPreview(); err != nil {
		return Response{}, err
	}
	if len(response.Evidence) > 0 {
		if err := validateEvidenceRefs(response, bundle); err != nil {
			return Response{}, err
		}
	}
	// The provider never owns proposal lifetime. Preview/SealProposal assigns
	// StageCore's short expiry immediately before explicit operator Apply.
	response.Proposal.ExpiresAt = nil
	response.MissingContext = mergeContextLists(response.MissingContext, collection.MissingContext)
	return response, nil
}

func validateEvidenceRefs(response Response, bundle ContextBundle) error {
	allowed := make(map[string]struct{}, len(bundle.Facts))
	for _, fact := range bundle.Facts {
		allowed[string(fact.Kind)+"\x00"+fact.RefID] = struct{}{}
	}
	for _, evidence := range response.Evidence {
		if _, ok := allowed[evidence.Kind+"\x00"+evidence.ID]; !ok {
			return fmt.Errorf("%w: %s/%s was not supplied in canonical context", ErrUngroundedEvidence, evidence.Kind, evidence.ID)
		}
	}
	return nil
}

func (s CanonicalDraftContextSource) CollectDraftContext(ctx context.Context, projectID, revisionID string) (EvidenceCollection, error) {
	var out EvidenceCollection
	projectID = strings.TrimSpace(projectID)
	revisionID = strings.TrimSpace(revisionID)
	if projectID == "" || revisionID == "" {
		return out, invalid("draft context requires project_id and revision_id")
	}
	if s.Store == nil {
		return out, fmt.Errorf("%w: canonical Draft store is unavailable", ErrEvidenceUnavailable)
	}
	project, err := s.Store.GetProject(ctx, projectID)
	if err != nil {
		return out, err
	}
	revision, err := s.Store.GetRevision(ctx, revisionID)
	if err != nil {
		return out, err
	}
	if revision.ProjectID != project.ID {
		return out, domain.ErrNotFound
	}
	appendEvidenceFact(&out, ContextProjectSummary, project.ID, fmt.Sprintf(
		"project name=%s description=%s lifecycle_state=%s current_revision_id=%s",
		strings.TrimSpace(project.Name), strings.TrimSpace(project.Description), project.LifecycleState, project.CurrentRevisionID,
	))
	parent := ""
	if revision.ParentRevisionID != nil {
		parent = *revision.ParentRevisionID
	}
	appendEvidenceFact(&out, ContextRevisionSummary, revision.ID, fmt.Sprintf(
		"revision number=%d status=%s parent_revision_id=%s change_note=%s created_by=%s",
		revision.RevisionNumber, revision.Status, parent, strings.TrimSpace(revision.ChangeNote), strings.TrimSpace(revision.CreatedBy),
	))

	cues, err := s.Store.ListCues(ctx, revisionID)
	if err != nil {
		return out, err
	}
	for _, cue := range cues {
		steps := make([]string, 0, len(cue.Actions))
		for _, action := range cue.Actions {
			steps = append(steps, fmt.Sprintf("%s:%s", strings.TrimSpace(action.TargetRef), strings.TrimSpace(action.CapabilityKey)))
		}
		appendEvidenceFact(&out, ContextCueSummary, cue.ID, fmt.Sprintf(
			"cue label=%s name=%s type=%s criticality=%s enabled=%t actions=%d action_targets=%s",
			strings.TrimSpace(cue.DisplayLabel), strings.TrimSpace(cue.Name), cue.CueType, cue.Criticality, cue.Enabled, len(cue.Actions), strings.Join(steps, ","),
		))
	}
	if len(cues) == 0 {
		addMissing(&out, "current Draft contains no Cues")
	}

	routes, err := s.Store.ListRoutes(ctx, revisionID)
	if err != nil {
		return out, err
	}
	for _, route := range routes {
		targets := make([]string, 0, len(route.Actions))
		for _, action := range route.Actions {
			target := ""
			if action.OutputID != nil {
				target = "output:" + *action.OutputID
			}
			if action.CueID != nil {
				target = "cue:" + *action.CueID
			}
			if target != "" {
				targets = append(targets, target)
			}
		}
		appendEvidenceFact(&out, ContextRoutingSummary, route.ID, fmt.Sprintf(
			"route name=%s input_id=%s priority=%s enabled=%t actions=%d targets=%s",
			strings.TrimSpace(route.Name), route.InputID, route.PriorityClass, route.Enabled, len(route.Actions), strings.Join(targets, ","),
		))
	}
	if len(routes) == 0 {
		addMissing(&out, "current Draft contains no Routes")
	}

	aliases, err := s.Store.ListAliases(ctx, projectID)
	if err != nil {
		return out, err
	}
	for _, alias := range aliases {
		appendEvidenceFact(&out, ContextDeviceSummary, alias.ID, fmt.Sprintf(
			"device_mapping logical_name=%s logical_type=%s target_ref=%s group=%s",
			strings.TrimSpace(alias.LogicalName), strings.TrimSpace(alias.LogicalType), strings.TrimSpace(alias.TargetRef), strings.TrimSpace(alias.GroupName),
		))
	}
	if len(aliases) == 0 {
		addMissing(&out, "Project contains no Device Mappings")
	}
	return out, nil
}
