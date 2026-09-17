package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/assistant"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type assistantWorkspaceTaskRequest struct {
	RequestID         string                  `json:"request_id"`
	Kind              assistant.RequestKind   `json:"kind"`
	Scope             assistant.EvidenceScope `json:"scope,omitempty"`
	Prompt            string                  `json:"prompt"`
	RuntimeSnapshotID string                  `json:"runtime_snapshot_id,omitempty"`
	SessionID         string                  `json:"session_id,omitempty"`
	CueExecutionID    string                  `json:"cue_execution_id,omitempty"`
}

func registerOperatorAssistantWorkspaceRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, server *Server) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/assistant/status", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		project, err := stageStore.GetProject(r.Context(), projectID)
		if err != nil {
			writeAssistantWorkspaceError(w, err)
			return
		}
		availability := assistant.WorkspaceAvailability{}
		if current := server.assistantWorkspace; current != nil {
			availability = current.Availability()
		}
		providerState := "UNAVAILABLE"
		if availability.Provider {
			providerState = "AVAILABLE"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"provider_state": providerState,
			"tasks": map[string]bool{
				string(assistant.RequestExplain):  availability.Explain,
				string(assistant.RequestDiagnose): availability.Diagnose,
				string(assistant.RequestDraft):    availability.Draft,
			},
			"project_id":           project.ID,
			"revision_id":          project.CurrentRevisionID,
			"normal_operator_mode": true,
		})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/assistant/tasks", withAuthenticatedSession(auth, true, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
		if err := userauth.Authorize(session.User.Role, userauth.PermissionProjectRead); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
			return
		}
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		var input assistantWorkspaceTaskRequest
		if !decodeBoundedJSON(w, r, &input) {
			return
		}
		project, err := stageStore.GetProject(r.Context(), projectID)
		if err != nil {
			writeAssistantWorkspaceError(w, err)
			return
		}
		current := server.assistantWorkspace
		if current == nil {
			writeAssistantWorkspaceError(w, assistant.ErrProviderUnavailable)
			return
		}
		response, err := current.Respond(r.Context(), assistant.WorkspaceTaskInput{
			RequestID:         strings.TrimSpace(input.RequestID),
			ProjectID:         project.ID,
			RevisionID:        project.CurrentRevisionID,
			RuntimeSnapshotID: strings.TrimSpace(input.RuntimeSnapshotID),
			SessionID:         strings.TrimSpace(input.SessionID),
			CueExecutionID:    strings.TrimSpace(input.CueExecutionID),
			Kind:              input.Kind,
			Scope:             input.Scope,
			Prompt:            strings.TrimSpace(input.Prompt),
		})
		if err != nil {
			writeAssistantWorkspaceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"response": response})
	}))
}

func writeAssistantWorkspaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, assistant.ErrProviderUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "ASSISTANT_PROVIDER_UNAVAILABLE"})
	case errors.Is(err, assistant.ErrEvidenceUnavailable):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "ASSISTANT_EVIDENCE_UNAVAILABLE"})
	case errors.Is(err, assistant.ErrUngroundedEvidence):
		writeJSON(w, http.StatusBadGateway, map[string]any{"error_code": "ASSISTANT_RESPONSE_UNGROUNDED"})
	case errors.Is(err, assistant.ErrInvalidContract), errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "ASSISTANT_REQUEST_INVALID"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "NOT_FOUND"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "ASSISTANT_REQUEST_FAILED"})
	}
}
