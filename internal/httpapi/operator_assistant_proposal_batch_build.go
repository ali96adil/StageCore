package httpapi

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

type assistantBatchChecklistPayload struct {
	SessionID *string  `json:"session_id,omitempty"`
	CueID     *string  `json:"cue_id,omitempty"`
	CueRef    string   `json:"cue_ref,omitempty"`
	Items     []string `json:"items"`
}

type assistantBatchNotePayload struct {
	SessionID *string `json:"session_id,omitempty"`
	CueID     *string `json:"cue_id,omitempty"`
	CueRef    string  `json:"cue_ref,omitempty"`
	Category  string  `json:"category"`
	Body      string  `json:"body"`
}

type assistantBatchRouteActionPayload struct {
	OutputID   *string         `json:"output_id,omitempty"`
	CueID      *string         `json:"cue_id,omitempty"`
	CueRef     string          `json:"cue_ref,omitempty"`
	Parameters json.RawMessage `json:"parameters"`
}

type assistantBatchRoutePayload struct {
	Name                string                             `json:"name"`
	InputID             string                             `json:"input_id"`
	ConditionDefinition json.RawMessage                    `json:"condition_definition"`
	TransformDefinition json.RawMessage                    `json:"transform_definition"`
	DelayMS             *int64                             `json:"delay_ms,omitempty"`
	DebounceMS          *int64                             `json:"debounce_ms,omitempty"`
	PriorityClass       domain.PriorityClass               `json:"priority_class"`
	ErrorPolicy         json.RawMessage                    `json:"error_policy"`
	Enabled             bool                               `json:"enabled"`
	Actions             []assistantBatchRouteActionPayload `json:"actions"`
}

func buildAssistantProposalBatch(projectID string, proposal assistant.DraftProposal) ([]store.AssistantProposalMutation, error) {
	mutations := make([]store.AssistantProposalMutation, 0, len(proposal.Operations))
	for _, op := range proposal.Operations {
		mutation := store.AssistantProposalMutation{Ref: strings.TrimSpace(op.Ref), TargetID: strings.TrimSpace(op.TargetID)}
		switch op.Kind {
		case assistant.ProposalCueDraft:
			var body cueWriteRequest
			if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
				return nil, err
			}
			cue, actions := body.toDomain(proposal.BaseRevisionID, mutation.TargetID)
			mutation.Kind = store.AssistantProposalMutationCue
			mutation.Cue = cue
			mutation.CueActions = actions
		case assistant.ProposalRoutingDraft:
			var body assistantBatchRoutePayload
			if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
				return nil, err
			}
			if len(body.ConditionDefinition) == 0 {
				body.ConditionDefinition = json.RawMessage(`null`)
			}
			if len(body.TransformDefinition) == 0 {
				body.TransformDefinition = json.RawMessage(`null`)
			}
			if len(body.ErrorPolicy) == 0 {
				body.ErrorPolicy = json.RawMessage(`{}`)
			}
			actions := make([]store.AssistantProposalRouteAction, 0, len(body.Actions))
			for index, action := range body.Actions {
				parameters := action.Parameters
				if len(parameters) == 0 {
					parameters = json.RawMessage(`{}`)
				}
				actions = append(actions, store.AssistantProposalRouteAction{
					Action: domain.RouteAction{OrderIndex: index, OutputID: action.OutputID, CueID: action.CueID, Parameters: parameters},
					CueRef: strings.TrimSpace(action.CueRef),
				})
			}
			mutation.Kind = store.AssistantProposalMutationRoute
			mutation.Route = domain.Route{
				ID: mutation.TargetID, RevisionID: proposal.BaseRevisionID,
				Name: strings.TrimSpace(body.Name), InputID: strings.TrimSpace(body.InputID),
				ConditionDefinition: body.ConditionDefinition, TransformDefinition: body.TransformDefinition,
				DelayMS: body.DelayMS, DebounceMS: body.DebounceMS, PriorityClass: body.PriorityClass,
				ErrorPolicy: body.ErrorPolicy, Enabled: body.Enabled,
			}
			mutation.RouteActions = actions
		case assistant.ProposalDeviceMappingDraft:
			var body targetCreateRequest
			if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
				return nil, err
			}
			if len(body.Configuration) == 0 {
				body.Configuration = json.RawMessage(`{}`)
			}
			mutation.Kind = store.AssistantProposalMutationAlias
			mutation.Alias = domain.ProjectDeviceAlias{
				ID: mutation.TargetID, ProjectID: projectID,
				LogicalName: strings.TrimSpace(body.LogicalName), LogicalType: strings.TrimSpace(body.LogicalType),
				TargetRef: strings.TrimSpace(body.TargetRef), GroupName: strings.TrimSpace(body.GroupName), ProjectConfig: body.Configuration,
			}
		case assistant.ProposalNoteDraft:
			var body assistantBatchNotePayload
			if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
				return nil, err
			}
			mutation.Kind = store.AssistantProposalMutationNote
			mutation.Note = store.CreateNoteParams{SessionID: body.SessionID, CueID: body.CueID, Category: body.Category, Body: body.Body}
			mutation.NoteCueRef = strings.TrimSpace(body.CueRef)
		case assistant.ProposalChecklistDraft:
			var body assistantBatchChecklistPayload
			if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
				return nil, err
			}
			checklist, err := renderAssistantChecklist(body.Items)
			if err != nil {
				return nil, err
			}
			mutation.Kind = store.AssistantProposalMutationNote
			mutation.Note = store.CreateNoteParams{SessionID: body.SessionID, CueID: body.CueID, Category: "ASSISTANT_CHECKLIST", Body: checklist}
			mutation.NoteCueRef = strings.TrimSpace(body.CueRef)
		case assistant.ProposalTemplateDraft:
			return nil, errors.Join(errAssistantProposalApplyUnsupported, errors.New("current-project template apply has no canonical mutation surface"))
		default:
			return nil, assistant.ErrInvalidContract
		}
		mutations = append(mutations, mutation)
	}
	return mutations, nil
}
