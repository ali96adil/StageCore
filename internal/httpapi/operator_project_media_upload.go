package httpapi

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/userauth"
	"github.com/ali96adil/StageCore/internal/vault"
)

const maxProjectFileUploadBytes int64 = 1 << 30

var errProjectUploadSessionRevoked = errors.New("project media upload session revoked")

// WithScopedProjectMediaUpload opts in to a project-scoped raw file upload
// endpoint. It requires a live media.upload grant for every streamed chunk;
// the request is staged directly into the existing content-addressed Vault.
func WithScopedProjectMediaUpload(auth *userauth.Service, access *projectaccess.Service,
	projects *store.Store, objects *vault.Vault,
) Option {
	return func(s *Server) {
		if s == nil || s.mux == nil || auth == nil || access == nil || projects == nil || objects == nil {
			return
		}
		s.projectMediaUpload = true
		s.mux.HandleFunc("POST /api/v1/projects/{project_id}/files",
			withAuthenticatedSession(auth, true, func(w http.ResponseWriter, r *http.Request,
				session userauth.Session, token string,
			) {
				uploadProjectMedia(w, r, auth, access, projects, objects, session, token)
			}))
	}
}

func uploadProjectMedia(w http.ResponseWriter, r *http.Request, auth *userauth.Service,
	access *projectaccess.Service, projects *store.Store, objects *vault.Vault,
	session userauth.Session, token string,
) {
	projectID := strings.TrimSpace(r.PathValue("project_id"))
	if !allowScopedProject(w, r, session, access, projectaccess.MediaUpload) {
		return
	}
	if r.ContentLength > maxProjectFileUploadBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error_code": "PROJECT_FILE_TOO_LARGE"})
		return
	}
	filename, err := decodeProjectUploadName(r.Header.Get("X-Project-File-Name"), 255)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_FILE_NAME_INVALID"})
		return
	}
	assetName := ""
	assetNameHeader := r.Header.Get("X-Project-Asset-Name")
	if assetNameHeader != "" {
		assetName, err = decodeProjectUploadName(assetNameHeader, 160)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_FILE_ASSET_NAME_INVALID"})
			return
		}
	} else {
		assetName = strings.TrimSuffix(filename, path.Ext(filename))
		if assetName == "" {
			assetName = filename
		}
	}
	if !validProjectUploadMediaType(filename, r.Header.Get("Content-Type")) {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error_code": "PROJECT_FILE_TYPE_INVALID"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxProjectFileUploadBytes)
	body := &authorizedCaptureReader{source: r.Body, authorize: func() error {
		current, validateErr := auth.ValidatePassive(r.Context(), token)
		if validateErr != nil || current.ID != session.ID || current.User.ID != session.User.ID {
			return errProjectUploadSessionRevoked
		}
		return access.Check(r.Context(), current.User.ID, projectID, projectaccess.MediaUpload)
	}}
	object, err := objects.ImportObject(r.Context(), body)
	if err != nil {
		writeProjectMediaUploadError(w, body.denied, err)
		return
	}
	if object.SizeBytes == 0 {
		_, _ = objects.RemoveObjectIfUnreferenced(context.WithoutCancel(r.Context()), object.ContentHash)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error_code": "PROJECT_FILE_EMPTY"})
		return
	}
	if body.denied == nil {
		body.denied = body.authorize()
	}
	if body.denied != nil {
		_, _ = objects.RemoveObjectIfUnreferenced(context.WithoutCancel(r.Context()), object.ContentHash)
		writeProjectMediaUploadError(w, body.denied, nil)
		return
	}
	writeCtx := projectMutationContext(r, auth, session, userauth.PermissionProjectEdit, access, projectaccess.MediaUpload)
	managed, err := projects.RegisterManagedMedia(writeCtx, store.RegisterManagedMediaParams{
		ProjectID: projectID, Name: assetName, AssetPolicy: store.MediaPolicyManaged,
		ContentHash: object.ContentHash, SizeBytes: object.SizeBytes,
		RelativePath: object.RelativePath, OriginalFilename: filename,
	})
	if err != nil {
		_, _ = objects.RemoveObjectIfUnreferenced(context.WithoutCancel(r.Context()), object.ContentHash)
		if errors.Is(err, projectaccess.ErrDenied) || errors.Is(err, projectaccess.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_NOT_FOUND"})
			return
		}
		if errors.Is(err, userauth.ErrForbidden) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error_code": "FORBIDDEN"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_FILE_REGISTER_FAILED"})
		return
	}
	if err := body.authorize(); err != nil {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error_code":                     "PROJECT_FILE_UPLOAD_AUTHORITY_REVOKED",
			"effect_may_have_been_committed": true,
			"retryable":                      false,
		})
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"media_asset_id":     managed.Asset.ID,
		"content_version_id": managed.Version.ID,
		"content_hash":       managed.Version.ContentHash,
		"size_bytes":         managed.Version.SizeBytes,
	})
}

func decodeProjectUploadName(raw string, maxBytes int) (string, error) {
	if len(raw) == 0 || len(raw) > 1024 {
		return "", errors.New("missing or oversized name")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || !utf8.ValidString(decoded) {
		return "", errors.New("invalid encoded name")
	}
	decoded = strings.TrimSpace(strings.ReplaceAll(decoded, "\\", "/"))
	decoded = path.Base(decoded)
	if decoded == "" || decoded == "." || decoded == ".." || len(decoded) > maxBytes {
		return "", errors.New("invalid name")
	}
	for _, char := range decoded {
		if unicode.IsControl(char) {
			return "", errors.New("control character in name")
		}
	}
	return decoded, nil
}

func validProjectUploadMediaType(filename, raw string) bool {
	if len(raw) == 0 || len(raw) > 256 {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return false
	}
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	if mediaType == "application/octet-stream" {
		return true
	}
	expectedTopLevel := map[string]string{
		"AUDIO": "audio", "VIDEO": "video", "IMAGE": "image",
	}[projectaccess.ProjectFileCategory(filename)]
	return expectedTopLevel == "" || parts[0] == expectedTopLevel
}

func writeProjectMediaUploadError(w http.ResponseWriter, authorityErr, importErr error) {
	if errors.Is(authorityErr, errProjectUploadSessionRevoked) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error_code": "SESSION_INVALID"})
		return
	}
	if errors.Is(authorityErr, projectaccess.ErrDenied) || errors.Is(authorityErr, projectaccess.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error_code": "PROJECT_NOT_FOUND"})
		return
	}
	var tooLarge *http.MaxBytesError
	if errors.As(importErr, &tooLarge) {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error_code": "PROJECT_FILE_TOO_LARGE"})
		return
	}
	if errors.Is(importErr, storagehealth.ErrRuntimeReserve) {
		writeJSON(w, http.StatusInsufficientStorage, map[string]any{"error_code": "STORAGE_RUNTIME_RESERVE"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error_code": "PROJECT_FILE_UPLOAD_FAILED"})
}
