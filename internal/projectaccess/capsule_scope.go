package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// LocalExportCapsuleIdentity is extracted ONLY from a capsule already verified
// by showcapsule.Verify under the Hub's trusted exports root. Never populate
// it directly from a browser-provided ProjectID, title or manifest JSON.
// Imports from another Hub may have no matching local Snapshot; they require
// a separately authorized Owner-only restore path.
type LocalExportCapsuleIdentity struct {
	ProjectID      string
	SnapshotID     string
	RevisionID     string
	SnapshotSHA256 string
}

// CheckLocalExportCapsuleRead qualifies future Project-scoped listing and
// manifest reads of already-exported local Show Capsules. Caller MUST validate
// the directory ID, trusted exports root, canonical checksum and object hashes
// using showcapsule.Verify before invoking this helper. This method is not a
// filesystem path verifier, import planner or backup/restore permission.
//
// A capsule's mutable ProjectName is intentionally not an authority: renaming
// a Show must not orphan its existing exports. The local immutable published
// Snapshot and live capsule.export grant establish the read boundary.
func (s *Service) CheckLocalExportCapsuleRead(
	ctx context.Context, userID, expectedProjectID string, identity LocalExportCapsuleIdentity,
) error {
	if s == nil || s.db == nil || strings.TrimSpace(userID) == "" {
		return ErrDenied
	}
	expectedProjectID = strings.TrimSpace(expectedProjectID)
	if expectedProjectID == "" || identity.ProjectID != expectedProjectID ||
		strings.TrimSpace(identity.SnapshotID) == "" ||
		strings.TrimSpace(identity.RevisionID) == "" ||
		len(identity.SnapshotSHA256) != 64 ||
		identity.SnapshotSHA256 != strings.ToLower(identity.SnapshotSHA256) {
		return ErrDenied
	}

	if err := s.Check(ctx, userID, expectedProjectID, CapsuleExport); err != nil {
		return err
	}
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM runtime_snapshots
		WHERE runtime_snapshot_id = ? AND project_id = ?
		  AND revision_id = ? AND content_hash = ? AND status = 'PUBLISHED'
	`, identity.SnapshotID, expectedProjectID, identity.RevisionID, identity.SnapshotSHA256).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	if err != nil {
		return fmt.Errorf("qualify local export capsule Snapshot: %w", err)
	}
	if exists != 1 {
		return ErrDenied
	}
	// Recheck against grant revocation just before returning. An eventual
	// HTTP/streaming route still needs a reviewed output-time revoke fence.
	return s.Check(ctx, userID, expectedProjectID, CapsuleExport)
}
