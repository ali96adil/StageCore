package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/simulationinput"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type simulationInputInjectRequest struct {
	RequestID       string          `json:"request_id"`
	InputID         string          `json:"input_id"`
	Value           json.RawMessage `json:"value"`
	ConfirmCritical bool            `json:"confirm_critical"`
}

func WithOperatorSimulationInputs(auth *userauth.Service, inputs *simulationinput.Service) Option {
	return func(s *Server) {
		if auth == nil || inputs == nil { return }
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/simulation/inputs", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			items, err := inputs.List(r.Context(), r.PathValue("project_id"))
			if err != nil {
				writeSimulationError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, items)
		}))
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/simulation/inputs/inject", withPermission(auth, userauth.PermissionRuntimeControl, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			var body simulationInputInjectRequest
			if !decodeBoundedJSON(w, r, &body) { return }
			result := inputs.Inject(r.Context(), simulationinput.InjectRequest{
				ProjectID: r.PathValue("project_id"),
				Issuer: session.User.ID,
				RequestID: strings.TrimSpace(body.RequestID),
				InputID: strings.TrimSpace(body.InputID),
				Value: body.Value,
				ConfirmCritical: body.ConfirmCritical,
			})
			writeRuntimeCommandResponse(w, http.StatusOK, result, map[string]any{"scope": "SIMULATION_ONLY", "source": "TEST"})
		}))
	}
}
