package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/simulationcontrol"
	"github.com/ali96adil/StageCore/internal/simulator"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type simulationStartRequest struct {
	Name         string `json:"name"`
	RequestID    string `json:"request_id"`
	StartKind    string `json:"start_kind"`
	StartCueID   string `json:"start_cue_id"`
	EndCueID     string `json:"end_cue_id"`
	CheckpointID string `json:"checkpoint_id"`
}

type simulationCueRequest struct {
	RequestID            string  `json:"request_id"`
	ExpectedCurrentCueID *string `json:"expected_current_cue_id"`
	RequestedCueID       *string `json:"requested_cue_id"`
	OperatorNote         *string `json:"operator_note"`
}

type simulationSessionRequest struct {
	RequestID string `json:"request_id"`
}

type simulationCheckpointRestoreRequest struct {
	CheckpointID string `json:"checkpoint_id"`
}

type simulationFaultRequest struct {
	TargetRef  string `json:"target_ref"`
	Capability string `json:"capability"`
	Behavior   string `json:"behavior"`
	DelayMS    int64  `json:"delay_ms"`
	ErrorCode  string `json:"error_code"`
	Message    string `json:"message"`
	Uses       int    `json:"uses"`
}

type simulationFaultClearRequest struct {
	TargetRef  string `json:"target_ref"`
	Capability string `json:"capability"`
}

type simulationTargetStateRequest struct {
	TargetRef string `json:"target_ref"`
	Online    bool   `json:"online"`
}

func WithOperatorSimulation(auth *userauth.Service, control *simulationcontrol.Service) Option {
	return func(s *Server) {
		if auth == nil || control == nil {
			return
		}
		registerOperatorSimulationRoutes(s.mux, auth, control)
	}
}

func registerOperatorSimulationRoutes(mux *http.ServeMux, auth *userauth.Service, control *simulationcontrol.Service) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/simulation", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		status, err := control.Status(r.Context(), r.PathValue("project_id"))
		if err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/start", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body simulationStartRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		created, result := control.Start(r.Context(), simulationcontrol.StartRequest{
			ProjectID: r.PathValue("project_id"),
			Name: body.Name,
			Issuer: session.User.ID,
			RequestID: strings.TrimSpace(body.RequestID),
			StartKind: domain.SessionStartPositionKind(strings.ToUpper(strings.TrimSpace(body.StartKind))),
			StartCueID: body.StartCueID,
			EndCueID: body.EndCueID,
			CheckpointID: body.CheckpointID,
		})
		writeRuntimeCommandResponse(w, http.StatusCreated, result, map[string]any{"session": created})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/go", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body simulationCueRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		result := control.Go(r.Context(), simulationcontrol.CueRequest{
			SessionID: active.ID,
			Issuer: session.User.ID,
			RequestID: strings.TrimSpace(body.RequestID),
			ExpectedCurrentCueID: body.ExpectedCurrentCueID,
			RequestedCueID: body.RequestedCueID,
			OperatorNote: body.OperatorNote,
		})
		writeRuntimeCommandResponse(w, http.StatusOK, result, nil)
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/stop", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		var body simulationSessionRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		result := control.Stop(r.Context(), simulationcontrol.StopRequest{SessionID: active.ID, Issuer: session.User.ID, RequestID: strings.TrimSpace(body.RequestID)})
		writeRuntimeCommandResponse(w, http.StatusOK, result, nil)
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/confirm-start", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		updated, err := control.ConfirmStartState(r.Context(), active.ID, session.User.ID)
		if err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": updated, "verified": false})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/checkpoints", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		checkpoint, err := control.CaptureCheckpoint(r.Context(), active.ID)
		if err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"checkpoint": checkpoint})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/restore", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		var body simulationCheckpointRestoreRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		checkpoint, err := control.RestoreCheckpoint(r.Context(), active.ID, body.CheckpointID)
		if err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"checkpoint": checkpoint, "replay": false})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/faults", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		var body simulationFaultRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		err := control.ConfigureFault(r.Context(), active.ID, simulator.FaultScenario{
			TargetRef: body.TargetRef,
			Capability: body.Capability,
			Behavior: body.Behavior,
			DelayMS: body.DelayMS,
			ErrorCode: body.ErrorCode,
			Message: body.Message,
			Uses: body.Uses,
		})
		if err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"configured": true, "scope": "SIMULATION_ONLY"})
	}))

	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/simulation/faults", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		var body simulationFaultClearRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		if err := control.ClearFault(r.Context(), active.ID, body.TargetRef, body.Capability); err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/targets/state", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		var body simulationTargetStateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		if err := control.SetTargetOnline(r.Context(), active.ID, body.TargetRef, body.Online); err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated": true, "scope": "SIMULATION_ONLY"})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/reset", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		active, ok := activeSimulationSession(w, r, control)
		if !ok {
			return
		}
		if err := control.ResetTwin(r.Context(), active.ID); err != nil {
			writeSimulationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reset": true, "scope": "SIMULATION_ONLY"})
	}))
}

func activeSimulationSession(w http.ResponseWriter, r *http.Request, control *simulationcontrol.Service) (domain.Session, bool) {
	status, err := control.Status(r.Context(), r.PathValue("project_id"))
	if err != nil {
		writeSimulationError(w, err)
		return domain.Session{}, false
	}
	if status.Session == nil || status.Session.Type != domain.SessionSimulation || status.Session.Status != domain.SessionActive {
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "SIMULATION_NOT_ACTIVE"})
		return domain.Session{}, false
	}
	return *status.Session, true
}

func writeSimulationError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	code := "SIMULATION_OPERATION_FAILED"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, domain.ErrInvalidInput):
		status, code = http.StatusBadRequest, "INVALID_REQUEST"
	case errors.Is(err, domain.ErrConflict):
		status, code = http.StatusConflict, "CONFLICT"
	}
	writeJSON(w, status, map[string]any{"error_code": code, "message": err.Error()})
}
