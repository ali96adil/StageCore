package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type runtimeStartRequest struct {
	Mode      string `json:"mode"`
	Name      string `json:"name"`
	RequestID string `json:"request_id"`
}

type runtimeCommandRequest struct {
	RequestID            string  `json:"request_id"`
	ExpectedCurrentCueID *string `json:"expected_current_cue_id"`
	OperatorNote         *string `json:"operator_note"`
}

type runtimeJumpRequest struct {
	RequestID            string  `json:"request_id"`
	CueID                string  `json:"cue_id"`
	ExpectedCurrentCueID *string `json:"expected_current_cue_id"`
	OperatorNote         *string `json:"operator_note"`
	Confirm              bool    `json:"confirm"`
}

type runtimeExecutionView struct {
	ID          string                 `json:"cue_execution_id"`
	CueID       string                 `json:"cue_id"`
	Result      domain.ExecutionResult `json:"result"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at"`
}

type runtimeStatusView struct {
	Project         projectView           `json:"project"`
	Mode            string                `json:"mode"`
	Snapshot        *snapshotView         `json:"runtime_snapshot"`
	Session         *sessionView          `json:"session"`
	CurrentCue      *cueSummaryView       `json:"current_cue"`
	NextCue         *cueSummaryView       `json:"next_cue"`
	LatestExecution *runtimeExecutionView `json:"latest_execution"`
}

func WithOperatorRuntime(auth *userauth.Service, projectStore *store.Store, runtime *runtimecontrol.Service) Option {
	return func(s *Server) {
		if auth == nil || projectStore == nil || runtime == nil {
			return
		}
		registerOperatorRuntimeRoutes(s.mux, auth, projectStore, runtime)
	}
}

func registerOperatorRuntimeRoutes(mux *http.ServeMux, auth *userauth.Service, projectStore *store.Store, runtime *runtimecontrol.Service) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/runtime", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		view, err := buildRuntimeStatus(r, projectStore, r.PathValue("project_id"))
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/start", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeStartRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		mode := domain.SessionType(strings.ToUpper(strings.TrimSpace(body.Mode)))
		if mode == domain.SessionShow {
			if err := userauth.Authorize(session.User.Role, userauth.PermissionShowEnterExit); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return
			}
		}
		created, result := runtime.StartSession(r.Context(), runtimecontrol.StartRequest{
			ProjectID: r.PathValue("project_id"), Mode: mode, Name: body.Name,
			Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
		})
		writeRuntimeCommandResponse(w, http.StatusCreated, result, map[string]any{"session": makeSessionView(created)})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/stop-session", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeCommandRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeRuntimeSession(w, r, projectStore)
		if !ok {
			return
		}
		if active.Type == domain.SessionShow {
			if err := userauth.Authorize(session.User.Role, userauth.PermissionShowEnterExit); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return
			}
		}
		result := runtime.StopSession(r.Context(), runtimecontrol.StopRequest{
			SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, nil)
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/go", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeCommandRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeRuntimeSession(w, r, projectStore)
		if !ok {
			return
		}
		result := runtime.Go(r.Context(), runtimecontrol.CueRequest{
			SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
			ExpectedCurrentCueID: body.ExpectedCurrentCueID, OperatorNote: body.OperatorNote,
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, nil)
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/jump", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeJumpRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		if !body.Confirm {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CONFIRMATION_REQUIRED"})
			return
		}
		cueID := strings.TrimSpace(body.CueID)
		if cueID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CUE_REQUIRED"})
			return
		}
		active, ok := activeRuntimeSession(w, r, projectStore)
		if !ok {
			return
		}
		result := runtime.Go(r.Context(), runtimecontrol.CueRequest{
			SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
			ExpectedCurrentCueID: body.ExpectedCurrentCueID, RequestedCueID: &cueID, OperatorNote: body.OperatorNote,
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, map[string]any{"operation": "JUMP"})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/stop", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeCommandRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeRuntimeSession(w, r, projectStore)
		if !ok {
			return
		}
		result := runtime.StopCue(r.Context(), runtimecontrol.StopRequest{
			SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, nil)
	}))
}

func activeRuntimeSession(w http.ResponseWriter, r *http.Request, projectStore *store.Store) (domain.Session, bool) {
	active, err := projectStore.ActiveSessionForProject(r.Context(), r.PathValue("project_id"))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "RUNTIME_UNAVAILABLE"})
		return domain.Session{}, false
	}
	if active == nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "SESSION_NOT_ACTIVE"})
		return domain.Session{}, false
	}
	return *active, true
}

func buildRuntimeStatus(r *http.Request, projectStore *store.Store, projectID string) (runtimeStatusView, error) {
	project, err := projectStore.GetProject(r.Context(), projectID)
	if err != nil {
		return runtimeStatusView{}, err
	}
	latest, err := projectStore.LatestPublishedRuntimeSnapshotForProject(r.Context(), project.ID)
	if err != nil {
		return runtimeStatusView{}, err
	}
	view := runtimeStatusView{Project: makeProjectView(project), Mode: "EDIT"}
	if latest != nil {
		value := makeSnapshotView(*latest)
		view.Snapshot = &value
	}
	active, err := projectStore.ActiveSessionForProject(r.Context(), project.ID)
	if err != nil || active == nil {
		return view, err
	}
	view.Mode = string(active.Type)
	sessionValue := makeSessionView(*active)
	view.Session = &sessionValue
	activeSnapshot, err := projectStore.GetRuntimeSnapshot(r.Context(), active.RuntimeSnapshotID)
	if err != nil {
		return runtimeStatusView{}, err
	}
	snapshotValue := makeSnapshotView(activeSnapshot)
	view.Snapshot = &snapshotValue
	cues, err := projectStore.ListCues(r.Context(), activeSnapshot.RevisionID)
	if err != nil {
		return runtimeStatusView{}, err
	}
	view.CurrentCue, view.NextCue = currentAndNextCue(cues, active.CurrentCueID)
	executions, err := projectStore.ListCueExecutions(r.Context(), active.ID)
	if err != nil {
		return runtimeStatusView{}, err
	}
	if len(executions) > 0 {
		last := executions[len(executions)-1]
		view.LatestExecution = &runtimeExecutionView{
			ID: last.ID, CueID: last.CueID, Result: last.Result,
			StartedAt: last.StartedAt, CompletedAt: last.CompletedAt,
		}
	}
	return view, nil
}

func writeRuntimeCommandResponse(w http.ResponseWriter, successStatus int, result contracts.CommandResult, extra map[string]any) {
	payload := map[string]any{"result": result}
	for key, value := range extra {
		payload[key] = value
	}
	status := successStatus
	if result.Status == contracts.CommandRejected {
		status = http.StatusConflict
		if result.Error != nil && result.Error.ErrorCode == "NOT_FOUND" {
			status = http.StatusNotFound
		}
	} else if result.Status == contracts.CommandFailed {
		status = http.StatusServiceUnavailable
	} else if result.Status == contracts.CommandTimedOut {
		status = http.StatusGatewayTimeout
	} else if result.Status == contracts.CommandCancelled {
		status = http.StatusOK
	}
	writeJSON(w, status, payload)
}
