package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/showcapsule"
	"github.com/ali96adil/StageCore/internal/userauth"
)

type ShowCapsuleService interface {
	BuildManifest(context.Context, string, showcapsule.BuildOptions) (showcapsule.Manifest, error)
	Export(context.Context, string, string, showcapsule.BuildOptions) (showcapsule.ExportResult, error)
	PlanImport(context.Context, string) (showcapsule.ImportPlan, error)
	Materialize(context.Context, string, showcapsule.MaterializeOptions) (showcapsule.MaterializeResult, error)
}

type showCapsuleExportRequest struct {
	RuntimeSnapshotID string                 `json:"runtime_snapshot_id"`
	Mode              showcapsule.ExportMode `json:"mode"`
}

type showCapsuleEntry struct {
	CapsuleID         string                 `json:"capsule_id"`
	ProjectID         string                 `json:"project_id"`
	ProjectName       string                 `json:"project_name"`
	RuntimeSnapshotID string                 `json:"runtime_snapshot_id"`
	ExportMode        showcapsule.ExportMode `json:"export_mode"`
	CreatedAt         string                 `json:"created_at"`
	Location          string                 `json:"location"`
}

func WithOperatorShowCapsules(auth *userauth.Service, service ShowCapsuleService, root string) Option {
	return withOperatorShowCapsules(auth, service, root, nil)
}

// WithScopedOperatorShowCapsules is available for cutover qualification only.
func WithScopedOperatorShowCapsules(auth *userauth.Service, service ShowCapsuleService, root string, access *projectaccess.Service) Option {
	if access == nil {
		return func(*Server) {}
	}
	return withOperatorShowCapsules(auth, service, root, access)
}

