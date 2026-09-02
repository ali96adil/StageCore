package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	stagevault "github.com/ali96adil/StageCore/internal/vault"
)

// WithOperatorExecutionEnvironmentCapture adds the explicit local-file capture
// path for F-025. The request body is streamed through the existing immutable
// Vault; the manifest is promoted to CONTENT_BOUND only after Vault identity is
// verified by Store.
func WithOperatorExecutionEnvironmentCapture(auth *userauth.Service, stageStore *store.Store, vault *stagevault.Vault) Option {
	return func(s *Server) {
		if auth == nil || stageStore == nil || vault == nil {
			return
		}
		registerOperatorExecutionEnvironmentCaptureRoute(s.mux, auth, stageStore, vault)
	}
}

func registerOperatorExecutionEnvironmentCaptureRoute(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, vault *stagevault.Vault) {
	path := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/assets/{asset_key}/capture"
	mux.HandleFunc("POST "+path, withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
		if !ok {
			return
		}

		environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
		assetKey := strings.TrimSpace(r.PathValue("asset_key"))
		environment, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_CAPTURE_FAILED")
			return
		}
		if environment.RevisionID != revision.ID {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND"})
			return
		}
		assetFound := false
		for _, asset := range environment.Manifest.Assets {
			if asset.Key == assetKey {
				assetFound = true
				break
			}
		}
		if !assetFound {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_ASSET_NOT_FOUND"})
			return
		}

		// Reject known-invalid mutations before accepting bytes. Store repeats
		// these checks after import so a SHOW/freeze race still fails closed.
		if err := stageStore.RequireProjectConfigurationMutable(r.Context(), project.ID); err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_CAPTURE_FAILED")
			return
		}
		if revision.Status != domain.RevisionDraft {
			writeExecutionEnvironmentStoreError(w, domain.ErrRevisionFrozen, "EXECUTION_ENVIRONMENT_CAPTURE_FAILED")
			return
		}

		object, err := vault.ImportObject(r.Context(), r.Body)
		if err != nil {
			if errors.Is(err, storagehealth.ErrRuntimeReserve) {
				writeJSON(w, http.StatusInsufficientStorage, map[string]any{"error_code": "STORAGE_RUNTIME_RESERVE"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_CAPTURE_VAULT_FAILED"})
			return
		}

		updated, err := stageStore.CaptureExecutionEnvironmentAsset(r.Context(), environment.ID, assetKey, object.ContentHash, object.SizeBytes)
		if err != nil {
			writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_CAPTURE_FAILED")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"execution_environment": makeExecutionEnvironmentView(updated),
			"vault_object": map[string]any{
				"content_hash": object.ContentHash,
				"size_bytes":   object.SizeBytes,
			},
		})
	}))
}
