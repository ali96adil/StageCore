package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ResourceKind identifies a stable StageCore row whose Project association
// is defined in the database, not supplied by the requesting browser.
type ResourceKind string

const (
	ResourceCue           ResourceKind = "cue"
	ResourceRevision      ResourceKind = "revision"
	ResourceSnapshot      ResourceKind = "snapshot"
	ResourceSession       ResourceKind = "session"
	ResourceMediaAsset    ResourceKind = "media_asset"
	ResourceMediaVersion  ResourceKind = "media_content_version"
	ResourceMediaLocation ResourceKind = "media_location"
)

// ResolveProjectForResource is a read-only scope resolver for future
// per-Project route middleware. It never interprets resource IDs as Project
// identifiers and never trusts a user-provided ProjectID as ownership proof.
// Missing IDs return ErrNotFound; database errors never grant access.
//
// A raw Vault content hash is deliberately NOT a ResourceKind: the same blob
// may be referenced from separate Projects. For any download, resolve through
// the authorized media_asset_id/content_version_id, never just a hash.
//
// WARNING: qualification for read routes only. It is NOT yet installed in
// production HTTP handlers and must not be used for GO/actuation authorization.
func (s *Service) ResolveProjectForResource(ctx context.Context, kind ResourceKind, resourceID string) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrDenied
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return "", ErrNotFound
	}

	var query string
	switch kind {
	case ResourceCue:
		query = `SELECT r.project_id
			FROM cues c JOIN project_revisions r ON r.revision_id = c.revision_id
			WHERE c.cue_id = ?`
	case ResourceRevision:
		query = `SELECT project_id FROM project_revisions WHERE revision_id = ?`
	case ResourceSnapshot:
		query = `SELECT project_id FROM runtime_snapshots WHERE runtime_snapshot_id = ?`
	case ResourceSession:
		// Do not accept a Session whose Project conflicts with its Snapshot.
		query = `SELECT s.project_id FROM sessions s
			JOIN runtime_snapshots rs ON rs.runtime_snapshot_id = s.runtime_snapshot_id
				AND rs.project_id = s.project_id
			WHERE s.session_id = ?`
	case ResourceMediaAsset:
		query = `SELECT project_id FROM media_assets WHERE media_asset_id = ?`
	case ResourceMediaVersion:
		query = `SELECT a.project_id FROM media_content_versions v
			JOIN media_assets a ON a.media_asset_id = v.media_asset_id
			WHERE v.content_version_id = ?`
	case ResourceMediaLocation:
		query = `SELECT a.project_id FROM media_locations l
			JOIN media_content_versions v ON v.content_version_id = l.content_version_id
			JOIN media_assets a ON a.media_asset_id = v.media_asset_id
			WHERE l.media_location_id = ?`
	default:
		// This includes raw Vault hashes and device/capsule IDs, for which
		// project ownership or operational authority is not yet qualified.
		return "", ErrDenied
	}

	var projectID string
	err := s.db.QueryRowContext(ctx, query, resourceID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s owning Project: %w", kind, err)
	}
	if strings.TrimSpace(projectID) == "" {
		return "", ErrDenied
	}
	return projectID, nil
}

// CheckProjectResourceRead verifies the actual resource owner AND the current
// user's live grant. Media assets, versions and locations require media.read,
// not merely project.view. The Check evaluator also requires project.view for
// every non-Owner grant. This is an unwired qualification helper, not an HTTP
// or raw-content-hash download authorization gate.
func (s *Service) CheckProjectResourceRead(ctx context.Context, userID, expectedProjectID string, kind ResourceKind, resourceID string) error {
	expectedProjectID = strings.TrimSpace(expectedProjectID)
	if expectedProjectID == "" {
		return ErrDenied
	}
	actual, err := s.ResolveProjectForResource(ctx, kind, resourceID)
	if err != nil {
		return err
	}
	if actual != expectedProjectID {
		return ErrDenied
	}
	permission := ProjectView
	switch kind {
	case ResourceMediaAsset, ResourceMediaVersion, ResourceMediaLocation:
		permission = MediaRead
	}
	return s.Check(ctx, userID, actual, permission)
}
