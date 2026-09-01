package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const maxOfflineExtensionUploadBytes int64 = extension.MaxOfflineBundlePayloadSize + extension.MaxOfflineBundleMetadataSize + extension.MaxOfflineBundleManifestSize + (2 << 20)

type operatorExtensionOfflineBundles struct {
	users    *userauth.Service
	importer *extension.OfflineBundleImporter
	catalog  *extension.TrustedCatalog
	audit    *securityaudit.Service
}

func WithOperatorExtensionOfflineBundles(users *userauth.Service, importer *extension.OfflineBundleImporter, catalog *extension.TrustedCatalog, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || importer == nil {
			return
		}
		h := &operatorExtensionOfflineBundles{users: users, importer: importer, catalog: catalog, audit: audit}
		s.mux.HandleFunc("POST /api/v1/extensions/import-bundle", withPermission(users, userauth.PermissionPluginManage, h.importBundle))
		if catalog != nil {
			s.mux.HandleFunc("POST /api/v1/extensions/catalog/sync", withPermission(users, userauth.PermissionPluginManage, h.syncCatalog))
		}
	}
}

func (h *operatorExtensionOfflineBundles) importBundle(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	if r.ContentLength > maxOfflineExtensionUploadBytes {
		h.record(r, session, "extension_bundle", securityaudit.ResultRejected, "EXTENSION_BUNDLE_TOO_LARGE", nil)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error_code": "EXTENSION_BUNDLE_TOO_LARGE"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOfflineExtensionUploadBytes)
	result, err := h.importer.Import(r.Context(), r.Body, session.User.Username)
	if err != nil {
		status, code := offlineBundleError(err)
		h.record(r, session, "extension_bundle", securityaudit.ResultRejected, code, nil)
		writeJSON(w, status, map[string]any{"error_code": code})
		return
	}
	h.record(r, session, result.Package.PackageID, securityaudit.ResultSuccess, "", map[string]any{
		"extension_id": result.Package.Manifest.ExtensionID,
		"version": result.Package.Manifest.Version,
		"source": result.Package.Manifest.Source,
		"already_registered": result.AlreadyRegistered,
		"payload_sha256": result.PayloadSHA256,
		"payload_size_bytes": result.PayloadSizeBytes,
	})
	writeJSON(w, http.StatusCreated, result)
}

func (h *operatorExtensionOfflineBundles) syncCatalog(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	result, err := h.catalog.Sync(r.Context(), session.User.Username)
	if err != nil {
		status, code := offlineBundleError(err)
		h.record(r, session, "trusted_catalog", securityaudit.ResultRejected, code, nil)
		writeJSON(w, status, map[string]any{"error_code": code})
		return
	}
	h.record(r, session, "trusted_catalog", securityaudit.ResultSuccess, "", map[string]any{"imported_count": len(result.Imported)})
	writeJSON(w, http.StatusOK, result)
}

func offlineBundleError(err error) (int, string) {
	var maxBytes *http.MaxBytesError
	switch {
	case errors.Is(err, domain.ErrShowConfigurationLocked):
		return http.StatusConflict, "SHOW_CONFIGURATION_LOCKED"
	case errors.Is(err, extension.ErrOfflineBundleSource), errors.Is(err, extension.ErrOfficialSourceRequiresTrustedPath):
		return http.StatusForbidden, "EXTENSION_BUNDLE_SOURCE_FORBIDDEN"
	case errors.Is(err, extension.ErrOfflineBundleTooLarge), errors.As(err, &maxBytes):
		return http.StatusRequestEntityTooLarge, "EXTENSION_BUNDLE_TOO_LARGE"
	case errors.Is(err, extension.ErrOfflineBundleIntegrity):
		return http.StatusBadRequest, "EXTENSION_BUNDLE_INTEGRITY_FAILED"
	case errors.Is(err, extension.ErrOfflineBundleInvalid):
		return http.StatusBadRequest, "EXTENSION_BUNDLE_INVALID"
	case errors.Is(err, extension.ErrTrustedCatalogUnavailable):
		return http.StatusServiceUnavailable, "EXTENSION_CATALOG_UNAVAILABLE"
	default:
		return http.StatusBadRequest, "EXTENSION_BUNDLE_IMPORT_FAILED"
	}
}

func (h *operatorExtensionOfflineBundles) record(r *http.Request, session userauth.Session, resourceID, result, reason string, metadata any) {
	if h.audit == nil {
		return
	}
	eventType := "extension.bundle.import"
	if strings.TrimSpace(resourceID) == "trusted_catalog" {
		eventType = "extension.catalog.sync"
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType: eventType,
		ActorUserID: session.User.ID,
		ActorUsername: session.User.Username,
		Source: "operator-web",
		ResourceType: "extension_package",
		ResourceID: strings.TrimSpace(resourceID),
		Result: result,
		Reason: reason,
		Metadata: metadata,
	})
}
