package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const assistantProposalPreviewTTL = 15 * time.Minute

var (
	errAssistantProposalStale            = errors.New("assistant proposal stale")
	errAssistantProposalApplyUnsupported = errors.New("assistant proposal apply unsupported")
	errAssistantProposalPayloadInvalid   = errors.New("assistant proposal payload invalid")
	errAssistantProposalDraftRequired    = errors.New("assistant proposal draft required")
)

type assistantProposalRequest struct {
	Proposal assistant.DraftProposal `json:"proposal"`
}

type assistantProposalApplyRequest struct {
	Proposal assistant.DraftProposal `json:"proposal"`
	Apply    bool                    `json:"apply"`
}

type assistantChecklistPayload struct {
	SessionID *string  `json:"session_id,omitempty"`
	CueID     *string  `json:"cue_id,omitempty"`
	Items     []string `json:"items"`
}

func registerOperatorAssistantProposalRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, audit *securityaudit.Service) {
	mux.HandleFunc("POST /api/v1/projects/{project_id}/assistant/proposals/preview", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		var input assistantProposalRequest
		if !decodeBoundedJSON(w, r, &input) {
			return
		}
		if err := input.Proposal.ValidateForPreview(); err != nil {
			writeAssistantProposalError(w, err)
			return
		}
		project, err := stageStore.GetProject(r.Context(), projectID)
		if err != nil {
			writeAssistantProposalError(w, err)
			return
		}
		if project.CurrentRevisionID != input.Proposal.BaseRevisionID {
			writeAssistantProposalError(w, errAssistantProposalStale)
			return
		}
		sealed, err := assistant.SealProposal(input.Proposal, time.Now().UTC().Add(assistantProposalPreviewTTL))
		if err != nil {
			writeAssistantProposalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"proposal":        sealed,
			"apply_required":  true,
			"operation_count": len(sealed.Operations),
		})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/assistant/proposals/apply", withAuthenticatedSession(auth, true, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		var input assistantProposalApplyRequest
		if !decodeBoundedJSON(w, r, &input) {
			return
		}
		if !input.Apply {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, "explicit apply confirmation required")
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "ASSISTANT_PROPOSAL_CONFIRMATION_REQUIRED"})
			return
		}
		if err := input.Proposal.ValidateForApply(time.Now().UTC()); err != nil {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, err.Error())
			writeAssistantProposalError(w, err)
			return
		}
		op := input.Proposal.Operations[0]
		permission := assistantProposalPermission(op.Kind)
		if err := userauth.Authorize(session.User.Role, permission); err != nil {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, "permission denied")
			writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
			return
		}
		project, err := stageStore.GetProject(r.Context(), projectID)
		if err != nil {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultFailed, err.Error())
			writeAssistantProposalError(w, err)
			return
		}
		if project.CurrentRevisionID != input.Proposal.BaseRevisionID {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, errAssistantProposalStale.Error())
			writeAssistantProposalError(w, errAssistantProposalStale)
			return
		}
		result, err := applyAssistantProposalOperation(r, stageStore, projectID, session, input.Proposal.BaseRevisionID, op)
		if err != nil {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, assistantProposalAuditResult(err), err.Error())
			writeAssistantProposalError(w, err)
			return
		}
		appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultSuccess, "")
		writeJSON(w, http.StatusOK, map[string]any{
			"applied": true,
			"kind":    op.Kind,
			"result":  result,
		})
	}))
}

func assistantProposalPermission(kind assistant.ProposalOperationKind) userauth.Permission {
	switch kind {
	case assistant.ProposalNoteDraft, assistant.ProposalChecklistDraft:
		return userauth.PermissionNoteWrite
	default:
		return userauth.PermissionProjectEdit
	}
}

