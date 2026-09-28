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

type executionEnvironmentRebuildPlanSeedRequest struct {
	SourceSnapshotID string `json:"source_snapshot_id"`
}

type executionEnvironmentRebuildPlanUpdateRequest struct {
	SourceSnapshotID string                           `json:"source_snapshot_id"`
	Plan             executionenv.AssistedRebuildPlan `json:"plan"`
}

type executionEnvironmentRebuildPlanView struct {
	RebuildPlanID          string                           `json:"rebuild_plan_id"`
	ExecutionEnvironmentID string                           `json:"execution_environment_id"`
	RevisionID             string                           `json:"revision_id"`
	SourceSnapshotID       string                           `json:"source_snapshot_id"`
	ContentSHA256          string                           `json:"content_sha256"`
	CreatedBy              string                           `json:"created_by"`
	CreatedAt              time.Time                        `json:"created_at"`
	UpdatedBy              string                           `json:"updated_by"`
	UpdatedAt              time.Time                        `json:"updated_at"`
	Plan                   executionenv.AssistedRebuildPlan `json:"plan"`
}

func registerOperatorExecutionEnvironmentRebuildPlanRoutes(
	mux *http.ServeMux,
	auth *userauth.Service,
	stageStore *store.Store,
) {
	path := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/rebuild-plan"

	mux.HandleFunc("GET "+path, withPermission(auth, userauth.PermissionProjectRead, func(
		w http.ResponseWriter, r *http.Request, _ userauth.Session,
	) {
		environment, ok := loadRebuildPlanEnvironment(w, r, stageStore)
		if !ok {
			return
		}
		plan, err := stageStore.GetExecutionEnvironmentRebuildPlan(r.Context(), environment.ID)
		if err != nil {
			writeExecutionEnvironmentRebuildPlanError(w, err, "EXECUTION_ENVIRONMENT_REBUILD_PLAN_UNAVAILABLE")
			return
		}
		writeJSON(w, http.StatusOK, makeExecutionEnvironmentRebuildPlanView(plan))
	}))

	mux.HandleFunc("POST "+path+"/seed", withPermission(auth, userauth.PermissionProjectEdit, func(
		w http.ResponseWriter, r *http.Request, session userauth.Session,
	) {
		environment, ok := loadRebuildPlanEnvironment(w, r, stageStore)
		if !ok {
			return
		}
		var body executionEnvironmentRebuildPlanSeedRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		sourceID := strings.TrimSpace(body.SourceSnapshotID)
		if sourceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_REBUILD_PLAN_SOURCE_REQUIRED",
			})
			return
		}
		source, err := stageStore.GetExecutionEnvironmentSnapshot(r.Context(), sourceID)
		if err != nil {
			writeExecutionEnvironmentRebuildPlanError(w, err, "EXECUTION_ENVIRONMENT_REBUILD_PLAN_SEED_FAILED")
			return
		}
		if source.EnvironmentManifestID != environment.ID || source.RevisionID != environment.RevisionID {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_NOT_FOUND",
			})
			return
		}
		seeded, err := executionenv.SeedAssistedRebuildPlan(source.Snapshot)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_REBUILD_PLAN_SOURCE_UNSUPPORTED",
			})
			return
		}
		stored, err := stageStore.UpsertExecutionEnvironmentRebuildPlan(
			r.Context(), environment.ID, source.ID, seeded, session.User.ID,
		)
		if err != nil {
			writeExecutionEnvironmentRebuildPlanError(w, err, "EXECUTION_ENVIRONMENT_REBUILD_PLAN_SEED_FAILED")
			return
		}
		writeJSON(w, http.StatusCreated, makeExecutionEnvironmentRebuildPlanView(stored))
	}))

	mux.HandleFunc("PUT "+path, withPermission(auth, userauth.PermissionProjectEdit, func(
		w http.ResponseWriter, r *http.Request, session userauth.Session,
	) {
		environment, ok := loadRebuildPlanEnvironment(w, r, stageStore)
		if !ok {
			return
		}
		var body executionEnvironmentRebuildPlanUpdateRequest
		if !decodeBoundedJSON(w, r, &body) {
			return
		}
		sourceID := strings.TrimSpace(body.SourceSnapshotID)
		if sourceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_REBUILD_PLAN_SOURCE_REQUIRED",
			})
			return
		}
		stored, err := stageStore.UpsertExecutionEnvironmentRebuildPlan(
			r.Context(), environment.ID, sourceID, body.Plan, session.User.ID,
		)
		if err != nil {
			writeExecutionEnvironmentRebuildPlanError(w, err, "EXECUTION_ENVIRONMENT_REBUILD_PLAN_UPDATE_FAILED")
			return
		}
		writeJSON(w, http.StatusOK, makeExecutionEnvironmentRebuildPlanView(stored))
	}))

	mux.HandleFunc("DELETE "+path, withPermission(auth, userauth.PermissionProjectEdit, func(
		w http.ResponseWriter, r *http.Request, _ userauth.Session,
	) {
		environment, ok := loadRebuildPlanEnvironment(w, r, stageStore)
		if !ok {
			return
		}
		if r.URL.Query().Get("confirm") != "true" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_REBUILD_PLAN_DELETE_CONFIRMATION_REQUIRED",
			})
			return
		}
		if err := stageStore.DeleteExecutionEnvironmentRebuildPlan(r.Context(), environment.ID); err != nil {
			writeExecutionEnvironmentRebuildPlanError(w, err, "EXECUTION_ENVIRONMENT_REBUILD_PLAN_DELETE_FAILED")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func loadRebuildPlanEnvironment(
	w http.ResponseWriter,
	r *http.Request,
	stageStore *store.Store,
) (store.ExecutionEnvironmentManifest, bool) {
	_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
	if !ok {
		return store.ExecutionEnvironmentManifest{}, false
	}
	environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
	environment, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
	if err != nil {
		writeExecutionEnvironmentRebuildPlanError(w, err, "EXECUTION_ENVIRONMENT_REBUILD_PLAN_UNAVAILABLE")
		return store.ExecutionEnvironmentManifest{}, false
	}
	if environment.RevisionID != revision.ID {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND",
		})
		return store.ExecutionEnvironmentManifest{}, false
	}
	return environment, true
}

func makeExecutionEnvironmentRebuildPlanView(
	item store.ExecutionEnvironmentRebuildPlan,
) executionEnvironmentRebuildPlanView {
	return executionEnvironmentRebuildPlanView{
		RebuildPlanID:          item.ID,
		ExecutionEnvironmentID: item.EnvironmentManifestID,
		RevisionID:             item.RevisionID,
		SourceSnapshotID:       item.SourceSnapshotID,
		ContentSHA256:          item.ContentSHA256,
		CreatedBy:              item.CreatedBy,
		CreatedAt:              item.CreatedAt,
		UpdatedBy:              item.UpdatedBy,
		UpdatedAt:              item.UpdatedAt,
		Plan:                   item.Plan,
	}
}

func writeExecutionEnvironmentRebuildPlanError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, domain.ErrShowConfigurationLocked):
		writeJSON(w, http.StatusLocked, map[string]any{"error_code": "SHOW_CONFIGURATION_LOCKED"})
	case errors.Is(err, domain.ErrRevisionFrozen):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "REVISION_NOT_DRAFT"})
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_REBUILD_PLAN_INVALID"})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_REBUILD_PLAN_NOT_FOUND"})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_REBUILD_PLAN_CONFLICT"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": fallback})
	}
}
