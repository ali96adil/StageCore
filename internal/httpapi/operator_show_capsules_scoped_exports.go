package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/showcapsule"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const maxScopedCapsuleScanEntries = 128

// WithScopedLocalCapsuleExports registers an *opt-in* read-only listing for
// current local Project exports. It is NOT registered by the normal Hub
// assembler, and it does not change the globally authorized Show Capsule
// library, import planner, backup.restore or any live SHOW routes.
//
// This is an incremental qualification route, NOT a system-wide ACL cutover.
// The configured root is a trusted Hub-controlled directory, not a request
// parameter. Import capsules from other Hubs require independent Owner
// verification and may not resolve to any local Project Snapshot.
func WithScopedLocalCapsuleExports(
	auth *userauth.Service, access *projectaccess.Service, capsuleRoot string,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || access == nil ||
			strings.TrimSpace(capsuleRoot) == "" {
			return
		}
		exportsRoot := filepath.Join(filepath.Clean(capsuleRoot), "exports")
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/show-capsules/exports",
			withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
				w.Header().Set("Cache-Control", "no-store")
				projectID := strings.TrimSpace(r.PathValue("project_id"))
				if projectID == "" {
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "SCOPED_CAPSULES_NOT_FOUND"})
					return
				}
				if err := access.Check(r.Context(), session.User.ID, projectID, projectaccess.CapsuleExport); err != nil {
					writeScopedExportAccessError(w, err)
					return
				}
				entries, err := listScopedLocalCapsuleExports(r.Context(), exportsRoot, access, session.User.ID, projectID)
				if err != nil {
					if errors.Is(err, projectaccess.ErrDenied) || errors.Is(err, projectaccess.ErrNotFound) {
						writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "SCOPED_CAPSULES_NOT_FOUND"})
						return
					}
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "SCOPED_CAPSULES_UNAVAILABLE"})
					return
				}
				// Recheck revocations before serialization; this is NOT a
				// substitute for a future atomic streaming authorization fence.
				if err := access.Check(r.Context(), session.User.ID, projectID, projectaccess.CapsuleExport); err != nil {
					writeScopedExportAccessError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"project_id": projectID,
					"capsules": entries,
					"scoped": true,
					"read_only": true,
					"location": "exports",
				})
			}))
	}
}

func writeScopedExportAccessError(w http.ResponseWriter, err error) {
	if errors.Is(err, projectaccess.ErrDenied) || errors.Is(err, projectaccess.ErrNotFound) {
		// Unknown Project, denied Project and missing user grants are
		// intentionally indistinguishable to the requesting user.
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "SCOPED_CAPSULES_NOT_FOUND"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "SCOPED_CAPSULES_UNAVAILABLE"})
}

// Only verified local exports whose immutable published Snapshot belongs
// to the requested Project may appear. The actual Project ID comes from
// a canonical verified manifest, never a title or URL-supplied capsule ID.
// Import inventory is deliberately excluded.
func listScopedLocalCapsuleExports(ctx context.Context, root string, access *projectaccess.Service,
	userID, projectID string) ([]showCapsuleEntry, error) {
	entries := make([]showCapsuleEntry, 0)
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect trusted Capsule exports directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("Capsule exports root must be a real directory")
	}
	items, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("enumerate local Capsule exports: %w", err)
	}
	if len(items) > maxScopedCapsuleScanEntries {
		// Fail whole request instead of returning a truncated view.
		// Cursor-based inventory with a stable ID/index is a future gate.
		return nil, fmt.Errorf("Capsule export inventory requires pagination")
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !item.IsDir() || stageid.ValidateCanonical(item.Name()) != nil {
			continue // Ignore symlinks, staging directories and foreign names.
		}
		// Revalidate filesystem entry before reading. The export root must
		// remain Hub-controlled; no user-writable directory is authorized.
		fullPath := filepath.Join(root, item.Name())
		current, err := os.Lstat(fullPath)
		if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 {
			continue
		}
		manifest, err := showcapsule.Verify(fullPath)
		if err != nil || manifest.CapsuleID != item.Name() ||
			manifest.Project.ProjectID != projectID {
			continue
		}
		identity := projectaccess.LocalExportCapsuleIdentity{
			ProjectID: manifest.Project.ProjectID,
			RevisionID: manifest.Project.RevisionID,
			SnapshotID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
			SnapshotSHA256: manifest.RuntimeSnapshot.ContentSHA256,
		}
		err = access.CheckLocalExportCapsuleRead(ctx, userID, projectID, identity)
		if errors.Is(err, projectaccess.ErrDenied) || errors.Is(err, projectaccess.ErrNotFound) {
			// Either access was revoked while scanning, or a local file no
			// longer matches its authoritative published Snapshot. Do not
			// expose the capsule; the final live grant check also applies.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("qualify scoped Capsule export: %w", err)
		}
		entries = append(entries, showCapsuleEntry{
			CapsuleID: manifest.CapsuleID,
			ProjectID: projectID,
			ProjectName: manifest.Project.Name, // historical title, not authority
			RuntimeSnapshotID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
			ExportMode: manifest.ExportMode,
			CreatedAt: manifest.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
			Location: "exports",
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt != entries[j].CreatedAt {
			return entries[i].CreatedAt > entries[j].CreatedAt
		}
		return entries[i].CapsuleID < entries[j].CapsuleID
	})
	return entries, nil
}
