package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/showcapsule"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const (
	maxOwnerImportEntries = 128
	maxOwnerImportTreeEntries = 4096
	maxOwnerImportTreeBytes int64 = 8 << 30 // 8 GiB; bounded review, not a transfer limit
	maxOwnerImportMetadataBytes int64 = 2 << 20
)

// ownerCapsulePlanner intentionally has NO Materialize method. This opt-in
// owner-only preview cannot mutate a Project, import Vault bytes or approve
// a restore, even if an implementation also supports those operations.
type ownerCapsulePlanner interface {
	PlanImport(context.Context, string) (showcapsule.ImportPlan, error)
}

// WithOwnerCapsuleImportReview registers an opt-in read-only import inventory
// and readiness review. It does not replace the legacy/global Capsule
// library, import plan or backup.restore handlers and is NOT enabled in the
// production Hub assembler. Imports from a different Hub do not yet have a
// trusted local Project/role ACL: NEVER grant this to a Project Operator.
func WithOwnerCapsuleImportReview(
	auth *userauth.Service, planner ownerCapsulePlanner, capsuleRoot string,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || planner == nil ||
			strings.TrimSpace(capsuleRoot) == "" {
			return
		}
		root := filepath.Join(filepath.Clean(capsuleRoot), "imports")
		s.mux.HandleFunc("GET /api/v1/show-capsules/imports/review",
			withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
				w.Header().Set("Cache-Control", "no-store")
				if !authorizeOwnerCapsuleReview(w, session) { return }
				items, skipped, err := ownerVerifiedImportInventory(r.Context(), root)
				if err != nil {
					writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "CAPSULE_IMPORT_REVIEW_UNAVAILABLE"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"imports": items,
					"skipped_unverified": skipped,
					"read_only": true,
					"authorized_to_restore": false,
					"scope": "OWNER_REVIEW_ONLY",
					"note": "Verified import inventory only; a separate explicit restore authorization and fresh readiness check are required.",
				})
			}))

		s.mux.HandleFunc("GET /api/v1/show-capsules/imports/{capsule_id}/review",
			withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, _ string) {
				w.Header().Set("Cache-Control", "no-store")
				if !authorizeOwnerCapsuleReview(w, session) { return }
				capsuleID := strings.TrimSpace(r.PathValue("capsule_id"))
				if stageid.ValidateCanonical(capsuleID) != nil {
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "CAPSULE_IMPORT_NOT_FOUND"})
					return
				}
				path, err := ownerSafeImportCapsule(root, capsuleID)
				if err != nil {
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "CAPSULE_IMPORT_NOT_FOUND"})
					return
				}
				verified, err := showcapsule.Verify(path)
				if err != nil || verified.CapsuleID != capsuleID {
					writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error_code": "CAPSULE_IMPORT_NOT_VERIFIED"})
					return
				}
				plan, err := planner.PlanImport(r.Context(), path)
				if err != nil {
					writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error_code": "CAPSULE_IMPORT_PLAN_UNAVAILABLE"})
					return
				}
				// This is a readiness preview, not a restore action or token.
				// A later mutation needs separate OWNER+CSRF+SHOW fence and
				// fresh plan evaluation on the same trusted filesystem tree.
				if plan.CapsuleID != capsuleID || plan.ProjectID != verified.Project.ProjectID ||
					plan.RuntimeSnapshotID != verified.RuntimeSnapshot.RuntimeSnapshotID {
					writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error_code": "CAPSULE_IMPORT_PLAN_MISMATCH"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"plan": plan, "read_only": true,
					"authorized_to_restore": false, "scope": "OWNER_REVIEW_ONLY",
				})
			}))
	}
}

func authorizeOwnerCapsuleReview(w http.ResponseWriter, session userauth.Session) bool {
	if session.User.Role != userauth.RoleOwner ||
		userauth.Authorize(session.User.Role, userauth.PermissionBackupRestore) != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "CAPSULE_IMPORT_OWNER_REQUIRED"})
		return false
	}
	return true
}

func ownerVerifiedImportInventory(ctx context.Context, root string) ([]showCapsuleEntry, int, error) {
	entries := make([]showCapsuleEntry, 0)
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return entries, 0, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, fmt.Errorf("trusted imports directory unavailable")
	}
	files, err := os.ReadDir(root)
	if err != nil {
		return nil, 0, err
	}
	// Fail the whole request instead of enumerating only a subset.
	if len(files) > maxOwnerImportEntries {
		return nil, 0, fmt.Errorf("import inventory exceeds bounded review")
	}
	skipped := 0
	for _, entry := range files {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		if !entry.IsDir() || stageid.ValidateCanonical(entry.Name()) != nil {
			skipped++
			continue
		}
		path, err := ownerSafeImportCapsule(root, entry.Name())
		if err != nil {
			skipped++
			continue
		}
		manifest, err := showcapsule.Verify(path)
		if err != nil || manifest.CapsuleID != entry.Name() {
			skipped++
			continue
		}
		entries = append(entries, showCapsuleEntry{
			CapsuleID: manifest.CapsuleID,
			ProjectID: manifest.Project.ProjectID,
			ProjectName: manifest.Project.Name,
			RuntimeSnapshotID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
			ExportMode: manifest.ExportMode,
			CreatedAt: manifest.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
			Location: "imports",
		})
	}
	sort.Slice(entries, func(i,j int) bool {
		if entries[i].CreatedAt != entries[j].CreatedAt {
			return entries[i].CreatedAt > entries[j].CreatedAt
		}
		return entries[i].CapsuleID < entries[j].CapsuleID
	})
	return entries, skipped, nil
}

// ownerSafeImportCapsule is a conservative preflight for a Hub-controlled
// directory. Reject every symlink, device, socket, FIFO, oversized manifest,
// excessive file count and oversized untrusted content tree before Verify or
// PlanImport opens anything. This is NOT an openat2/fd-rooted TOCTOU solution;
// do not accept an adversary-writable imports root in production.
func ownerSafeImportCapsule(root, id string) (string, error) {
	if stageid.ValidateCanonical(id) != nil { return "", fmt.Errorf("invalid capsule id") }
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("untrusted imports root")
	}
	path := filepath.Join(root, id)
	info, err = os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("missing or unsafe capsule directory")
	}
	var count int
	var totalBytes int64
	err = filepath.WalkDir(path, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		count++
		if count > maxOwnerImportTreeEntries { return fmt.Errorf("import tree entry cap") }
		typ := entry.Type()
		if typ&os.ModeSymlink != 0 { return fmt.Errorf("import contains symlink") }
		if entry.IsDir() { return nil }
		if !typ.IsRegular() && typ != 0 { return fmt.Errorf("import contains nonregular file") }
		meta, err := entry.Info()
		if err != nil || !meta.Mode().IsRegular() || meta.Size() < 0 {
			return fmt.Errorf("import file is not regular")
		}
		if meta.Size() > maxOwnerImportTreeBytes-totalBytes {
			return fmt.Errorf("import tree size cap")
		}
		totalBytes += meta.Size()
		if filepath.Base(name) == showcapsule.ManifestFileName ||
			filepath.Base(name) == showcapsule.ManifestChecksumFile {
			if meta.Size() > maxOwnerImportMetadataBytes {
				return fmt.Errorf("oversized capsule metadata")
			}
		}
		return nil
	})
	if err != nil { return "", err }
	return path, nil
}
