package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

type visualEngineActionView struct {
	CueID   string        `json:"cue_id"`
	CueName string        `json:"cue_name"`
	Action  domain.Action `json:"action"`
}

type visualEngineWorkspaceView struct {
	ProjectID               string                        `json:"project_id"`
	RevisionID              string                        `json:"revision_id"`
	RevisionStatus          domain.RevisionStatus         `json:"revision_status"`
	ContractVersion         int                           `json:"contract_version"`
	Capabilities            []string                      `json:"capabilities"`
	NativeConfigured        bool                          `json:"native_configured"`
	ExternalEngineSupported bool                          `json:"external_engine_supported"`
	MachineRoles            []machineRoleView             `json:"machine_roles"`
	LiveSources             []deviceexperience.LiveSource `json:"live_sources"`
	Outputs                 []domain.OutputDefinition     `json:"outputs"`
	Actions                 []visualEngineActionView      `json:"actions"`
}

func WithOperatorVisualEngine(auth *userauth.Service, stageStore *store.Store, devices *deviceexperience.Repository) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil || devices == nil {
			return
		}
		registerOperatorVisualEngineRoutes(s.mux, auth, stageStore, devices)
	}
}

func registerOperatorVisualEngineRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, devices *deviceexperience.Repository) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/visual-engine", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		project, err := stageStore.GetProject(r.Context(), projectID)
		if err != nil {
			writeVisualEngineStoreError(w, err)
			return
		}
		if strings.TrimSpace(project.CurrentRevisionID) == "" {
			writeJSON(w, http.StatusConflict, map[string]any{"error_code": "VISUAL_ENGINE_REVISION_UNAVAILABLE"})
			return
		}
		revision, err := stageStore.GetRevision(r.Context(), project.CurrentRevisionID)
		if err != nil {
			writeVisualEngineStoreError(w, err)
			return
		}
		outputs, err := stageStore.ListOutputs(r.Context(), revision.ID)
		if err != nil {
			writeVisualEngineStoreError(w, err)
			return
		}
		cues, err := stageStore.ListCues(r.Context(), revision.ID)
		if err != nil {
			writeVisualEngineStoreError(w, err)
			return
		}
		roles, err := stageStore.ListMachineRoles(r.Context(), projectID)
		if err != nil {
			writeVisualEngineStoreError(w, err)
			return
		}
		liveSources, err := devices.ListLiveSources(r.Context(), projectID)
		if err != nil {
			writeVisualEngineStoreError(w, err)
			return
		}

		visualOutputs := make([]domain.OutputDefinition, 0)
		for _, output := range outputs {
			if visualengine.IsCapability(output.CapabilityKey) {
				visualOutputs = append(visualOutputs, output)
			}
		}
		visualActions := make([]visualEngineActionView, 0)
		for _, cue := range cues {
			for _, action := range cue.Actions {
				if visualengine.IsCapability(action.CapabilityKey) {
					visualActions = append(visualActions, visualEngineActionView{CueID: cue.ID, CueName: cue.Name, Action: action})
				}
			}
		}
		visualRoles := make([]machineRoleView, 0)
		for _, role := range roles {
			if machineRoleSupportsVisualEngine(role) {
				visualRoles = append(visualRoles, makeMachineRoleView(role))
			}
		}

		writeJSON(w, http.StatusOK, visualEngineWorkspaceView{
			ProjectID:               projectID,
			RevisionID:              revision.ID,
			RevisionStatus:          revision.Status,
			ContractVersion:         visualengine.ContractVersion1,
			Capabilities:            visualengine.CapabilityKeys(),
			NativeConfigured:        len(visualOutputs) > 0 || len(visualActions) > 0 || len(visualRoles) > 0,
			ExternalEngineSupported: true,
			MachineRoles:            visualRoles,
			LiveSources:             liveSources,
			Outputs:                 visualOutputs,
			Actions:                 visualActions,
		})
	}))
}

func machineRoleSupportsVisualEngine(role domain.MachineRole) bool {
	for _, capability := range role.RequiredCapabilities {
		if visualengine.IsCapability(capability) || deviceexperience.IsLiveSourceCapability(capability) {
			return true
		}
	}
	return false
}

func writeVisualEngineStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "VISUAL_ENGINE_PROJECT_NOT_FOUND"})
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "VISUAL_ENGINE_INVALID_REQUEST"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "VISUAL_ENGINE_WORKSPACE_UNAVAILABLE"})
	}
}
