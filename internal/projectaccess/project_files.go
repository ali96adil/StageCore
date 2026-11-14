package projectaccess

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// ProjectFile is a logical Vault-backed Project file listing item. It contains
// NO filesystem path or authorization token. Each Media Content Version is a
// separate row; multiple Projects may reference the same underlying blob.
type ProjectFile struct {
	MediaAssetID     string `json:"media_asset_id"`
	MediaAssetName   string `json:"media_asset_name"`
	AssetPolicy      string `json:"asset_policy"`
	ContentVersionID string `json:"content_version_id,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	SizeBytes        int64  `json:"size_bytes"`
	Category         string `json:"category"`
}

// ProjectFileWorkspace is a virtual `Projects -> <Show> -> Files` directory.
// A Project exists independently of whether it already has Media Assets.
// ProjectID, never the display name, is the immutable workspace identity.
type ProjectFileWorkspace struct {
	ProjectID   string        `json:"project_id"`
	ProjectName string        `json:"project_name"`
	Files       []ProjectFile `json:"files"`
	NextCursor  string        `json:"next_cursor,omitempty"`
}

var ErrWorkspacePaginationRequired = errors.New("projectaccess: Project Files exceed unpaged qualification limit")

const maxUnpagedProjectFiles = 500

// ListScopedProjectFiles supplies the bounded legacy unpaged view. Only users with live
// Project media.read grants can inspect file names or metadata, including
// empty workspaces. It never enumerates other Projects or exposes Vault paths.
// The future HTTP route must revalidate session/grants at response/stream time;
// this helper alone is NOT an atomic access-control cutover.
func (s *Service) ListScopedProjectFiles(
	ctx context.Context, userID, projectID string,
) (ProjectFileWorkspace, error) {
	return s.listScopedProjectFiles(ctx, userID, projectID, maxUnpagedProjectFiles, false, fileCursor{})
}

type fileCursor struct {
	Project string
	Name    string `json:"-"`
	Asset   string
	Created int64 `json:"-"`
	Version string
}

// ListScopedProjectFilesPage preserves the unpaged sort and resumes after the
// last returned asset/version. Each page independently checks live grants.
func (s *Service) ListScopedProjectFilesPage(ctx context.Context, userID, projectID string, size int, cursor string) (ProjectFileWorkspace, error) {
	projectID = strings.TrimSpace(projectID)
	if size < 1 || size > 100 || len(cursor) > 4096 {
		return ProjectFileWorkspace{}, ErrInvalid
	}
	var after fileCursor
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(decoded, &after) != nil || after.Project != projectID || after.Asset == "" {
			return ProjectFileWorkspace{}, ErrInvalid
		}
	}
	return s.listScopedProjectFiles(ctx, userID, projectID, size, true, after)
}

func (s *Service) listScopedProjectFiles(ctx context.Context, userID, projectID string, size int, paginated bool, after fileCursor) (ProjectFileWorkspace, error) {
	if s == nil || s.db == nil || strings.TrimSpace(userID) == "" ||
		strings.TrimSpace(projectID) == "" {
		return ProjectFileWorkspace{}, ErrDenied
	}
	projectID = strings.TrimSpace(projectID)
	if err := s.Check(ctx, userID, projectID, MediaRead); err != nil {
		return ProjectFileWorkspace{}, err
	}
	var projectName string
	if err := s.db.QueryRowContext(ctx,
		`SELECT name FROM projects WHERE project_id = ?`, projectID,
	).Scan(&projectName); err != nil {
		// A missing Project and a forbidden Project must look identical
		// to callers of an eventual HTTP wrapper.
		return ProjectFileWorkspace{}, ErrDenied
	}
	if after.Asset != "" {
		err := s.db.QueryRowContext(ctx, `
			SELECT a.name, -COALESCE(v.created_at_us, 0)
			FROM media_assets a
			LEFT JOIN media_content_versions v ON v.media_asset_id = a.media_asset_id
			WHERE a.project_id = ? AND a.media_asset_id = ? AND COALESCE(v.content_version_id, '') = ?
		`, projectID, after.Asset, after.Version).Scan(&after.Name, &after.Created)
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectFileWorkspace{}, ErrInvalid
		}
		if err != nil {
			return ProjectFileWorkspace{}, fmt.Errorf("resolve Project Files cursor: %w", err)
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.media_asset_id, a.name, a.asset_policy,
		       COALESCE(v.content_version_id, ''),
		       COALESCE(v.original_filename, ''),
		       COALESCE(v.size_bytes, 0)
		FROM media_assets a
		LEFT JOIN media_content_versions v ON v.media_asset_id = a.media_asset_id
		WHERE a.project_id = ? AND (? = '' OR
		   (a.name COLLATE NOCASE, a.media_asset_id, -COALESCE(v.created_at_us, 0), COALESCE(v.content_version_id, ''))
		   > (?, ?, ?, ?))
		ORDER BY a.name COLLATE NOCASE, a.media_asset_id,
		         -COALESCE(v.created_at_us, 0), COALESCE(v.content_version_id, '')
		LIMIT ?
	`, projectID, after.Asset, after.Name, after.Asset, after.Created, after.Version, size+1)
	if err != nil {
		return ProjectFileWorkspace{}, fmt.Errorf("query Project Files: %w", err)
	}
	defer rows.Close()
	files := make([]ProjectFile, 0)
	var last fileCursor
	var nextCursor string
	for rows.Next() {
		var item ProjectFile
		if err := rows.Scan(&item.MediaAssetID, &item.MediaAssetName, &item.AssetPolicy,
			&item.ContentVersionID, &item.OriginalFilename, &item.SizeBytes); err != nil {
			return ProjectFileWorkspace{}, fmt.Errorf("scan Project File: %w", err)
		}
		if len(files) == size {
			if !paginated {
				return ProjectFileWorkspace{}, ErrWorkspacePaginationRequired
			}
			encoded, err := json.Marshal(last)
			if err != nil {
				return ProjectFileWorkspace{}, err
			}
			nextCursor = base64.RawURLEncoding.EncodeToString(encoded)
			break
		}
		filename := item.OriginalFilename
		if filename == "" {
			filename = item.MediaAssetName
		}
		item.Category = projectFileCategory(filename)
		files = append(files, item)
		last = fileCursor{Project: projectID, Asset: item.MediaAssetID, Version: item.ContentVersionID}
	}
	if err := rows.Err(); err != nil {
		return ProjectFileWorkspace{}, fmt.Errorf("iterate Project Files: %w", err)
	}
	if err := rows.Close(); err != nil {
		return ProjectFileWorkspace{}, fmt.Errorf("close Project Files cursor: %w", err)
	}
	// Revalidate grants after reading. Future streaming endpoints also need
	// live per-chunk authorization and a fully reviewed revocation fence.
	if err := s.Check(ctx, userID, projectID, MediaRead); err != nil {
		return ProjectFileWorkspace{}, err
	}
	return ProjectFileWorkspace{
		ProjectID: projectID, ProjectName: projectName, Files: files, NextCursor: nextCursor,
	}, nil
}

// Categorization is presentation only: an extension is NOT proof of MIME type,
// safe decodability, or authorization to serve the bytes to a browser.
func projectFileCategory(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".wav", ".mp3", ".flac", ".aiff", ".aif", ".m4a", ".aac", ".ogg", ".opus":
		return "AUDIO"
	case ".mp4", ".mov", ".mkv", ".webm", ".avi", ".m4v":
		return "VIDEO"
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".heic", ".tif", ".tiff", ".bmp":
		return "IMAGE"
	default:
		return "OTHER"
	}
}