func applyAssistantProposalOperation(r *http.Request, stageStore *store.Store, projectID string, session userauth.Session, baseRevisionID string, op assistant.ProposalOperation) (any, error) {
	if strings.TrimSpace(op.TargetID) != "" {
		return nil, fmt.Errorf("%w: Slice C1 supports create proposals only", errAssistantProposalApplyUnsupported)
	}
	switch op.Kind {
	case assistant.ProposalCueDraft:
		var body cueWriteRequest
		if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
			return nil, err
		}
		revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Assistant proposal apply")
		if err != nil {
			return nil, err
		}
		cue, actions := body.toDomain(revision.ID, "")
		created, err := stageStore.CreateCueWithActions(r.Context(), cue, actions)
		if err != nil {
			return nil, err
		}
		return map[string]any{"revision": makeRevisionView(revision), "cue": makeCueView(created)}, nil

	case assistant.ProposalRoutingDraft:
		baseRevision, err := stageStore.GetRevision(r.Context(), strings.TrimSpace(baseRevisionID))
		if err != nil {
			return nil, err
		}
		if baseRevision.Status != domain.RevisionDraft {
			return nil, errAssistantProposalDraftRequired
		}
		var body routeCreateRequest
		if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
			return nil, err
		}
		revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Assistant proposal apply")
		if err != nil {
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
		actions := make([]domain.RouteAction, 0, len(body.Actions))
		for index, action := range body.Actions {
			parameters := action.Parameters
			if len(parameters) == 0 {
				parameters = json.RawMessage(`{}`)
			}
			actions = append(actions, domain.RouteAction{OrderIndex: index, OutputID: action.OutputID, CueID: action.CueID, Parameters: parameters})
		}
		created, err := stageStore.CreateRouteWithActions(r.Context(), domain.Route{
			RevisionID: revision.ID, Name: strings.TrimSpace(body.Name), InputID: strings.TrimSpace(body.InputID),
			ConditionDefinition: body.ConditionDefinition, TransformDefinition: body.TransformDefinition,
			DelayMS: body.DelayMS, DebounceMS: body.DebounceMS, PriorityClass: body.PriorityClass,
			ErrorPolicy: body.ErrorPolicy, Enabled: body.Enabled,
		}, actions)
		if err != nil {
			return nil, err
		}
		return map[string]any{"revision": makeRevisionView(revision), "route": routeView(created)}, nil

	case assistant.ProposalDeviceMappingDraft:
		var body targetCreateRequest
		if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
			return nil, err
		}
		if len(body.Configuration) == 0 {
			body.Configuration = json.RawMessage(`{}`)
		}
		created, err := stageStore.CreateAlias(r.Context(), domain.ProjectDeviceAlias{
			ProjectID: projectID, LogicalName: strings.TrimSpace(body.LogicalName), LogicalType: strings.TrimSpace(body.LogicalType),
			TargetRef: strings.TrimSpace(body.TargetRef), GroupName: strings.TrimSpace(body.GroupName), ProjectConfig: body.Configuration,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"target": targetView(created)}, nil

	case assistant.ProposalNoteDraft:
		var body createNoteRequest
		if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
			return nil, err
		}
		created, err := stageStore.CreateNote(r.Context(), projectID, store.CreateNoteParams{
			SessionID: body.SessionID, CueID: body.CueID, Category: body.Category,
			Body: body.Body, CreatedBy: session.User.Username,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"note": viewNote(created)}, nil

	case assistant.ProposalChecklistDraft:
		var body assistantChecklistPayload
		if err := decodeAssistantProposalPayload(op.Payload, &body); err != nil {
			return nil, err
		}
		checklist, err := renderAssistantChecklist(body.Items)
		if err != nil {
			return nil, err
		}
		created, err := stageStore.CreateNote(r.Context(), projectID, store.CreateNoteParams{
			SessionID: body.SessionID, CueID: body.CueID, Category: "ASSISTANT_CHECKLIST",
			Body: checklist, CreatedBy: session.User.Username,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{"note": viewNote(created)}, nil

	case assistant.ProposalTemplateDraft:
		return nil, fmt.Errorf("%w: current-project template apply has no canonical mutation surface", errAssistantProposalApplyUnsupported)
	default:
		return nil, assistant.ErrInvalidContract
	}
}

func decodeAssistantProposalPayload(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", errAssistantProposalPayloadInvalid, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%w: payload must contain exactly one JSON value", errAssistantProposalPayloadInvalid)
	}
	return nil
}

func renderAssistantChecklist(items []string) (string, error) {
	if len(items) == 0 || len(items) > 64 {
		return "", fmt.Errorf("%w: checklist requires 1-64 items", errAssistantProposalPayloadInvalid)
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || len(item) > 512 {
			return "", fmt.Errorf("%w: checklist items must be non-empty and <= 512 bytes", errAssistantProposalPayloadInvalid)
		}
		lines = append(lines, "- [ ] "+item)
	}
	return strings.Join(lines, "\n"), nil
}

func appendAssistantProposalAudit(r *http.Request, audit *securityaudit.Service, session userauth.Session, projectID string, proposal assistant.DraftProposal, result, reason string) {
	if audit == nil {
		return
	}
	kind := ""
	if len(proposal.Operations) == 1 {
		kind = string(proposal.Operations[0].Kind)
	}
	_, _ = audit.Append(r.Context(), securityaudit.Event{
		EventType: "assistant.proposal.apply", ActorUserID: session.User.ID, ActorUsername: session.User.Username,
		Source: "operator_web", ResourceType: "project", ResourceID: projectID,
		Result: result, Reason: reason,
		Metadata: map[string]any{"kind": kind, "base_revision_id": proposal.BaseRevisionID, "operation_count": len(proposal.Operations)},
	})
}

func assistantProposalAuditResult(err error) string {
	switch {
	case errors.Is(err, assistant.ErrProposalExpired), errors.Is(err, assistant.ErrProposalAtomicBatchRequired),
		errors.Is(err, errAssistantProposalStale), errors.Is(err, errAssistantProposalApplyUnsupported),
		errors.Is(err, errAssistantProposalDraftRequired), errors.Is(err, errAssistantProposalPayloadInvalid),
		errors.Is(err, assistant.ErrInvalidContract), errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrRevisionFrozen),
		errors.Is(err, domain.ErrShowConfigurationLocked):
		return securityaudit.ResultRejected
	default:
		return securityaudit.ResultFailed
	}
}

func writeAssistantProposalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, assistant.ErrInvalidContract), errors.Is(err, errAssistantProposalPayloadInvalid), errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "ASSISTANT_PROPOSAL_INVALID"})
	case errors.Is(err, assistant.ErrProposalExpired):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "ASSISTANT_PROPOSAL_EXPIRED"})
	case errors.Is(err, assistant.ErrProposalAtomicBatchRequired):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "ASSISTANT_PROPOSAL_ATOMIC_BATCH_REQUIRED"})
	case errors.Is(err, errAssistantProposalStale):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "ASSISTANT_PROPOSAL_STALE"})
	case errors.Is(err, errAssistantProposalDraftRequired):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "ASSISTANT_PROPOSAL_DRAFT_REQUIRED"})
	case errors.Is(err, errAssistantProposalApplyUnsupported):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "ASSISTANT_PROPOSAL_APPLY_UNSUPPORTED"})
	case errors.Is(err, domain.ErrShowConfigurationLocked):
		writeJSON(w, http.StatusLocked, map[string]any{"error_code": "SHOW_CONFIGURATION_LOCKED"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "NOT_FOUND"})
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrRevisionFrozen):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CONFLICT"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "ASSISTANT_PROPOSAL_APPLY_FAILED"})
	}
}
