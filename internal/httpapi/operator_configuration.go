package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type targetCreateRequest struct {
	LogicalName   string          `json:"logical_name"`
	LogicalType   string          `json:"logical_type"`
	TargetRef     string          `json:"target_ref,omitempty"`
	GroupName     string          `json:"group_name,omitempty"`
	Configuration json.RawMessage `json:"configuration"`
}

type inputCreateRequest struct {
	Name        string          `json:"name"`
	SourceRef   string          `json:"source_ref"`
	EventType   string          `json:"event_type"`
	ValueSchema json.RawMessage `json:"value_schema"`
	Enabled     bool            `json:"enabled"`
}

type outputCreateRequest struct {
	Name          string          `json:"name"`
	TargetRef     string          `json:"target_ref"`
	CapabilityKey string          `json:"capability_key"`
	ValueSchema   json.RawMessage `json:"value_schema"`
	Criticality   string          `json:"criticality"`
}

type routeActionCreateRequest struct {
	OutputID   *string         `json:"output_id,omitempty"`
	CueID      *string         `json:"cue_id,omitempty"`
	Parameters json.RawMessage `json:"parameters"`
}

type routeCreateRequest struct {
	Name                string                     `json:"name"`
	InputID             string                     `json:"input_id"`
	ConditionDefinition json.RawMessage            `json:"condition_definition"`
	TransformDefinition json.RawMessage            `json:"transform_definition"`
	DelayMS             *int64                     `json:"delay_ms,omitempty"`
	DebounceMS          *int64                     `json:"debounce_ms,omitempty"`
	PriorityClass       domain.PriorityClass       `json:"priority_class"`
	ErrorPolicy         json.RawMessage            `json:"error_policy"`
	Enabled             bool                       `json:"enabled"`
	Actions             []routeActionCreateRequest `json:"actions"`
}

type targetConfigurationView struct {
	ID            string          `json:"alias_id"`
	LogicalName   string          `json:"logical_name"`
	LogicalType   string          `json:"logical_type"`
	TargetRef     string          `json:"target_ref,omitempty"`
	GroupName     string          `json:"group_name,omitempty"`
	Configuration json.RawMessage `json:"configuration"`
}

type inputConfigurationView struct {
	ID          string          `json:"input_id"`
	RevisionID  string          `json:"revision_id"`
	Name        string          `json:"name"`
	SourceRef   string          `json:"source_ref"`
	EventType   string          `json:"event_type"`
	ValueSchema json.RawMessage `json:"value_schema"`
	Enabled     bool            `json:"enabled"`
}

type outputConfigurationView struct {
	ID            string          `json:"output_id"`
	RevisionID    string          `json:"revision_id"`
	Name          string          `json:"name"`
	TargetRef     string          `json:"target_ref"`
	CapabilityKey string          `json:"capability_key"`
	ValueSchema   json.RawMessage `json:"value_schema"`
	Criticality   string          `json:"criticality"`
}

type routeActionConfigurationView struct {
	ID         string          `json:"route_action_id"`
	OutputID   *string         `json:"output_id,omitempty"`
	CueID      *string         `json:"cue_id,omitempty"`
	Parameters json.RawMessage `json:"parameters"`
}

type routeConfigurationView struct {
	ID                  string                         `json:"route_id"`
	RevisionID          string                         `json:"revision_id"`
	Name                string                         `json:"name"`
	InputID             string                         `json:"input_id"`
	ConditionDefinition json.RawMessage                `json:"condition_definition"`
	TransformDefinition json.RawMessage                `json:"transform_definition"`
	DelayMS             *int64                         `json:"delay_ms,omitempty"`
	DebounceMS          *int64                         `json:"debounce_ms,omitempty"`
	PriorityClass       domain.PriorityClass           `json:"priority_class"`
	ErrorPolicy         json.RawMessage                `json:"error_policy"`
	Enabled             bool                           `json:"enabled"`
	Actions             []routeActionConfigurationView `json:"actions"`
}

func WithOperatorConfiguration(auth *userauth.Service, stageStore *store.Store) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil {
			return
		}
		registerOperatorConfigurationRoutes(s.mux, auth, stageStore)
	}
}

func registerOperatorConfigurationRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/configuration", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, revision, ok := loadProjectAndRevision(w, r, stageStore)
		if !ok {
			return
		}
		aliases, err := stageStore.ListAliases(r.Context(), project.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CONFIGURATION_UNAVAILABLE"})
			return
		}
		inputs, err := stageStore.ListInputs(r.Context(), revision.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CONFIGURATION_UNAVAILABLE"})
			return
		}
		outputs, err := stageStore.ListOutputs(r.Context(), revision.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CONFIGURATION_UNAVAILABLE"})
			return
		}
		routes, err := stageStore.ListRoutes(r.Context(), revision.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CONFIGURATION_UNAVAILABLE"})
			return
		}
		cues, err := stageStore.ListCues(r.Context(), revision.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CONFIGURATION_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision": makeRevisionView(revision),
			"targets":  targetViews(aliases),
			"inputs":   inputViews(inputs),
			"outputs":  outputViews(outputs),
			"routes":   routeViews(routes),
			"cues":     cueConfigurationChoices(cues),
		})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/targets", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		if _, err := stageStore.GetProject(r.Context(), projectID); err != nil {
			writeProjectStoreError(w, err)
			return
		}
		var body targetCreateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		if len(body.Configuration) == 0 {
			body.Configuration = json.RawMessage(`{}`)
		}
		created, err := stageStore.CreateAlias(r.Context(), domain.ProjectDeviceAlias{
			ProjectID: projectID, LogicalName: strings.TrimSpace(body.LogicalName),
			LogicalType: strings.TrimSpace(body.LogicalType), TargetRef: strings.TrimSpace(body.TargetRef),
			GroupName: strings.TrimSpace(body.GroupName), ProjectConfig: body.Configuration,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "TARGET_CREATE_FAILED"})
			return
		}
		writeJSON(w, http.StatusCreated, targetView(created))
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/inputs", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Operator routing edit")
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		var body inputCreateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		if len(body.ValueSchema) == 0 {
			body.ValueSchema = json.RawMessage(`{}`)
		}
		created, err := stageStore.CreateInput(r.Context(), domain.InputDefinition{
			RevisionID: revision.ID, Name: strings.TrimSpace(body.Name), SourceRef: strings.TrimSpace(body.SourceRef),
			EventType: strings.TrimSpace(body.EventType), ValueSchema: body.ValueSchema, Enabled: body.Enabled,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INPUT_CREATE_FAILED"})
			return
		}
		writeJSON(w, http.StatusCreated, inputView(created))
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/outputs", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Operator routing edit")
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		var body outputCreateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		if len(body.ValueSchema) == 0 {
			body.ValueSchema = json.RawMessage(`{}`)
		}
		created, err := stageStore.CreateOutput(r.Context(), domain.OutputDefinition{
			RevisionID: revision.ID, Name: strings.TrimSpace(body.Name), TargetRef: strings.TrimSpace(body.TargetRef),
			CapabilityKey: strings.TrimSpace(body.CapabilityKey), ValueSchema: body.ValueSchema, Criticality: strings.TrimSpace(body.Criticality),
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "OUTPUT_CREATE_FAILED"})
			return
		}
		writeJSON(w, http.StatusCreated, outputView(created))
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/routes", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		projectID := strings.TrimSpace(r.PathValue("project_id"))
		revision, err := stageStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Operator routing edit")
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		var body routeCreateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
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
			params := action.Parameters
			if len(params) == 0 {
				params = json.RawMessage(`{}`)
			}
			actions = append(actions, domain.RouteAction{OrderIndex: index, OutputID: action.OutputID, CueID: action.CueID, Parameters: params})
		}
		created, err := stageStore.CreateRouteWithActions(r.Context(), domain.Route{
			RevisionID: revision.ID, Name: strings.TrimSpace(body.Name), InputID: strings.TrimSpace(body.InputID),
			ConditionDefinition: body.ConditionDefinition, TransformDefinition: body.TransformDefinition,
			DelayMS: body.DelayMS, DebounceMS: body.DebounceMS, PriorityClass: body.PriorityClass,
			ErrorPolicy: body.ErrorPolicy, Enabled: body.Enabled,
		}, actions)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "ROUTE_CREATE_FAILED"})
			return
		}
		writeJSON(w, http.StatusCreated, routeView(created))
	}))
}

func targetView(value domain.ProjectDeviceAlias) targetConfigurationView {
	return targetConfigurationView{ID: value.ID, LogicalName: value.LogicalName, LogicalType: value.LogicalType, TargetRef: value.TargetRef, GroupName: value.GroupName, Configuration: value.ProjectConfig}
}
func targetViews(values []domain.ProjectDeviceAlias) []targetConfigurationView {
	out := make([]targetConfigurationView, 0, len(values)); for _, value := range values { out = append(out, targetView(value)) }; return out
}
func inputView(value domain.InputDefinition) inputConfigurationView {
	return inputConfigurationView{ID: value.ID, RevisionID: value.RevisionID, Name: value.Name, SourceRef: value.SourceRef, EventType: value.EventType, ValueSchema: value.ValueSchema, Enabled: value.Enabled}
}
func inputViews(values []domain.InputDefinition) []inputConfigurationView {
	out := make([]inputConfigurationView, 0, len(values)); for _, value := range values { out = append(out, inputView(value)) }; return out
}
func outputView(value domain.OutputDefinition) outputConfigurationView {
	return outputConfigurationView{ID: value.ID, RevisionID: value.RevisionID, Name: value.Name, TargetRef: value.TargetRef, CapabilityKey: value.CapabilityKey, ValueSchema: value.ValueSchema, Criticality: value.Criticality}
}
func outputViews(values []domain.OutputDefinition) []outputConfigurationView {
	out := make([]outputConfigurationView, 0, len(values)); for _, value := range values { out = append(out, outputView(value)) }; return out
}
func routeView(value domain.Route) routeConfigurationView {
	actions := make([]routeActionConfigurationView, 0, len(value.Actions))
	for _, action := range value.Actions { actions = append(actions, routeActionConfigurationView{ID: action.ID, OutputID: action.OutputID, CueID: action.CueID, Parameters: action.Parameters}) }
	return routeConfigurationView{ID: value.ID, RevisionID: value.RevisionID, Name: value.Name, InputID: value.InputID, ConditionDefinition: value.ConditionDefinition, TransformDefinition: value.TransformDefinition, DelayMS: value.DelayMS, DebounceMS: value.DebounceMS, PriorityClass: value.PriorityClass, ErrorPolicy: value.ErrorPolicy, Enabled: value.Enabled, Actions: actions}
}
func routeViews(values []domain.Route) []routeConfigurationView {
	out := make([]routeConfigurationView, 0, len(values)); for _, value := range values { out = append(out, routeView(value)) }; return out
}
func cueConfigurationChoices(values []domain.Cue) []cueSummaryView {
	out := make([]cueSummaryView, 0, len(values)); for _, value := range values { out = append(out, cueSummaryView{ID: value.ID, DisplayLabel: value.DisplayLabel, Name: value.Name, OrderIndex: value.OrderIndex}) }; return out
}
