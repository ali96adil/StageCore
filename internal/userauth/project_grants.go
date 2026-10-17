package userauth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SC-024 phase 1: a capability registry, not an enforcement switch.
// Existing HTTP routes still use global role permissions. Do not present
// these grants as isolation until ALL project-aware read/write/runtime and
// media endpoints use the project-scoped checks, including realtime events.

type ProjectCapability string

const (
	CapProjectView    ProjectCapability = "project.view"
	CapProjectRename  ProjectCapability = "project.rename"
	CapProjectEdit    ProjectCapability = "project.edit"
	CapMediaRead      ProjectCapability = "media.read"
	CapMediaUpload    ProjectCapability = "media.upload"
	CapMediaManage    ProjectCapability = "media.manage"
	CapCueEdit        ProjectCapability = "cue.edit"
	CapSnapshotPublish ProjectCapability = "snapshot.publish"
	CapRehearsalControl ProjectCapability = "rehearsal.control"
	CapShowGO         ProjectCapability = "show.go"
	CapShowEnterExit  ProjectCapability = "show.enter_exit"
	CapDeviceConfigure ProjectCapability = "device.configure"
	CapExport         ProjectCapability = "export"
)

var ErrProjectGrantNotFound = errors.New("project grant not found")
var ErrProjectNotFound = errors.New("project not found")

type ProjectGrant struct {
	ProjectID   string            `json:"project_id"`
	UserID      string            `json:"user_id"`
	Capability  ProjectCapability `json:"capability"`
	GrantedBy   string            `json:"granted_by_user_id"`
	GrantedAt   time.Time         `json:"granted_at"`
}

type ProjectGrantAudit struct {
	EventID      int64             `json:"event_id"`
	ProjectID    string            `json:"project_id"`
	TargetUserID string            `json:"target_user_id"`
	ActorUserID  string            `json:"actor_user_id"`
	Capability   ProjectCapability `json:"capability"`
	Action       string            `json:"action"`
	CreatedAt    time.Time         `json:"created_at"`
}

func validProjectCapability(cap ProjectCapability) bool {
	switch cap {
	case CapProjectView, CapProjectRename, CapProjectEdit,
		CapMediaRead, CapMediaUpload, CapMediaManage, CapCueEdit,
		CapSnapshotPublish, CapRehearsalControl, CapShowGO,
		CapShowEnterExit, CapDeviceConfigure, CapExport:
		return true
	}
	return false
}

// A Viewer cannot receive a hidden control capability merely because the
// owner entered its name. OPERATOR / TECHNICIAN grants are bounded to this
// project's capability set; no global user/secret/backup admin permissions.
func capabilityAllowedForRole(role string, cap ProjectCapability) bool {
	if role == RoleViewer {
		return cap == CapProjectView || cap == CapMediaRead
	}
	return role == RoleOperator || role == RoleTechnician
}

func (s *Service) requireGrantOwner(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, actorID string) error {
	var role string
	var enabled int
	err := q.QueryRowContext(ctx, "SELECT role, enabled FROM local_users WHERE user_id = ?", actorID).Scan(&role, &enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { return ErrForbidden }
		return fmt.Errorf("verify project grant owner: %w", err)
	}
	if enabled != 1 || role != RoleOwner {
		return ErrForbidden
	}
	return nil
}

func (s *Service) requireGrantTarget(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, targetID string, cap ProjectCapability) error {
	var role string
	var enabled int
	err := q.QueryRowContext(ctx, "SELECT role, enabled FROM local_users WHERE user_id = ?", targetID).Scan(&role, &enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { return ErrForbidden }
		return fmt.Errorf("verify project grant user: %w", err)
	}
	if enabled != 1 || !capabilityAllowedForRole(role, cap) {
		return ErrForbidden
	}
	return nil
}

func ensureGrantProject(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, projectID string) error {
	var found string
	err := q.QueryRowContext(ctx, "SELECT project_id FROM projects WHERE project_id = ?", projectID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) { return ErrProjectNotFound }
	if err != nil { return fmt.Errorf("verify grant project: %w", err) }
	return nil
}

// GrantProjectCapability adds an exact capability for a single Project.
// Grant writes and their audit record share one transaction. Calling twice
// is idempotent and does not falsely record a second grant action.
func (s *Service) GrantProjectCapability(ctx context.Context, actorID, targetID, projectID string, cap ProjectCapability) (ProjectGrant, error) {
	actorID, targetID, projectID = strings.TrimSpace(actorID), strings.TrimSpace(targetID), strings.TrimSpace(projectID)
	if actorID == "" || targetID == "" || projectID == "" || !validProjectCapability(cap) {
		return ProjectGrant{}, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return ProjectGrant{}, fmt.Errorf("begin project grant: %w", err) }
	defer tx.Rollback()
	if err := s.requireGrantOwner(ctx, tx, actorID); err != nil { return ProjectGrant{}, err }
	if err := s.requireGrantTarget(ctx, tx, targetID, cap); err != nil { return ProjectGrant{}, err }
	if err := ensureGrantProject(ctx, tx, projectID); err != nil { return ProjectGrant{}, err }
	now := s.now().UTC().UnixMicro()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO project_capability_grants
		(project_id,user_id,capability,granted_by_user_id,granted_at_us)
		VALUES(?,?,?,?,?)
		ON CONFLICT(project_id,user_id,capability) DO NOTHING
	`, projectID, targetID, string(cap), actorID, now)
	if err != nil { return ProjectGrant{}, fmt.Errorf("insert project grant: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return ProjectGrant{}, err }
	if affected == 1 {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO project_capability_grant_audit
			(project_id,target_user_id,actor_user_id,capability,action,created_at_us)
			VALUES(?,?,?,?,'GRANT',?)
		`, projectID, targetID, actorID, string(cap), now)
		if err != nil { return ProjectGrant{}, fmt.Errorf("audit project grant: %w", err) }
	}
	var grant ProjectGrant
	var timestamp int64
	err = tx.QueryRowContext(ctx, `
		SELECT project_id,user_id,capability,granted_by_user_id,granted_at_us
		FROM project_capability_grants
		WHERE project_id=? AND user_id=? AND capability=?
	`, projectID, targetID, string(cap)).Scan(
		&grant.ProjectID, &grant.UserID, &grant.Capability, &grant.GrantedBy, &timestamp)
	if err != nil { return ProjectGrant{}, fmt.Errorf("read project grant: %w", err) }
	grant.GrantedAt = time.UnixMicro(timestamp).UTC()
	if err := tx.Commit(); err != nil { return ProjectGrant{}, fmt.Errorf("commit project grant: %w", err) }
	return grant, nil
}

