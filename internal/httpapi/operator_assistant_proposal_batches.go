package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type assistantProposalBatchApplyRequest struct {
	Proposal assistant.DraftProposal `json:"proposal"`
	Apply    bool                    `json:"apply"`
}

func registerOperatorAssistantProposalBatchRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, audit *securityaudit.Service) {
	mux.HandleFunc("POST /api/v1/projects/{project_id}/assistant/proposals/apply-batch", withAuthenticatedSession(auth, true, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		var input assistantProposalBatchApplyRequest
		if !decodeBoundedJSON(w, r, &input) {
			return
		}
		if !input.Apply {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, "explicit atomic batch apply confirmation required")
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "ASSISTANT_PROPOSAL_CONFIRMATION_REQUIRED"})
			return
		}
		if err := input.Proposal.ValidateForBatchApply(time.Now().UTC()); err != nil {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, err.Error())
			writeAssistantProposalError(w, err)
			return
		}
		permissions := make([]userauth.Permission, 0, len(input.Proposal.Operations))
		for _, op := range input.Proposal.Operations {
			permission := assistantProposalPermission(op.Kind)
			permissions = append(permissions, permission)
			if err := userauth.Authorize(session.User.Role, permission); err != nil &&
				!scopedProjectRoleMayUsePermission(r, session.User.Role, permission) {
				appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, "permission denied")
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return
			}
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
		if assistantProposalBatchRequiresDraft(input.Proposal) {
			revision, err := stageStore.GetRevision(r.Context(), input.Proposal.BaseRevisionID)
			if err != nil {
				appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultFailed, err.Error())
				writeAssistantProposalError(w, err)
				return
			}
			if revision.Status != domain.RevisionDraft {
				appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultRejected, errAssistantProposalDraftRequired.Error())
				writeAssistantProposalError(w, errAssistantProposalDraftRequired)
				return
			}
		}
		mutations, err := buildAssistantProposalBatch(projectID, input.Proposal)
		if err != nil {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, assistantProposalAuditResult(err), err.Error())
			writeAssistantProposalError(w, err)
			return
		}
		ctx := store.WithCueDraftHistoryActor(
			projectMutationContextForPermissions(r, auth, session, permissions, nil, projectaccess.ProjectEdit),
			session.User.ID,
		)
		results, err := stageStore.ApplyAssistantProposalBatch(ctx, projectID, input.Proposal.BaseRevisionID, session.User.Username, mutations)
		if errors.Is(err, store.ErrProjectRevisionChanged) {
			err = errAssistantProposalStale
		}
		if err != nil {
			appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, assistantProposalAuditResult(err), err.Error())
			writeAssistantProposalError(w, err)
			return
		}
		appendAssistantProposalAudit(r, audit, session, projectID, input.Proposal, securityaudit.ResultSuccess, "")
		for _, permission := range permissions {
			if !recheckCommittedProjectMutation(w, r, auth, session, permission, nil, projectaccess.ProjectEdit) {
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"applied":         true,
			"atomic_batch":    true,
			"operation_count": len(results),
			"results":         assistantProposalBatchResultViews(input.Proposal, results),
		})
	}))
}

func assistantProposalBatchRequiresDraft(proposal assistant.DraftProposal) bool {
	for _, op := range proposal.Operations {
		switch op.Kind {
		case assistant.ProposalCueDraft, assistant.ProposalRoutingDraft:
			return true
		}
	}
	return false
}

func assistantProposalBatchResultViews(proposal assistant.DraftProposal, results []store.AssistantProposalMutationResult) []map[string]any {
	views := make([]map[string]any, 0, len(results))
	for i, result := range results {
		kind := assistant.ProposalOperationKind("")
		ref := result.Ref
		if i < len(proposal.Operations) {
			kind = proposal.Operations[i].Kind
			if ref == "" {
				ref = proposal.Operations[i].Ref
			}
		}
		views = append(views, map[string]any{
			"kind":      kind,
			"ref":       ref,
			"entity_id": result.EntityID,
			"updated":   result.Updated,
		})
	}
	return views
}
