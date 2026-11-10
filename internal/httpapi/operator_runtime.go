package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/runtimecontrol"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type runtimeStartRequest struct {
	RehearsalPurpose domain.RehearsalPurpose `json:"rehearsal_purpose,omitempty"`
	Mode             string                  `json:"mode"`
	Name             string                  `json:"name"`
	RequestID        string                  `json:"request_id"`
}

type runtimeCommandRequest struct {
	ExpectedSessionID    *string `json:"expected_session_id,omitempty"`
	RequestID            string  `json:"request_id"`
	ExpectedCurrentCueID *string `json:"expected_current_cue_id"`
	OperatorNote         *string `json:"operator_note"`
	Force                bool    `json:"force"`
	Confirm              string  `json:"confirm"`
}

type runtimeStopSessionRequest struct {
	runtimeCommandRequest
	RehearsalTabletStopSessionID   string `json:"rehearsal_tablet_stop_session_id,omitempty"`
	RehearsalExternalStopSessionID string `json:"rehearsal_external_stop_session_id,omitempty"`
}

type runtimeEmergencyBlackoutRequest struct {
	RequestID string `json:"request_id"`
	Enabled   bool   `json:"enabled"`
	Confirm   string `json:"confirm"`
}

type runtimeJumpRequest struct {
	ExpectedSessionID    *string `json:"expected_session_id,omitempty"`
	RequestID            string  `json:"request_id"`
	CueID                string  `json:"cue_id"`
	ExpectedCurrentCueID *string `json:"expected_current_cue_id"`
	OperatorNote         *string `json:"operator_note"`
	RehearsalSessionID   *string `json:"rehearsal_session_id,omitempty"`
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
	Project               projectView           `json:"project"`
	Mode                  string                `json:"mode"`
	Snapshot              *snapshotView         `json:"runtime_snapshot"`
	Session               *sessionView          `json:"session"`
	Cues                  []cueSummaryView      `json:"cues"`
	CurrentCue            *cueSummaryView       `json:"current_cue"`
	NextCue               *cueSummaryView       `json:"next_cue"`
	LatestExecution       *runtimeExecutionView `json:"latest_execution"`
	ManagedOutputBlackout bool                  `json:"managed_output_blackout"`
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
		permission := userauth.PermissionRuntimeControl
		needed := projectaccess.RehearsalControl
		if mode == domain.SessionShow {
			permission = userauth.PermissionShowEnterExit
			needed = projectaccess.ShowEnterExit
			if err := userauth.Authorize(session.User.Role, permission); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return
			}
		}
		grant, scopeErr := currentProjectRouteGrant(r)
		if scopeErr == nil && grant.access != nil {
			scopeErr = grant.access.Check(r.Context(), session.User.ID, r.PathValue("project_id"), needed)
		}
		if !writeProjectQualificationError(w, scopeErr) {
			return
		}
		ctx := projectMutationContext(r, auth, session, permission, nil, projectaccess.ProjectView, needed)
		created, result := runtime.StartSession(ctx, runtimecontrol.StartRequest{
			ProjectID: r.PathValue("project_id"), Mode: mode, Name: body.Name,
			RehearsalPurpose: body.RehearsalPurpose,
			Issuer:           session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
		})
		writeRuntimeCommandResponse(w, http.StatusCreated, result, map[string]any{"session": makeSessionView(created)})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/stop-session", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeStopSessionRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeRuntimeSession(w, r, projectStore)
		if !ok {
			return
		}
		permission := userauth.PermissionRuntimeControl
		needed := projectaccess.RehearsalControl
		if active.Type == domain.SessionShow {
			permission = userauth.PermissionShowEnterExit
			needed = projectaccess.ShowEnterExit
			if err := userauth.Authorize(session.User.Role, permission); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return
			}
		}
		if body.Force {
			if body.RehearsalTabletStopSessionID != "" || body.RehearsalExternalStopSessionID != "" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "FORCE_EXIT_WITH_STOP_ACTION"})
				return
			}
			if strings.TrimSpace(body.Confirm) != "FORCE_EXIT_WITHOUT_BLACKOUT" {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CONFIRMATION_REQUIRED"})
				return
			}
			if !qualifyRuntimeIntent(w, r, active.ID, projectaccess.RuntimeForceExit) {
				return
			}
			ctx := projectMutationContext(r, auth, session, permission, nil)
			result := runtime.ForceStopSession(ctx, runtimecontrol.StopRequest{
				SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
			})
			writeRuntimeCommandResponse(w, http.StatusOK, result, map[string]any{
				"forced":          true,
				"safety_bypassed": true,
			})
			return
		}
		if !qualifyRuntimeIntent(w, r, active.ID, projectaccess.RuntimeStopSession) {
			return
		}
		ctx := projectMutationContext(r, auth, session, permission, nil, projectaccess.ProjectView, needed)
		result := runtime.StopSession(ctx, runtimecontrol.StopRequest{
			SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
			RehearsalTabletStopSessionID:   body.RehearsalTabletStopSessionID,
			RehearsalExternalStopSessionID: body.RehearsalExternalStopSessionID,
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
		if body.ExpectedSessionID != nil && active.ID != *body.ExpectedSessionID {
			writeJSON(w, http.StatusConflict, map[string]any{"error_code": "RUNTIME_SESSION_CHANGED"})
			return
		}
		if !qualifyRuntimeIntent(w, r, active.ID, projectaccess.RuntimeGO) {
			return
		}
		result := runtime.Go(qualifiedCueDispatchContext(r, active.ID, projectaccess.RuntimeGO), runtimecontrol.CueRequest{
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
		if body.RehearsalSessionID != nil &&
			(active.Type != domain.SessionRehearsal || active.ID != *body.RehearsalSessionID) {
			writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CUE_CHECK_SESSION_CHANGED"})
			return
		}
		if body.ExpectedSessionID != nil && active.ID != *body.ExpectedSessionID {
			writeJSON(w, http.StatusConflict, map[string]any{"error_code": "RUNTIME_SESSION_CHANGED"})
			return
		}
		if !qualifyRuntimeIntent(w, r, active.ID, projectaccess.RuntimeJump) {
			return
		}
		result := runtime.Go(qualifiedCueDispatchContext(r, active.ID, projectaccess.RuntimeJump), runtimecontrol.CueRequest{
			SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID),
			ExpectedCurrentCueID: body.ExpectedCurrentCueID, RequestedCueID: &cueID, OperatorNote: body.OperatorNote,
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, map[string]any{"operation": "JUMP"})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/project-blackout", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeEmergencyBlackoutRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		requiredConfirmation := "CLEAR"
		if body.Enabled {
			requiredConfirmation = "BLACKOUT"
		}
		if strings.TrimSpace(body.Confirm) != requiredConfirmation {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CONFIRMATION_REQUIRED"})
			return
		}
		intent := projectaccess.RuntimeBlackoutClear
		if body.Enabled {
			intent = projectaccess.RuntimeBlackoutEngage
		}
		if !qualifyRuntimeIntent(w, r, "", intent) {
			return
		}
		var scoped []projectaccess.Permission
		if body.Enabled {
			scoped = []projectaccess.Permission{projectaccess.ProjectView, projectaccess.ShowBlackoutEngage}
		}
		ctx := projectMutationContext(r, auth, session, userauth.PermissionRuntimeControl, nil, scoped...)
		if !body.Enabled {
			ctx = runtimeBlackoutClearContext(ctx, r, auth, session, "")
		}
		result := runtime.ProjectBlackout(ctx, runtimecontrol.ProjectEmergencyRequest{
			ProjectID: r.PathValue("project_id"),
			Issuer:    session.User.ID,
			RequestID: strings.TrimSpace(body.RequestID),
			Enabled:   body.Enabled,
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, map[string]any{
			"sessionless":             true,
			"managed_output_blackout": body.Enabled,
		})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/runtime/emergency-blackout", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body runtimeEmergencyBlackoutRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		requiredConfirmation := "CLEAR"
		if body.Enabled {
			requiredConfirmation = "BLACKOUT"
		}
		if strings.TrimSpace(body.Confirm) != requiredConfirmation {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CONFIRMATION_REQUIRED"})
			return
		}
		active, ok := activeRuntimeSession(w, r, projectStore)
		if !ok {
			return
		}
		intent := projectaccess.RuntimeBlackoutClear
		if body.Enabled {
			intent = projectaccess.RuntimeBlackoutEngage
		}
		if !qualifyRuntimeIntent(w, r, active.ID, intent) {
			return
		}
		var scoped []projectaccess.Permission
		if body.Enabled {
			scoped = []projectaccess.Permission{projectaccess.ProjectView, projectaccess.ShowBlackoutEngage}
		}
		ctx := projectMutationContext(r, auth, session, userauth.PermissionRuntimeControl, nil, scoped...)
		if !body.Enabled {
			ctx = runtimeBlackoutClearContext(ctx, r, auth, session, active.ID)
		}
		result := runtime.EmergencyBlackout(ctx, runtimecontrol.EmergencyRequest{
			SessionID: active.ID,
			Issuer:    session.User.ID,
			RequestID: strings.TrimSpace(body.RequestID),
			Enabled:   body.Enabled,
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, map[string]any{"managed_output_blackout": body.Enabled})
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
		if !qualifyRuntimeIntent(w, r, active.ID, projectaccess.RuntimeStopCue) {
			return
		}
		needed := projectaccess.RehearsalControl
		if active.Type == domain.SessionShow {
			needed = projectaccess.ShowStop
		}
		ctx := projectMutationContext(r, auth, session, userauth.PermissionRuntimeControl, nil, projectaccess.ProjectView, needed)
		result := runtime.StopCue(ctx, runtimecontrol.StopRequest{
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
	view := runtimeStatusView{Project: makeProjectView(project), Mode: "EDIT", Cues: []cueSummaryView{}}
	if latest != nil {
		value := makeSnapshotView(*latest)
		view.Snapshot = &value
		publishedCues, err := projectStore.ListCues(r.Context(), latest.RevisionID)
		if err != nil {
			return runtimeStatusView{}, err
		}
		view.Cues = makeRuntimeCueSummaries(publishedCues)
	}
	active, err := projectStore.ActiveSessionForProject(r.Context(), project.ID)
	if err != nil || active == nil {
		return view, err
	}
	view.Mode = string(active.Type)
	sessionValue := makeSessionView(*active)
	view.Session = &sessionValue
	view.ManagedOutputBlackout, err = projectStore.SessionManagedOutputBlackout(r.Context(), active.ID)
	if err != nil {
		return runtimeStatusView{}, err
	}
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
	view.Cues = makeRuntimeCueSummaries(cues)
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

func makeRuntimeCueSummaries(cues []domain.Cue) []cueSummaryView {
	items := make([]cueSummaryView, 0, len(cues))
	for _, cue := range cues {
		if cue.Enabled {
			items = append(items, makeCueSummary(cue))
		}
	}
	return items
}

// A clear releases output restrictions. Recheck the browser and, in
// qualification mode, its exact project/session intent at dispatch time.
func runtimeBlackoutClearContext(ctx context.Context, r *http.Request, auth *userauth.Service, session userauth.Session, sessionID string) context.Context {
	token, _ := browserSessionToken(r)
	access := qualificationAccess(r, nil)
	projectID := r.PathValue("project_id")
	return capability.WithDispatchCheck(ctx, func(ctx context.Context) error {
		current, err := auth.ValidatePassive(ctx, token)
		if err != nil || current.ID != session.ID || current.User.ID != session.User.ID ||
			userauth.Authorize(current.User.Role, userauth.PermissionRuntimeControl) != nil {
			return userauth.ErrForbidden
		}
		if access != nil {
			return access.CheckRuntimeIntent(ctx, current.User.ID, projectID, sessionID, projectaccess.RuntimeBlackoutClear)
		}
		return nil
	})
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
		} else if result.Error != nil && result.Error.ErrorCode == "FORBIDDEN" {
			status = http.StatusForbidden
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
