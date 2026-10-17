package userauth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ProjectCapability grants are independent of the existing global role map.
// They are NOT yet enforced by the Hub routes. A future gated rollout must
// check every Project and media endpoint, GO path and realtime subscription.
type ProjectCapability string

const (
	CapabilityProjectView     ProjectCapability = "project.view"
	CapabilityProjectRename   ProjectCapability = "project.rename"
	CapabilityProjectEdit     ProjectCapability = "project.edit"
	CapabilityCueEdit         ProjectCapability = "cue.edit"
	CapabilitySnapshotPublish ProjectCapability = "snapshot.publish"
	CapabilityRehearsal       ProjectCapability = "rehearsal.control"
	CapabilityShowGo          ProjectCapability = "show.go"
	CapabilityShowEnterExit   ProjectCapability = "show.enter_exit"
	CapabilityDeviceConfigure ProjectCapability = "device.configure"
	CapabilityMediaView       ProjectCapability = "media.view"
	CapabilityMediaUpload     ProjectCapability = "media.upload"
	CapabilityMediaManage     ProjectCapability = "media.manage"
	CapabilityAudioControl    ProjectCapability = "audio.control"
	CapabilityExport          ProjectCapability = "project.export"
)

var allowedProjectCapabilities = map[ProjectCapability]bool{
	CapabilityProjectView:true, CapabilityProjectRename:true,
	CapabilityProjectEdit:true, CapabilityCueEdit:true,
	CapabilitySnapshotPublish:true, CapabilityRehearsal:true,
	CapabilityShowGo:true, CapabilityShowEnterExit:true,
	CapabilityDeviceConfigure:true, CapabilityMediaView:true,
	CapabilityMediaUpload:true, CapabilityMediaManage:true,
	CapabilityAudioControl:true, CapabilityExport:true,
}

func validProjectCapability(c ProjectCapability) bool {
	return allowedProjectCapabilities[c]
}

// ProjectGrant is an explicitly selected permission on one Project only.
// There is no wildcard Project grant or implicit clone inheritance.
type ProjectGrant struct {
	ProjectID string            `json:"project_id"`
	UserID    string            `json:"user_id"`
	Capability ProjectCapability `json:"capability"`
}