// RevokeProjectCapability deletes a current grant and writes an audit entry.
// No existing runtime/browser session can cache a successful access check;
// eventual enforcement must query fresh policy before each request/dispatch.
func (s *Service) RevokeProjectCapability(ctx context.Context, actorID, targetID, projectID string, cap ProjectCapability) error {
	actorID, targetID, projectID = strings.TrimSpace(actorID), strings.TrimSpace(targetID), strings.TrimSpace(projectID)
	if actorID == "" || targetID == "" || projectID == "" || !validProjectCapability(cap) { return ErrForbidden }
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin project grant revoke: %w", err) }
	defer tx.Rollback()
	if err := s.requireGrantOwner(ctx, tx, actorID); err != nil { return err }
	if err := ensureGrantProject(ctx, tx, projectID); err != nil { return err }
	result, err := tx.ExecContext(ctx, `
		DELETE FROM project_capability_grants
		WHERE project_id = ? AND user_id = ? AND capability = ?
	`, projectID, targetID, string(cap))
	if err != nil { return fmt.Errorf("delete project grant: %w", err) }
	affected, err := result.RowsAffected()
	if err != nil { return err }
	if affected != 1 { return ErrProjectGrantNotFound }
	_, err = tx.ExecContext(ctx, `
		INSERT INTO project_capability_grant_audit
		(project_id,target_user_id,actor_user_id,capability,action,created_at_us)
		VALUES(?,?,?,?,'REVOKE',?)
	`, projectID, targetID, actorID, string(cap), s.now().UTC().UnixMicro())
	if err != nil { return fmt.Errorf("audit project grant revoke: %w", err) }
	if err := tx.Commit(); err != nil { return fmt.Errorf("commit project grant revoke: %w", err) }
	return nil
}

// HasProjectCapability is an independent, dynamically evaluated predicate.
// It DOES NOT replace existing global role-based middleware until rollout
// explicitly switches every scoped route, including list and realtime paths.
func (s *Service) HasProjectCapability(ctx context.Context, userID, projectID string, cap ProjectCapability) (bool, error) {
	userID, projectID = strings.TrimSpace(userID), strings.TrimSpace(projectID)
	if userID == "" || projectID == "" || !validProjectCapability(cap) { return false, nil }
	var role string
	var enabled int
	err := s.db.QueryRowContext(ctx, "SELECT role, enabled FROM local_users WHERE user_id=?", userID).Scan(&role, &enabled)
	if errors.Is(err, sql.ErrNoRows) { return false, nil }
	if err != nil { return false, fmt.Errorf("read project user: %w", err) }
	if enabled != 1 { return false, nil }
	if role == RoleOwner {
		// Even the owner cannot authorize a fabricated Project ID.
		var found int
		err = s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE project_id=?", projectID).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) { return false, nil }
		return err == nil, err
	}
	if !capabilityAllowedForRole(role, cap) { return false, nil }
	var permitted int
	err = s.db.QueryRowContext(ctx, `
		SELECT 1 FROM project_capability_grants
		WHERE user_id=? AND project_id=? AND capability=?
	`, userID, projectID, string(cap)).Scan(&permitted)
	if errors.Is(err, sql.ErrNoRows) { return false, nil }
	if err != nil { return false, fmt.Errorf("check project permission: %w", err) }
	return permitted == 1, nil
}

// ProjectGrantAuditHistory is read-only and OWNER-only. The audit does not
// expose passwords, browser tokens or device authority material.
func (s *Service) ProjectGrantAuditHistory(ctx context.Context, actorID, projectID string) ([]ProjectGrantAudit, error) {
	if strings.TrimSpace(projectID) == "" { return nil, ErrProjectNotFound }
	if err := s.requireGrantOwner(ctx, s.db, strings.TrimSpace(actorID)); err != nil { return nil, err }
	if err := ensureGrantProject(ctx, s.db, strings.TrimSpace(projectID)); err != nil { return nil, err }
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id,project_id,target_user_id,actor_user_id,capability,action,created_at_us
		FROM project_capability_grant_audit
		WHERE project_id = ? ORDER BY event_id DESC LIMIT 500
	`, projectID)
	if err != nil { return nil, fmt.Errorf("read project grant history: %w", err) }
	defer rows.Close()
	list := make([]ProjectGrantAudit, 0)
	for rows.Next() {
		var g ProjectGrantAudit
		var timestamp int64
		if err := rows.Scan(&g.EventID, &g.ProjectID, &g.TargetUserID, &g.ActorUserID, &g.Capability, &g.Action, &timestamp); err != nil {
			return nil, fmt.Errorf("scan project grant history: %w", err)
		}
		g.CreatedAt = time.UnixMicro(timestamp).UTC()
		list = append(list, g)
	}
	return list, rows.Err()
}
