package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/securityaudit"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const defaultTrustedExtensionCatalogRoot = "/opt/stagecore/extensions/catalog"

type operatorExtensionLibrary struct {
	users   *userauth.Service
	library *extension.Library
	audit   *securityaudit.Service
}

type registerExtensionRequest struct {
	PackageID string          `json:"package_id"`
	Manifest  json.RawMessage `json:"manifest"`
}

func WithOperatorExtensionLibrary(users *userauth.Service, library *extension.Library, audit *securityaudit.Service) Option {
	return func(s *Server) {
		if users == nil || library == nil {
			return
		}
		h := &operatorExtensionLibrary{users: users, library: library, audit: audit}
		s.mux.HandleFunc("GET /api/v1/extensions", withPermission(users, userauth.PermissionProjectRead, h.list))
		s.mux.HandleFunc("GET /api/v1/extensions/packages/{package_id}", withPermission(users, userauth.PermissionProjectRead, h.get))
		s.mux.HandleFunc("POST /api/v1/extensions/register", withPermission(users, userauth.PermissionPluginManage, h.register))

		importer, err := extension.NewOfflineBundleImporter(library)
		if err != nil {
			return
		}
		catalog, err := extension.NewTrustedCatalog(importer, defaultTrustedExtensionCatalogRoot)
		if err != nil {
			return
		}
		offline := &operatorExtensionOfflineBundles{users: users, importer: importer, catalog: catalog, audit: audit}
		s.mux.HandleFunc("POST /api/v1/extensions/import-bundle", withPermission(users, userauth.PermissionPluginManage, offline.importBundle))
		s.mux.HandleFunc("POST /api/v1/extensions/catalog/sync", withPermission(users, userauth.PermissionPluginManage, offline.syncCatalog))
	}
}

func (h *operatorExtensionLibrary) list(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	items, err := h.library.List(r.Context(), strings.TrimSpace(r.URL.Query().Get("extension_id")))
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_LIBRARY_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"manifest_schema_version": extension.ManifestSchemaVersion, "extensions": items})
}

func (h *operatorExtensionLibrary) get(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
	item, err := h.library.Get(r.Context(), strings.TrimSpace(r.PathValue("package_id")))
	if errors.Is(err, domain.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "EXTENSION_PACKAGE_NOT_FOUND"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "EXTENSION_LIBRARY_UNAVAILABLE"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *operatorExtensionLibrary) register(w http.ResponseWriter, r *http.Request, session userauth.Session) {
	var body registerExtensionRequest
	if !decodeBoundedJSON(w, r, &body) {
		return
	}
	packageID := strings.TrimSpace(body.PackageID)
	item, err := h.library.Register(r.Context(), packageID, body.Manifest, session.User.Username)
	if err != nil {
		status := http.StatusBadRequest
		code := "EXTENSION_REGISTER_FAILED"
		switch {
		case errors.Is(err, domain.ErrShowConfigurationLocked):
			status = http.StatusConflict
			code = "SHOW_CONFIGURATION_LOCKED"
		case errors.Is(err, domain.ErrNotFound):
			status = http.StatusNotFound
			code = "SOFTWARE_PACKAGE_NOT_FOUND"
		case errors.Is(err, extension.ErrOfficialSourceRequiresTrustedPath):
			status = http.StatusForbidden
			code = "EXTENSION_OFFICIAL_SOURCE_FORBIDDEN"
		}
		h.record(r, session, packageID, securityaudit.ResultRejected, code, nil)
		writeJSON(w, status, map[string]any{"error_code": code})
		return
	}
	h.record(r, session, item.PackageID, securityaudit.ResultSuccess, "", map[string]any{
		"extension_id": item.Manifest.ExtensionID,
		"version": item.Manifest.Version,
		"kind": item.Manifest.Kind,
		"source": item.Manifest.Source,
	})
	writeJSON(w, http.StatusCreated, item)
}

func (h *operatorExtensionLibrary) record(r *http.Request, session userauth.Session, resourceID, result, reason string, metadata any) {
	if h.audit == nil {
		return
	}
	_, _ = h.audit.Append(r.Context(), securityaudit.Event{
		EventType: "extension.package.register",
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