func withOperatorShowCapsules(auth *userauth.Service, service ShowCapsuleService, root string, access *projectaccess.Service) Option {
	return func(s *Server) {
		if auth == nil || service == nil || strings.TrimSpace(root) == "" {
			return
		}
		root = filepath.Clean(root)
		exportRoot := filepath.Join(root, "exports")
		importRoot := filepath.Join(root, "imports")

		recheckRead := func(w http.ResponseWriter, r *http.Request, session userauth.Session, permissions ...projectaccess.Permission) bool {
			token, _ := browserSessionToken(r)
			current, err := auth.ValidatePassive(r.Context(), token)
			if err != nil || current.ID != session.ID || current.User.ID != session.User.ID {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": "SESSION_INVALID"})
				return false
			}
			if userauth.Authorize(current.User.Role, userauth.PermissionProjectRead) != nil {
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return false
			}
			if len(permissions) == 0 {
				return allowScopedOwner(w, current, access)
			}
			return allowScopedProject(w, r, current, access, permissions...)
		}

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/show-capsule/preview", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if !allowScopedProject(w, r, session, access, projectaccess.CapsuleExport, projectaccess.MediaRead) {
				return
			}
			mode, ok := parseShowCapsuleMode(w, r.URL.Query().Get("mode"))
			if !ok {
				return
			}
			manifest, err := service.BuildManifest(r.Context(), r.PathValue("project_id"), showcapsule.BuildOptions{
				RuntimeSnapshotID: strings.TrimSpace(r.URL.Query().Get("runtime_snapshot_id")),
				Mode:              mode,
			})
			if err != nil {
				if !recheckRead(w, r, session, projectaccess.CapsuleExport, projectaccess.MediaRead) {
					return
				}
				writeProjectStoreError(w, err)
				return
			}
			if !recheckRead(w, r, session, projectaccess.CapsuleExport, projectaccess.MediaRead) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"manifest": manifest})
		}))

		exportPermission := userauth.PermissionProjectEdit
		if access != nil {
			exportPermission = userauth.PermissionProjectRead
		}
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/show-capsules", withPermission(auth, exportPermission, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if !allowScopedProject(w, r, session, access, projectaccess.CapsuleExport, projectaccess.MediaRead) {
				return
			}
			var body showCapsuleExportRequest
			if !decodeBoundedJSON(w, r, &body) {
				return
			}
			mode, ok := parseShowCapsuleMode(w, string(body.Mode))
			if !ok {
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			projectID := r.PathValue("project_id")
			projectScope := qualificationAccess(r, access)
			token, _ := browserSessionToken(r)
			csrf := r.Header.Get(csrfHeader)
			authorize := func(ctx context.Context) error {
				current, err := auth.ValidateCSRF(ctx, token, csrf)
				if err != nil || current.ID != session.ID || current.User.ID != session.User.ID ||
					userauth.Authorize(current.User.Role, exportPermission) != nil {
					return userauth.ErrForbidden
				}
				if projectScope != nil {
					return projectScope.CheckAll(ctx, current.User.ID, projectID, projectaccess.CapsuleExport, projectaccess.MediaRead)
				}
				return nil
			}
			writeExportAuthorizationError := func(err error) bool {
				if errors.Is(err, userauth.ErrForbidden) {
					writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
					return false
				}
				return writeProjectQualificationError(w, err)
			}
			if !writeExportAuthorizationError(authorize(r.Context())) {
				return
			}
			if err := os.MkdirAll(exportRoot, 0o750); err != nil {
				if !writeExportAuthorizationError(authorize(r.Context())) {
					return
				}
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CAPSULE_STORAGE_UNAVAILABLE"})
				return
			}
			result, err := service.Export(r.Context(), projectID, exportRoot, showcapsule.BuildOptions{
				RuntimeSnapshotID: strings.TrimSpace(body.RuntimeSnapshotID),
				Mode:              mode,
				AuthorizePublish:  authorize,
			})
			if errors.Is(err, userauth.ErrForbidden) || errors.Is(err, projectaccess.ErrDenied) || errors.Is(err, projectaccess.ErrNotFound) {
				writeExportAuthorizationError(err)
				return
			}
			if err != nil {
				if !writeExportAuthorizationError(authorize(r.Context())) {
					return
				}
				writeProjectStoreError(w, err)
				return
			}
			if err := authorize(r.Context()); err != nil {
				writeCapsuleCommittedMutationRevocation(w)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{
				"capsule_id":    result.CapsuleID,
				"manifest":      result.Manifest,
				"location":      "exports",
				"transfer_note": "Copy this capsule directory into the replacement Hub's Show Capsule imports directory before planning restore.",
			})
		}))

		s.mux.HandleFunc("GET /api/v1/show-capsules", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if !allowScopedOwner(w, session, access) {
				return
			}
			entries := make([]showCapsuleEntry, 0)
			for _, source := range []struct {
				name string
				path string
			}{{"imports", importRoot}, {"exports", exportRoot}} {
				items, err := listShowCapsules(source.path, source.name)
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CAPSULE_STORAGE_UNAVAILABLE"})
					return
				}
				entries = append(entries, items...)
			}
			sort.Slice(entries, func(i, j int) bool {
				if entries[i].CreatedAt != entries[j].CreatedAt {
					return entries[i].CreatedAt > entries[j].CreatedAt
				}
				return entries[i].CapsuleID < entries[j].CapsuleID
			})
			if !recheckRead(w, r, session) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"capsules": entries,
				"presentation": map[string]any{
					"en": map[string]string{"title": "Show Capsules", "import_hint": "Copy a capsule into the replacement Hub import directory, then run readiness planning before restore."},
					"ar": map[string]string{"title": "حزم العرض", "import_hint": "انسخ حزمة العرض إلى مجلد الاستيراد في جهاز StageCore البديل، ثم افحص الجاهزية قبل الاستعادة."},
				},
			})
		}))

		s.mux.HandleFunc("GET /api/v1/show-capsules/imports/{capsule_id}/plan", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if !allowScopedOwner(w, session, access) {
				return
			}
			path, ok := resolveCapsulePath(w, importRoot, r.PathValue("capsule_id"))
			if !ok {
				return
			}
			plan, err := service.PlanImport(r.Context(), path)
			if err != nil {
				if !recheckRead(w, r, session) {
					return
				}
				writeShowCapsuleError(w, err)
				return
			}
			if !recheckRead(w, r, session) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"plan": plan})
		}))

		s.mux.HandleFunc("POST /api/v1/show-capsules/imports/{capsule_id}/materialize", withPermission(auth, userauth.PermissionBackupRestore, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			if !allowScopedOwner(w, session, access) {
				return
			}
			path, ok := resolveCapsulePath(w, importRoot, r.PathValue("capsule_id"))
			if !ok {
				return
			}
			token, _ := browserSessionToken(r)
			csrf := r.Header.Get(csrfHeader)
			authorize := func(ctx context.Context) error {
				current, err := auth.ValidateCSRF(ctx, token, csrf)
				if err != nil || current.ID != session.ID || current.User.ID != session.User.ID ||
					userauth.Authorize(current.User.Role, userauth.PermissionBackupRestore) != nil {
					return userauth.ErrForbidden
				}
				return nil
			}
			ctx := projectMutationContext(r, auth, session, userauth.PermissionBackupRestore, access)
			result, err := service.Materialize(ctx, path, showcapsule.MaterializeOptions{
				ImportedBy: session.User.Username, AuthorizeWrite: authorize,
			})
			if errors.Is(err, userauth.ErrForbidden) {
				writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
				return
			}
			if err != nil {
				writeShowCapsuleError(w, err)
				return
			}
			if err := authorize(r.Context()); err != nil {
				writeCapsuleCommittedMutationRevocation(w)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"result": result})
		}))
	}
}

