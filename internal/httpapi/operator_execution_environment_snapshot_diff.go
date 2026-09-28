package httpapi

import (
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type executionEnvironmentSnapshotDiffView struct {
	ExecutionEnvironmentID string                    `json:"execution_environment_id"`
	BeforeSnapshotID       string                    `json:"before_snapshot_id"`
	AfterSnapshotID        string                    `json:"after_snapshot_id"`
	Diff                   executionenv.SnapshotDiff `json:"diff"`
}

func registerOperatorExecutionEnvironmentSnapshotDiffRoutes(
	mux *http.ServeMux,
	auth *userauth.Service,
	stageStore *store.Store,
) {
	path := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/snapshots/diff"
	mux.HandleFunc("GET "+path, withPermission(auth, userauth.PermissionProjectRead, func(
		w http.ResponseWriter, r *http.Request, _ userauth.Session,
	) {
		_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
		environment, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_SNAPSHOT_DIFF_UNAVAILABLE")
			return
		}
		if environment.RevisionID != revision.ID {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND",
			})
			return
		}

		beforeID := strings.TrimSpace(r.URL.Query().Get("before_snapshot_id"))
		afterID := strings.TrimSpace(r.URL.Query().Get("after_snapshot_id"))
		if beforeID == "" || afterID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_DIFF_IDS_REQUIRED",
			})
			return
		}

		before, err := stageStore.GetExecutionEnvironmentSnapshot(r.Context(), beforeID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_NOT_FOUND",
			})
			return
		}
		after, err := stageStore.GetExecutionEnvironmentSnapshot(r.Context(), afterID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_NOT_FOUND",
			})
			return
		}
		if before.EnvironmentManifestID != environment.ID ||
			after.EnvironmentManifestID != environment.ID ||
			before.RevisionID != revision.ID ||
			after.RevisionID != revision.ID {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_NOT_FOUND",
			})
			return
		}

		diff, err := executionenv.DiffSnapshots(before.Snapshot, after.Snapshot)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_DIFF_INCOMPATIBLE",
			})
			return
		}
		writeJSON(w, http.StatusOK, executionEnvironmentSnapshotDiffView{
			ExecutionEnvironmentID: environment.ID,
			BeforeSnapshotID:       before.ID,
			AfterSnapshotID:        after.ID,
			Diff:                   diff,
		})
	}))
}
