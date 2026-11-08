package projectaccess

import (
	"context"
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
}

var ErrWorkspacePaginationRequired = errors.New("projectaccess: Project Files exceed unpaged qualification limit")

const maxUnpagedProjectFiles = 500

// ListScopedProjectFiles is a READ-ONLY, UNWIRED qualification helper for a
// future authenticated Project Files browser route. Only users with live
// Project media.read grants can inspect file names or metadata, including
// empty workspaces. It never enumerates other Projects or exposes Vault paths.
// The future HTTP route must revalidate session/grants at response/stream time;
// this helper alone is NOT an atomic access-control cutover.
func (s *Service) ListScopedProjectFiles(
	ctx context.Context, userID, projectID string,
) (ProjectFileWorkspace, error) {
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.media_asset_id, a.name, a.asset_policy,
		       COALESCE(v.content_version_id, ''),
		       COALESCE(v.original_filename, ''),
		       COALESCE(v.size_bytes, 0)
		FROM media_assets a
		LEFT JOIN media_content_versions v ON v.media_asset_id = a.media_asset_id
		WHERE a.project_id = ?
		ORDER BY a.name COLLATE NOCASE, a.media_asset_id,
		         v.created_at_us DESC, v.content_version_id
		LIMIT ?
	`, projectID, maxUnpagedProjectFiles+1)
	if err != nil {
		return ProjectFileWorkspace{}, fmt.Errorf("query Project Files: %w", err)
	}
	defer rows.Close()
	files := make([]ProjectFile, 0)
	for rows.Next() {
		var item ProjectFile
		if err := rows.Scan(&item.MediaAssetID, &item.MediaAssetName, &item.AssetPolicy,
			&item.ContentVersionID, &item.OriginalFilename, &item.SizeBytes); err != nil {
			return ProjectFileWorkspace{}, fmt.Errorf("scan Project File: %w", err)
		}
		filename := item.OriginalFilename
		if filename == "" {
			filename = item.MediaAssetName
		}
		item.Category = projectFileCategory(filename)
		files = append(files, item)
		if len(files) > maxUnpagedProjectFiles {
			return ProjectFileWorkspace{}, ErrWorkspacePaginationRequired
		}
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
		ProjectID: projectID, ProjectName: projectName, Files: files,
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
