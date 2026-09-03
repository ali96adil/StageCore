package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
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
	CapsuleID          string                 `json:"capsule_id"`
	ProjectID          string                 `json:"project_id"`
	ProjectName        string                 `json:"project_name"`
	RuntimeSnapshotID  string                 `json:"runtime_snapshot_id"`
	ExportMode         showcapsule.ExportMode `json:"export_mode"`
	CreatedAt          string                 `json:"created_at"`
	Location           string                 `json:"location"`
}

func WithOperatorShowCapsules(auth *userauth.Service, service ShowCapsuleService, root string) Option {
	return func(s *Server) {
		if auth == nil || service == nil || strings.TrimSpace(root) == "" {
			return
		}
		root = filepath.Clean(root)
		exportRoot := filepath.Join(root, "exports")
		importRoot := filepath.Join(root, "imports")

		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/show-capsule/preview", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			mode, ok := parseShowCapsuleMode(w, r.URL.Query().Get("mode"))
			if !ok {
				return
			}
			manifest, err := service.BuildManifest(r.Context(), r.PathValue("project_id"), showcapsule.BuildOptions{
				RuntimeSnapshotID: strings.TrimSpace(r.URL.Query().Get("runtime_snapshot_id")),
				Mode: mode,
			})
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"manifest": manifest})
		}))

		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/show-capsules", withPermission(auth, userauth.PermissionProjectEdit, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			var body showCapsuleExportRequest
			if !decodeBoundedJSON(w, r, &body) {
				return
			}
			mode, ok := parseShowCapsuleMode(w, string(body.Mode))
			if !ok {
				return
			}
			if err := os.MkdirAll(exportRoot, 0o750); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CAPSULE_STORAGE_UNAVAILABLE"})
				return
			}
			result, err := service.Export(r.Context(), r.PathValue("project_id"), exportRoot, showcapsule.BuildOptions{
				RuntimeSnapshotID: strings.TrimSpace(body.RuntimeSnapshotID),
				Mode: mode,
			})
			if err != nil {
				writeProjectStoreError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{
				"capsule_id": result.CapsuleID,
				"manifest": result.Manifest,
				"location": "exports",
				"transfer_note": "Copy this capsule directory into the replacement Hub's Show Capsule imports directory before planning restore.",
			})
		}))

		s.mux.HandleFunc("GET /api/v1/show-capsules", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
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
			writeJSON(w, http.StatusOK, map[string]any{
				"capsules": entries,
				"presentation": map[string]any{
					"en": map[string]string{"title": "Show Capsules", "import_hint": "Copy a capsule into the replacement Hub import directory, then run readiness planning before restore."},
					"ar": map[string]string{"title": "حزم العرض", "import_hint": "انسخ حزمة العرض إلى مجلد الاستيراد في جهاز StageCore البديل، ثم افحص الجاهزية قبل الاستعادة."},
				},
			})
		}))

		s.mux.HandleFunc("GET /api/v1/show-capsules/imports/{capsule_id}/plan", withPermission(auth, userauth.PermissionProjectRead, func(w http.ResponseWriter, r *http.Request, _ userauth.Session) {
			path, ok := resolveCapsulePath(w, importRoot, r.PathValue("capsule_id"))
			if !ok {
				return
			}
			plan, err := service.PlanImport(r.Context(), path)
			if err != nil {
				writeShowCapsuleError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"plan": plan})
		}))

		s.mux.HandleFunc("POST /api/v1/show-capsules/imports/{capsule_id}/materialize", withPermission(auth, userauth.PermissionBackupRestore, func(w http.ResponseWriter, r *http.Request, session userauth.Session) {
			path, ok := resolveCapsulePath(w, importRoot, r.PathValue("capsule_id"))
			if !ok {
				return
			}
			result, err := service.Materialize(r.Context(), path, showcapsule.MaterializeOptions{ImportedBy: session.User.Username})
			if err != nil {
				writeShowCapsuleError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"result": result})
		}))
	}
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
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
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
			CapsuleID: manifest.CapsuleID,
			ProjectID: manifest.Project.ProjectID,
			ProjectName: manifest.Project.Name,
			RuntimeSnapshotID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
			ExportMode: manifest.ExportMode,
			CreatedAt: manifest.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
			Location: location,
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
