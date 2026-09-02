package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
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
		registerOperatorExecutionEnvironmentCaptureRoutes(s.mux, auth, stageStore, vault)
	}
}

func registerOperatorExecutionEnvironmentCaptureRoutes(mux *http.ServeMux, auth *userauth.Service, stageStore *store.Store, vault *stagevault.Vault) {
	base := "/api/v1/projects/{project_id}/revisions/{revision_id}/execution-environments/{execution_environment_id}/assets/{asset_key}"

	mux.HandleFunc("GET "+base+"/vault-status", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		_, _, environment, asset, ok := loadExecutionEnvironmentAsset(w, r, stageStore)
		if !ok {
			return
		}
		status := map[string]any{
			"execution_environment_id": environment.ID,
			"asset_key":                asset.Key,
			"capture_policy":           asset.CapturePolicy,
			"content_hash":             asset.ContentHash,
			"size_bytes":               asset.SizeBytes,
			"vault_available":          false,
		}
		if asset.CapturePolicy != executionenv.CaptureContentBound || asset.SizeBytes == nil {
			status["reason"] = "REFERENCE_ONLY"
			writeJSON(w, http.StatusOK, status)
			return
		}

		object, err := stageStore.GetVaultObject(r.Context(), asset.ContentHash)
		if errors.Is(err, domain.ErrNotFound) {
			status["reason"] = "NOT_IN_VAULT"
			writeJSON(w, http.StatusOK, status)
			return
		}
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_VAULT_STATUS_UNAVAILABLE"})
			return
		}
		if object.SizeBytes != *asset.SizeBytes {
			status["reason"] = "SIZE_MISMATCH"
			writeJSON(w, http.StatusOK, status)
			return
		}
		file, _, err := vault.OpenObject(r.Context(), asset.ContentHash)
		if err != nil {
			status["reason"] = "VAULT_OBJECT_UNAVAILABLE"
			writeJSON(w, http.StatusOK, status)
			return
		}
		_ = file.Close()
		status["vault_available"] = true
		status["reason"] = "AVAILABLE"
		writeJSON(w, http.StatusOK, status)
	}))

	mux.HandleFunc("POST "+base+"/capture", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
		project, revision, environment, asset, ok := loadExecutionEnvironmentAsset(w, r, stageStore)
		if !ok {
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

		updated, err := stageStore.CaptureExecutionEnvironmentAsset(r.Context(), environment.ID, asset.Key, object.ContentHash, object.SizeBytes)
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

func loadExecutionEnvironmentAsset(w http.ResponseWriter, r *http.Request, stageStore *store.Store) (domain.Project, domain.ProjectRevision, store.ExecutionEnvironmentManifest, executionenv.AssetRequirement, bool) {
	project, revision, ok := loadExecutionEnvironmentRevision(w, r, stageStore)
	if !ok {
		return domain.Project{}, domain.ProjectRevision{}, store.ExecutionEnvironmentManifest{}, executionenv.AssetRequirement{}, false
	}
	environmentID := strings.TrimSpace(r.PathValue("execution_environment_id"))
	assetKey := strings.TrimSpace(r.PathValue("asset_key"))
	environment, err := stageStore.GetExecutionEnvironmentManifest(r.Context(), environmentID)
	if err != nil {
		writeExecutionEnvironmentStoreError(w, err, "EXECUTION_ENVIRONMENT_CAPTURE_FAILED")
		return domain.Project{}, domain.ProjectRevision{}, store.ExecutionEnvironmentManifest{}, executionenv.AssetRequirement{}, false
	}
	if environment.RevisionID != revision.ID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_NOT_FOUND"})
		return domain.Project{}, domain.ProjectRevision{}, store.ExecutionEnvironmentManifest{}, executionenv.AssetRequirement{}, false
	}
	for _, asset := range environment.Manifest.Assets {
		if asset.Key == assetKey {
			return project, revision, environment, asset, true
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_ASSET_NOT_FOUND"})
	return domain.Project{}, domain.ProjectRevision{}, store.ExecutionEnvironmentManifest{}, executionenv.AssetRequirement{}, false
}