// ReplaceProjectGrants is a single transaction: an enabled OWNER (verified
// fresh against the database, not trusted from a stale UI session) sets the
// complete capability set for an existing non-owner user on one Project.
// Empty permissions revoke all. Unknown permissions and duplicate inputs
// fail before modifying any row.
func (s *Service) ReplaceProjectGrants(ctx context.Context, ownerID, userID, projectID string, permissions []ProjectCapability) error {
	if s == nil || s.db == nil { return errors.New("project ACL database unavailable") }
	if ownerID == "" || userID == "" || projectID == "" ||
		len(permissions) > len(allowedProjectCapabilities) {
		return ErrForbidden
	}
	seen := make(map[ProjectCapability]bool, len(permissions))
	for _, permission := range permissions {
		if !validProjectCapability(permission) || seen[permission] {
			return ErrForbidden
		}
		seen[permission] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin Project grant transaction: %w", err) }
	defer tx.Rollback()
	ownerRole, ownerEnabled, err := readProjectAccessUser(ctx, tx, ownerID)
	if err != nil { return err }
	if ownerRole != RoleOwner || !ownerEnabled { return ErrForbidden }
	targetRole, targetEnabled, err := readProjectAccessUser(ctx, tx, userID)
	if err != nil { return err }
	if !targetEnabled || targetRole == RoleOwner { return ErrForbidden }
	var exists int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) { return ErrForbidden }
	if err != nil { return fmt.Errorf("check grant Project: %w", err) }
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM project_capability_grants WHERE project_id = ? AND user_id = ?",
		projectID, userID); err != nil { return fmt.Errorf("clear Project grants: %w", err) }
	for _, permission := range permissions {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO project_capability_grants
			    (project_id, user_id, capability, granted_by_user_id, granted_at_us)
			VALUES (?, ?, ?, ?, ?)
		`, projectID, userID, string(permission), ownerID, s.now().UTC().UnixMicro())
		if err != nil { return fmt.Errorf("insert Project permission: %w", err) }
	}
	if err := tx.Commit(); err != nil { return fmt.Errorf("commit Project grants: %w", err) }
	return nil
}

// CheckProjectCapability re-reads enabled/role/project/grant every time.
// Revocation takes effect on the next check even for an existing session.
// OWNER may read all existing Projects; other users require an exact grant.
// Important: the SHOW/GO execution fence must independently revalidate at
// dispatch time before this can be used for live runtime authority.
func (s *Service) CheckProjectCapability(ctx context.Context, userID, projectID string, permission ProjectCapability) error {
	if s == nil || s.db == nil { return errors.New("project ACL database unavailable") }
	if userID == "" || projectID == "" || !validProjectCapability(permission) { return ErrForbidden }
	role, enabled, err := readProjectAccessUser(ctx, s.db, userID)
	if err != nil { return err }
	if !enabled { return ErrForbidden }
	var exists int
	err = s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id = ?", projectID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) { return ErrForbidden }
	if err != nil { return fmt.Errorf("check Project existence: %w", err) }
	if role == RoleOwner { return nil }
	var granted int
	err = s.db.QueryRowContext(ctx, `
		SELECT 1 FROM project_capability_grants
		WHERE project_id = ? AND user_id = ? AND capability = ?
	`, projectID, userID, string(permission)).Scan(&granted)
	if errors.Is(err, sql.ErrNoRows) { return ErrForbidden }
	if err != nil { return fmt.Errorf("read Project capability: %w", err) }
	if granted != 1 { return ErrForbidden }
	return nil
}

// ListAccessibleProjectIDs is a *future* filter primitive for the Owner/Operator
// Project picker. Existing GET /api/v1/projects is NOT yet switched to use it:
// filtering only one endpoint while direct-ID routes remain unprotected would
// create a false impression of privacy and must never be shipped as enforcement.
func (s *Service) ListAccessibleProjectIDs(ctx context.Context, userID string) ([]string, error) {
	if s == nil || s.db == nil { return nil, errors.New("project ACL database unavailable") }
	role, enabled, err := readProjectAccessUser(ctx, s.db, userID)
	if err != nil { return nil, err }
	if !enabled { return nil, ErrForbidden }
	query := `SELECT p.project_id FROM projects p
		JOIN project_capability_grants g ON g.project_id=p.project_id
		WHERE g.user_id=? AND g.capability='project.view'
		ORDER BY p.project_id`
	args := []any{userID}
	if role == RoleOwner {
		query, args = "SELECT project_id FROM projects ORDER BY project_id", nil
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil { return nil, fmt.Errorf("list accessible Project IDs: %w", err) }
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil { return nil, err }
		result = append(result, id)
	}
	if err := rows.Err(); err != nil { return nil, err }
	return result, nil
}

// ListGrantsForProject is Owner-only and reads one user/Project without
// revealing unrelated accounts or granting implicit access to Project files.
func (s *Service) ListGrantsForProject(ctx context.Context, ownerID, userID, projectID string) ([]ProjectCapability, error) {
	if s == nil || s.db == nil { return nil, errors.New("project ACL database unavailable") }
	role, enabled, err := readProjectAccessUser(ctx, s.db, ownerID)
	if err != nil { return nil, err }
	if role != RoleOwner || !enabled { return nil, ErrForbidden }
	rows, err := s.db.QueryContext(ctx, `
		SELECT capability FROM project_capability_grants
		WHERE project_id=? AND user_id=? ORDER BY capability
	`, projectID, userID)
	if err != nil { return nil, fmt.Errorf("list project capability grants: %w", err) }
	defer rows.Close()
	grants := make([]ProjectCapability, 0)
	for rows.Next() {
		var permission ProjectCapability
		if err := rows.Scan(&permission); err != nil { return nil, err }
		if !validProjectCapability(permission) { return nil, ErrForbidden }
		grants = append(grants, permission)
	}
	return grants, rows.Err()
}

type projectAccessQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readProjectAccessUser(ctx context.Context, q projectAccessQuerier, id string) (string, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" { return "", false, ErrForbidden }
	var role string
	var enabled int
	err := q.QueryRowContext(ctx, `
		SELECT role, enabled FROM local_users WHERE user_id = ?
	`, id).Scan(&role, &enabled)
	if errors.Is(err, sql.ErrNoRows) { return "", false, ErrForbidden }
	if err != nil { return "", false, fmt.Errorf("read Project user authority: %w", err) }
	return role, enabled == 1, nil
}
