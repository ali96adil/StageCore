package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type executionEnvironmentCreateRequest struct {
	Manifest executionenv.Manifest `json:"manifest"`
}

type executionEnvironmentBindingRequest struct {
	MachineRoleID *string `json:"machine_role_id"`
}

type executionEnvironmentView struct {
	ID            string                `json:"execution_environment_id"`
	RevisionID    string                `json:"revision_id"`
	EnvironmentKey string               `json:"environment_key"`
	Name          string                `json:"name"`
	AdapterKey    string                `json:"adapter_key"`
	ApplicationKey string               `json:"application_key"`
	MachineRoleID *string               `json:"machine_role_id,omitempty"`
	ContentSHA256 string                `json:"content_sha256"`
	CreatedBy     string                `json:"created_by"`
	CreatedAt     time.Time             `json:"created_at"`
	Manifest      executionenv.Manifest `json:"manifest"`
}

func WithOperatorExecutionEnvironments(auth *userauth.Service, stageStore *store.Store) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil {
			return
		}
		registerOperatorExecutionEnvironmentRoutes(s.mux, auth, stageStore)
	}
}

func registerOperatorExecutionEnvironmentRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store) {
	collection := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments"
	item := collection + "/{execution_environment_id}"

	mux.HandleFunc("GET "+collection, withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		environments, err := stageStore.ListExecutionEnvironmentManifests(r.Context(), revision.ID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENTS_UNAVAILABLE")
			return
		}
		roles, err := stageStore.ListMachineRoles(r.Context(), project.ID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENTS_UNAVAILABLE")
			return
		}
		views := make([]executionEnvironmentView, 0, len(environments))
		for _, environment := range environments {
			views = append(views, makeExecutionEnvironmentView(environment))
		}
		roleViews := make([]machineRoleView, 0, len(roles))
		for _, role := range roles {
			roleViews = append(roleViews, makeMachineRoleView(role))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revision":               makeRevisionView(revision),
			"execution_environments": views,
			"machine_roles":           roleViews,
		})
	}))

	mux.HandleFunc("POST "+collection, withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
		_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		var body executionEnvironmentCreateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		created, err := stageStore.CreateExecutionEnvironmentManifest(r.Context(), revision.ID, body.Manifest, session.User.ID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_CREATE_FAILED")
			return
		}
		writeJSON(w, http.StatusCreated, makeExecutionEnvironmentView(created))
	}))

	mux.HandleFunc("PUT "+item+"/machine-role", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
		existing, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_BIND_FAILED")
			return
		}
		if existing.RevisionID != revision.ID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND"})
			return
		}
		var body executionEnvironmentBindingRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		updated, err := stageStore.SetExecutionEnvironmentMachineRole(r.Context(), environmentID, body.MachineRoleID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_BIND_FAILED")
			return
		}
		writeJSON(w, http.StatusOK, makeExecutionEnvironmentView(updated))
	}))

	mux.HandleFunc("DELETE "+item, withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		if r.URL.Query().Get("confirm") != "true" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_DELETE_CONFIRMATION_REQUIRED"})
			return
		}
		environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
		existing, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_DELETE_FAILED")
			return
		}
		if existing.RevisionID != revision.ID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND"})
			return
		}
		if err := stageStore.DeleteExecutionEnvironmentManifest(r.Context(), environmentID); err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_DELETE_FAILED")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func loadExecutionEnvironmentRevision(w http.ResponseWriter, r *http.Request, stageStore *store.Store) (domain.Project, domain.ProjectRevision, bool) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	revisionID := strings.TrimSpace(r.PathValue("revision_id"))
	project, err := stageStore.GetProject(r.Context(), projectID)
	if err != nil {
		writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENTS_UNAVAILABLE")
		return domain.Project{}, domain.ProjectRevision{}, false
	}
	revision, err := stageStore.GetRevision(r.Context(), revisionID)
	if err != nil {
		writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENTS_UNAVAILABLE")
		return domain.Project{}, domain.ProjectRevision{}, false
	}
	if revision.ProjectID != project.ID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_REVISION_NOT_FOUND"})
		return domain.Project{}, domain.ProjectRevision{}, false
	}
	return project, revision, true
}

func makeExecutionEnvironmentView(environment store.ExecutionEnvironmentManifest) executionEnvironmentView {
	return executionEnvironmentView{
		ID: environment.ID, RevisionID: environment.RevisionID,
		EnvironmentKey: environment.Manifest.EnvironmentKey, Name: environment.Manifest.Name,
		AdapterKey: environment.Manifest.AdapterKey, ApplicationKey: environment.Manifest.Application.Key,
		MachineRoleID: environment.MachineRoleID, ContentSHA256: environment.ContentSHA256,
		CreatedBy: environment.CreatedBy, CreatedAt: environment.CreatedAt, Manifest: environment.Manifest,
	}
}

func writeExecutionEnvironmentStoreError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, domain.ErrShowConfigurationLocked):
		writeJSON(w, http.StatusLocked, map[string]any{"error_code": "SHOW_CONFIGURATION_LOCKED"})
	case errors.Is(err, domain.ErrRevisionFrozen):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "REVISION_NOT_DRAFT"})
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_INVALID"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND"})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_CONFLICT"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": fallback})
	}
}
