package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/publish"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type actionWriteRequest struct {
	ActionID       string          `json:"action_id"`
	OrderIndex     int             `json:"order_index"`
	ExecutionMode  string          `json:"execution_mode"`
	TargetRef      string          `json:"target_ref"`
	CapabilityKey  string          `json:"capability_key"`
	Parameters     json.RawMessage `json:"parameters"`
	TimeoutPolicy  json.RawMessage `json:"timeout_policy"`
	ErrorPolicy    json.RawMessage `json:"error_policy"`
	PriorityClass  string          `json:"priority_class"`
	Enabled        bool            `json:"enabled"`
}

type cueWriteRequest struct {
	DisplayLabel    string               `json:"display_label"`
	Name            string               `json:"name"`
	OrderIndex      int                  `json:"order_index"`
	CueType         string               `json:"cue_type"`
	Criticality     string               `json:"criticality"`
	Enabled         bool                 `json:"enabled"`
	ExecutionPolicy json.RawMessage      `json:"execution_policy"`
	NotesSummary    string               `json:"notes_summary"`
	Actions         []actionWriteRequest `json:"actions"`
}

type duplicateCueRequest struct {
	DisplayLabel string `json:"display_label"`
	Name         string `json:"name"`
	OrderIndex   int    `json:"order_index"`
}

type cueView struct {
	ID              string          `json:"cue_id"`
	RevisionID      string          `json:"revision_id"`
	DisplayLabel    string          `json:"display_label"`
	Name            string          `json:"name"`
	OrderIndex      int             `json:"order_index"`
	CueType         string          `json:"cue_type"`
	Criticality     string          `json:"criticality"`
	Enabled         bool            `json:"enabled"`
	ExecutionPolicy json.RawMessage `json:"execution_policy"`
	NotesSummary    string          `json:"notes_summary"`
	Actions         []actionView    `json:"actions"`
}

type actionView struct {
	ID             string          `json:"action_id"`
	OrderIndex     int             `json:"order_index"`
	ExecutionMode  string          `json:"execution_mode"`
	TargetRef      string          `json:"target_ref"`
	CapabilityKey  string          `json:"capability_key"`
	Parameters     json.RawMessage `json:"parameters"`
	TimeoutPolicy  json.RawMessage `json:"timeout_policy"`
	ErrorPolicy    json.RawMessage `json:"error_policy"`
	PriorityClass  string          `json:"priority_class"`
	Enabled        bool            `json:"enabled"`
}

func WithOperatorCuePublish(auth *userauth.Service, projectStore *store.Store, publisher *publish.Service) Option {
	return func(s *Server) {
		if auth == nil || projectStore == nil || publisher == nil {
			return
		}
		registerOperatorCuePublishRoutes(s.mux, auth, projectStore, publisher)
	}
}

func registerOperatorCuePublishRoutes(mux *http.ServeMux, auth *userauth.Service, projectStore *store.Store, publisher *publish.Service) {
	mux.HandleFunc("GET /api/v1/projects/{project_id}/cues", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, err := projectStore.GetProject(r.Context(), r.PathValue("project_id"))
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		cues, err := projectStore.ListCues(r.Context(), project.CurrentRevisionID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CUES_UNAVAILABLE"})
			return
		}
		items := make([]cueView, 0, len(cues))
		for _, cue := range cues {
			items = append(items, makeCueView(cue))
		}
		revision, err := projectStore.GetRevision(r.Context(), project.CurrentRevisionID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "REVISION_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"revision": makeRevisionView(revision), "cues": items})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/cues", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		projectID := r.PathValue("project_id")
		revision, err := projectStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Operator Web edit after Publish")
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		var body cueWriteRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		cue, actions := body.toDomain(revision.ID, "")
		created, err := projectStore.CreateCueWithActions(r.Context(), cue, actions)
		if err != nil {
			writeCueMutationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"revision": makeRevisionView(revision), "cue": makeCueView(created)})
	}))

	mux.HandleFunc("PUT /api/v1/projects/{project_id}/cues/{cue_id}", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		mapped, revision, ok := ensureEditableCue(w, r, projectStore, session)
		if !ok {
			return
		}
		var body cueWriteRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		cue, actions := body.toDomain(revision.ID, mapped.ID)
		updated, err := projectStore.ReplaceDraftCue(r.Context(), cue, actions)
		if err != nil {
			writeCueMutationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"revision": makeRevisionView(revision), "cue": makeCueView(updated)})
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/cues/{cue_id}/duplicate", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		mapped, revision, ok := ensureEditableCue(w, r, projectStore, session)
		if !ok {
			return
		}
		var body duplicateCueRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		created, err := projectStore.DuplicateDraftCue(r.Context(), revision.ID, mapped.ID, body.DisplayLabel, body.Name, body.OrderIndex)
		if err != nil {
			writeCueMutationError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"revision": makeRevisionView(revision), "cue": makeCueView(created)})
	}))

	mux.HandleFunc("DELETE /api/v1/projects/{project_id}/cues/{cue_id}", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		if strings.ToLower(strings.TrimSpace(r.URL.Query().Get("confirm"))) != "true" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CONFIRMATION_REQUIRED"})
			return
		}
		mapped, revision, ok := ensureEditableCue(w, r, projectStore, session)
		if !ok {
			return
		}
		if err := projectStore.DeleteDraftCue(r.Context(), revision.ID, mapped.ID); err != nil {
			writeCueMutationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/v1/projects/{project_id}/validation", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, err := projectStore.GetProject(r.Context(), r.PathValue("project_id"))
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		report, err := publisher.Validate(r.Context(), project.ID, project.CurrentRevisionID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "VALIDATION_UNAVAILABLE"})
			return
		}
		writeJSON(w, http.StatusOK, report)
	}))

	mux.HandleFunc("POST /api/v1/projects/{project_id}/publish", withPermission(auth, userauth.PermissionSnapshotPublish, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		project, err := projectStore.GetProject(r.Context(), r.PathValue("project_id"))
		if err != nil {
			writeProjectStoreError(w, err)
			return
		}
		created, report, err := publisher.Publish(r.Context(), project.ID, project.CurrentRevisionID, session.User.ID)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PUBLISH_FAILED"})
			return
		}
		if created.ID == "" && !report.Valid {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error_code": "VALIDATION_BLOCKED", "validation": report})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"runtime_snapshot": makeSnapshotView(created), "validation": report})
	}))
}

