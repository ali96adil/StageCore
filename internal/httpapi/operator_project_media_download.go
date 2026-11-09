package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/ali96adil/StageCore/internal/bulk"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

// WithScopedProjectMediaDownload qualifies browser media streaming without
// giving browser sessions access to the Companion's raw-hash endpoint.
func WithScopedProjectMediaDownload(auth *userauth.Service, access *projectaccess.Service, projects *store.Store, objects *vault.Vault) Option {
	return func(s *Server) {
		if auth == nil || access == nil || projects == nil || objects == nil {
			return
		}
		s.projectMediaDownload = true
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/files/{version_id}/download",
			withAuthenticatedSession(auth, false, func(w http.ResponseWriter, r *http.Request, session userauth.Session, token string) {
				if !allowScopedProject(w, r, session, access, projectaccess.MediaRead) {
					return
				}
				ctx := r.Context()
				projectID := strings.TrimSpace(r.PathValue("project_id"))
				version, err := projects.GetMediaContentVersion(ctx, r.PathValue("version_id"))
				if err != nil || access.CheckScopedVaultObjectRead(ctx, session.User.ID, projectID, version.ID, version.ContentHash) != nil {
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MEDIA_NOT_FOUND"})
					return
				}
				file, object, err := objects.OpenObject(ctx, version.ContentHash)
				if err != nil || object.SizeBytes != version.SizeBytes {
					if file != nil {
						file.Close()
					}
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MEDIA_NOT_FOUND"})
					return
				}
				defer file.Close()
				authorize := func() bool {
					current, err := auth.ValidatePassive(ctx, token)
					return err == nil && current.ID == session.ID && current.User.ID == session.User.ID &&
						access.CheckScopedVaultObjectRead(ctx, current.User.ID, projectID, version.ID, version.ContentHash) == nil
				}
				reader := &authorizedMediaReader{ReadSeeker: file, authorize: authorize}
				if s.bulk != nil {
					id, err := s.bulk.Begin(ctx, bulk.KindMediaSync, object.SizeBytes)
					if errors.Is(err, bulk.ErrShowBlocked) {
						writeJSON(w, http.StatusLocked, map[string]any{"error_code": "BULK_TRANSFER_BLOCKED_SHOW"})
						return
					}
					if err != nil {
						writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "BULK_TRANSFER_UNAVAILABLE"})
						return
					}
					reader.ReadSeeker = bulk.NewGuardedReadSeeker(ctx, s.bulk, id, file)
					defer func() {
						if reader.denied || ctx.Err() != nil {
							s.bulk.Cancel(id, "browser media transfer ended")
						} else {
							s.bulk.Complete(id)
						}
					}()
				}
				if !authorize() {
					reader.denied = true
					writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "MEDIA_NOT_FOUND"})
					return
				}
				filename := path.Base(strings.ReplaceAll(version.OriginalFilename, "\\", "/"))
				if filename == "." || filename == "/" || filename == ".." {
					filename = "media.bin"
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
				http.ServeContent(w, r, filename, object.CreatedAt, reader)
			}))
	}
}

type authorizedMediaReader struct {
	io.ReadSeeker
	authorize func() bool
	denied    bool
}

func (r *authorizedMediaReader) Read(data []byte) (int, error) {
	if r.denied || !r.authorize() {
		r.denied = true
		return 0, domain.ErrNotFound
	}
	return r.ReadSeeker.Read(data[:min(len(data), 64<<10)])
}
