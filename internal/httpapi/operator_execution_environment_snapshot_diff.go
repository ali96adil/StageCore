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

type executionEnvironmentSnapshotSummaryView struct {
	SnapshotID                string                             `json:"snapshot_id"`
	ContentSHA256             string                             `json:"content_sha256"`
	CaptureStatus             executionenv.SnapshotCaptureStatus `json:"capture_status"`
	ReconstructionFingerprint string                             `json:"reconstruction_fingerprint,omitempty"`
	CreatedBy                 string                             `json:"created_by"`
	CreatedAt                 time.Time                          `json:"created_at"`
}

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
	collection := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/snapshots"
	mux.HandleFunc("GET "+collection, withPermission(auth, userauth.PermissionProjectRead, func(
		w http.ResponseWriter, r *http.Request, _ userauth.Session,
	) {
		_, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}
		environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
		environment, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_SNAPSHOTS_UNAVAILABLE")
			return
		}
		if environment.RevisionID != revision.ID {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND",
			})
			return
		}
		snapshots, err := stageStore.ListExecutionEnvironmentSnapshots(r.Context(), environment.ID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_SNAPSHOTS_UNAVAILABLE")
			return
		}
		views := make([]executionEnvironmentSnapshotSummaryView, 0, len(snapshots))
		for _, item := range snapshots {
			if item.RevisionID != revision.ID || item.EnvironmentManifestID != environment.ID {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_SCOPE_CONFLICT",
				})
				return
			}
			views = append(views, executionEnvironmentSnapshotSummaryView{
				SnapshotID:                item.ID,
				ContentSHA256:             item.ContentSHA256,
				CaptureStatus:             item.Snapshot.CaptureStatus,
				ReconstructionFingerprint: item.Snapshot.ReconstructionFingerprint,
				CreatedBy:                 item.CreatedBy,
				CreatedAt:                 item.CreatedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"execution_environment_id": environment.ID,
			"snapshots":                views,
		})
	}))

	path := collection + "/diff"
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
			writeExecutionEnvironmentSnapshotLookupError(w, err)
			return
		}
		after, err := stageStore.GetExecutionEnvironmentSnapshot(r.Context(), afterID)
		if err != nil {
			writeExecutionEnvironmentSnapshotLookupError(w, err)
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


func writeExecutionEnvironmentSnapshotLookupError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error_code": "EXECUTION_ENVIRONMENT_SNAPSHOT_NOT_FOUND",
		})
		return
	}
	writeExecutionEnvironmentStoreError(
		w, err, "EXECUTION_ENVIRONMENT_SNAPSHOT_UNAVAILABLE",
	)
}