func writeCapsuleCommittedMutationRevocation(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusForbidden, map[string]any{
		"error_code":                     capability.DispatchAuthorityRevokedCode,
		"detail":                         "Authorization ended after the Show Capsule operation; verify current Capsule and Project state before taking further action.",
		"effect_may_have_been_committed": true,
		"retryable":                      false,
	})
}

func parseShowCapsuleMode(w http.ResponseWriter, raw string) (showcapsule.ExportMode, bool) {
	mode := showcapsule.ExportMode(strings.ToUpper(strings.TrimSpace(raw)))
	if mode == "" {
		mode = showcapsule.ExportManifestOnly
	}
	if mode != showcapsule.ExportManifestOnly && mode != showcapsule.ExportSelfContained {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_CAPSULE_MODE"})
		return "", false
	}
	return mode, true
}

func resolveCapsulePath(w http.ResponseWriter, root, capsuleID string) (string, bool) {
	capsuleID = strings.TrimSpace(capsuleID)
	if err := stageid.ValidateCanonical(capsuleID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_CAPSULE_ID"})
		return "", false
	}
	path := filepath.Join(root, capsuleID)
	if info, err := os.Lstat(path); err != nil || !info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "CAPSULE_NOT_FOUND"})
		return "", false
	}
	return path, true
}

func listShowCapsules(root, location string) ([]showCapsuleEntry, error) {
	items, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []showCapsuleEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]showCapsuleEntry, 0, len(items))
	for _, item := range items {
		if !item.IsDir() || stageid.ValidateCanonical(item.Name()) != nil {
			continue
		}
		manifest, err := showcapsule.Verify(filepath.Join(root, item.Name()))
		if err != nil {
			continue
		}
		out = append(out, showCapsuleEntry{
			CapsuleID:         manifest.CapsuleID,
			ProjectID:         manifest.Project.ProjectID,
			ProjectName:       manifest.Project.Name,
			RuntimeSnapshotID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
			ExportMode:        manifest.ExportMode,
			CreatedAt:         manifest.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
			Location:          location,
		})
	}
	return out, nil
}

func writeShowCapsuleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "INVALID_CAPSULE", "error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "CAPSULE_NOT_FOUND"})
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrShowConfigurationLocked):
		writeJSON(w, http.StatusConflict, map[string]any{"error_code": "CAPSULE_NOT_READY", "error": err.Error()})
	default:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error_code": "CAPSULE_VERIFICATION_FAILED", "error": err.Error()})
	}
}