func ensureEditableCue(w http.ResponseWriter, r *http.Request, projectStore *store.Store, session userauth.Session) (domain.Cue, domain.ProjectRevision, bool) {
	projectID := r.PathValue("project_id")
	requestedCueID := r.PathValue("cue_id")
	project, err := projectStore.GetProject(r.Context(), projectID)
	if err != nil {
		writeProjectStoreError(w, err)
		return domain.Cue{}, domain.ProjectRevision{}, false
	}
	source, err := projectStore.GetCue(r.Context(), requestedCueID)
	if err != nil || source.RevisionID != project.CurrentRevisionID {
		writeProjectStoreError(w, domain.ErrNotFound)
		return domain.Cue{}, domain.ProjectRevision{}, false
	}
	revision, err := projectStore.EnsureProjectDraft(r.Context(), projectID, session.User.ID, "Operator Web edit after Publish")
	if err != nil {
		writeProjectStoreError(w, err)
		return domain.Cue{}, domain.ProjectRevision{}, false
	}
	if revision.ID == source.RevisionID {
		return source, revision, true
	}
	cues, err := projectStore.ListCues(r.Context(), revision.ID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CUES_UNAVAILABLE"})
		return domain.Cue{}, domain.ProjectRevision{}, false
	}
	for _, cue := range cues {
		if cue.OrderIndex == source.OrderIndex {
			return cue, revision, true
		}
	}
	writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CUE_FORK_MAPPING_FAILED"})
	return domain.Cue{}, domain.ProjectRevision{}, false
}

func (body cueWriteRequest) toDomain(revisionID, cueID string) (domain.Cue, []domain.Action) {
	cue := domain.Cue{
		ID: cueID, RevisionID: revisionID, DisplayLabel: body.DisplayLabel, Name: body.Name,
		OrderIndex: body.OrderIndex, CueType: body.CueType, Criticality: body.Criticality,
		Enabled: body.Enabled, ExecutionPolicy: body.ExecutionPolicy, NotesSummary: body.NotesSummary,
	}
	actions := make([]domain.Action, 0, len(body.Actions))
	for _, item := range body.Actions {
		actions = append(actions, domain.Action{
			ID: item.ActionID, OrderIndex: item.OrderIndex, ExecutionMode: item.ExecutionMode,
			TargetRef: item.TargetRef, CapabilityKey: item.CapabilityKey, Parameters: item.Parameters,
			TimeoutPolicy: item.TimeoutPolicy, ErrorPolicy: item.ErrorPolicy,
			PriorityClass: domain.PriorityClass(item.PriorityClass), Enabled: item.Enabled,
		})
	}
	return cue, actions
}

func makeCueView(cue domain.Cue) cueView {
	view := cueView{
		ID: cue.ID, RevisionID: cue.RevisionID, DisplayLabel: cue.DisplayLabel, Name: cue.Name,
		OrderIndex: cue.OrderIndex, CueType: cue.CueType, Criticality: cue.Criticality,
		Enabled: cue.Enabled, ExecutionPolicy: cue.ExecutionPolicy, NotesSummary: cue.NotesSummary,
		Actions: make([]actionView, 0, len(cue.Actions)),
	}
	for _, action := range cue.Actions {
		view.Actions = append(view.Actions, actionView{
			ID: action.ID, OrderIndex: action.OrderIndex, ExecutionMode: action.ExecutionMode,
			TargetRef: action.TargetRef, CapabilityKey: action.CapabilityKey, Parameters: action.Parameters,
			TimeoutPolicy: action.TimeoutPolicy, ErrorPolicy: action.ErrorPolicy,
			PriorityClass: string(action.PriorityClass), Enabled: action.Enabled,
		})
	}
	return view
}

func writeProjectStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "NOT_FOUND"})
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrRevisionFrozen):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CONFLICT"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECTS_UNAVAILABLE"})
	}
}

func writeCueMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "CUE_INVALID"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "CUE_NOT_FOUND"})
	case errors.Is(err, domain.ErrRevisionFrozen), errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CUE_CONFLICT"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CUE_UPDATE_FAILED"})
	}
}
