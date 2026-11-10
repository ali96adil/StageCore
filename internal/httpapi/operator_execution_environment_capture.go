package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/projectaccess"
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

	mux.HandleFunc("GET "+base+"/vault-status", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
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
		deliverStatus := func() {
			if allowLiveProjectRead(w, r, auth, session, userauth.PermissionProjectRead) {
				writeJSON(w, http.StatusOK, status)
			}
		}
		if asset.CapturePolicy != executionenv.CaptureContentBound || asset.SizeBytes == nil {
			status["reason"] = "REFERENCE_ONLY"
			deliverStatus()
			return
		}

		object, err := stageStore.GetVaultObject(r.Context(), asset.ContentHash)
		if errors.Is(err, domain.ErrNotFound) {
			status["reason"] = "NOT_IN_VAULT"
			deliverStatus()
			return
		}
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_VAULT_STATUS_UNAVAILABLE"})
			return
		}
		if object.SizeBytes != *asset.SizeBytes {
			status["reason"] = "SIZE_MISMATCH"
			deliverStatus()
			return
		}
		file, _, err := vault.OpenObject(r.Context(), asset.ContentHash)
		if err != nil {
			status["reason"] = "VAULT_OBJECT_UNAVAILABLE"
			deliverStatus()
			return
		}
		_ = file.Close()
		status["vault_available"] = true
		status["reason"] = "AVAILABLE"
		deliverStatus()
	}))

	mux.HandleFunc("POST "+base+"/capture", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
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

		token, _ := browserSessionToken(r)
		body := &authorizedCaptureReader{source: r.Body, authorize: func() error {
			if _, err := currentProjectRouteGrant(r); err != nil {
				return err
			}
			current, err := auth.ValidatePassive(r.Context(), token)
			if err != nil || current.ID != session.ID || current.User.ID != session.User.ID ||
				userauth.Authorize(current.User.Role, userauth.PermissionProjectEdit) != nil {
				return userauth.ErrForbidden
			}
			return nil
		}}
		object, err := vault.ImportObject(r.Context(), body)
		if err != nil {
			if body.denied != nil {
				if !writeProjectMutationDenied(w, body.denied) {
					writeProjectQualificationError(w, body.denied)
				}
				return
			}
			if errors.Is(err, storagehealth.ErrRuntimeReserve) {
				writeJSON(w, http.StatusInsufficientStorage, map[string]any{"error_code": "STORAGE_RUNTIME_RESERVE"})
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXECUTION_ENVIRONMENT_CAPTURE_VAULT_FAILED"})
			return
		}

		ctx := projectMutationContext(r, auth, session, userauth.PermissionProjectEdit, nil, projectaccess.ProjectEdit, projectaccess.MediaUpload)
		updated, err := stageStore.CaptureExecutionEnvironmentAsset(ctx, environment.ID, asset.Key, object.ContentHash, object.SizeBytes, environment.ContentSHA256)
		if err != nil {
			_, _ = vault.RemoveObjectIfUnreferenced(context.WithoutCancel(r.Context()), object.ContentHash)
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
