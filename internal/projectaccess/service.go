// Package projectaccess implements StageCore's future Project-scoped access
// policy. This initial package is intentionally NOT installed into HTTP, GO,
// Vault, Companion or device routes; migration/data alone is not enforcement.
package projectaccess

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Permission string

const (
	ProjectView       Permission = "project.view"
	ProjectRename     Permission = "project.rename"
	ProjectEdit       Permission = "project.edit"
	CueEdit           Permission = "cue.edit"
	MediaRead         Permission = "media.read"
	MediaUpload       Permission = "media.upload"
	MediaManage       Permission = "media.manage"
	SnapshotPublish   Permission = "snapshot.publish"
	RehearsalControl  Permission = "rehearsal.control"
	ShowGO            Permission = "show.go"
	ShowEnterExit     Permission = "show.enter_exit"
	DeviceConfigure   Permission = "device.configure"
	LightingManual    Permission = "lighting.manual"
	AudioControl      Permission = "audio.control"
	CapsuleExport     Permission = "capsule.export"
)

var permissionSet = map[Permission]bool{
	ProjectView: true, ProjectRename: true, ProjectEdit: true, CueEdit: true,
	MediaRead: true, MediaUpload: true, MediaManage: true,
	SnapshotPublish: true, RehearsalControl: true, ShowGO: true,
	ShowEnterExit: true, DeviceConfigure: true, LightingManual: true, AudioControl: true,
	CapsuleExport: true,
}

var (
	ErrDenied = errors.New("projectaccess: permission denied")
	ErrInvalid = errors.New("projectaccess: invalid grant request")
	ErrNotFound = errors.New("projectaccess: user or project does not exist")
)

type Service struct {
	db *sql.DB
	now func() time.Time
}

func New(db *sql.DB) (*Service, error) {
	if db == nil { return nil, fmt.Errorf("%w: database required", ErrInvalid) }
	return &Service{db: db, now: time.Now}, nil
}

// SetPermissions completely replaces one non-OWNER user's rights to exactly
// one existing Project. An empty list atomically removes their membership and
// all permission grants. Missing ProjectView is invalid for nonempty lists.
// This method is not an HTTP endpoint: a separate authenticated Owner-only,
// audited route must be reviewed before it is exposed to operators.
func (s *Service) SetPermissions(ctx context.Context, actingOwnerID, targetUserID, projectID string, permissions []Permission) error {
	if s == nil { return ErrInvalid }
	actingOwnerID = strings.TrimSpace(actingOwnerID)
	targetUserID = strings.TrimSpace(targetUserID)
	projectID = strings.TrimSpace(projectID)
	if actingOwnerID == "" || targetUserID == "" || projectID == "" {
		return ErrInvalid
	}
	unique := map[Permission]bool{}
	for _, permission := range permissions {
		if !permissionSet[permission] || unique[permission] {
			return fmt.Errorf("%w: unsupported or repeated capability", ErrInvalid)
		}
		unique[permission] = true
	}
	if len(unique)>0 && !unique[ProjectView] {
		return fmt.Errorf("%w: project.view required for a membership", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin project grants: %w", err) }
	defer tx.Rollback()

	var ownerRole string
	var ownerEnabled int
	err = tx.QueryRowContext(ctx, `SELECT role, enabled FROM local_users WHERE user_id = ?`, actingOwnerID).Scan(&ownerRole, &ownerEnabled)
	if errors.Is(err, sql.ErrNoRows) { return ErrDenied }
	if err != nil { return fmt.Errorf("read granting Owner: %w", err) }
	if ownerRole != "OWNER" || ownerEnabled != 1 { return ErrDenied }

	var targetRole string
	var targetEnabled int
	err = tx.QueryRowContext(ctx, `SELECT role, enabled FROM local_users WHERE user_id = ?`, targetUserID).Scan(&targetRole, &targetEnabled)
	if errors.Is(err, sql.ErrNoRows) { return ErrNotFound }
	if err != nil { return fmt.Errorf("read target user: %w", err) }
	if targetRole == "OWNER" { return fmt.Errorf("%w: Owner always has global access; never use a scoped grant", ErrInvalid) }
	if targetEnabled != 1 { return ErrDenied }
	// VIEWER is a strictly read-only account, even when an Owner specifies
	// broader Project capabilities by mistake. Operators and Technicians may
	// receive extra explicitly scoped rights, but not unknown role strings.
	switch targetRole {
	case "VIEWER":
		for permission := range unique {
			if permission != ProjectView && permission != MediaRead {
				return fmt.Errorf("%w: VIEWER can only view assigned Project and media", ErrDenied)
			}
		}
	case "OPERATOR", "TECHNICIAN":
		// Capability allowlist still applies individually below.
	default:
		return ErrDenied
	}

	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE project_id = ?`, projectID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) { return ErrNotFound }
	if err != nil { return fmt.Errorf("read target project: %w", err) }

	if len(unique) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_memberships WHERE project_id = ? AND user_id = ?`, projectID, targetUserID); err != nil {
			return fmt.Errorf("remove project membership: %w", err)
		}
		return tx.Commit()
	}
	nowUS := s.now().UTC().UnixMicro()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_memberships(project_id,user_id,created_by_user_id,created_at_us,updated_at_us)
		VALUES(?,?,?,?,?)
		ON CONFLICT(project_id,user_id) DO UPDATE SET updated_at_us=excluded.updated_at_us
	`,projectID,targetUserID,actingOwnerID,nowUS,nowUS); err != nil {
		return fmt.Errorf("upsert project membership: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_permission_grants WHERE project_id = ? AND user_id = ?`,projectID,targetUserID); err != nil {
		return fmt.Errorf("clear old project permissions: %w", err)
	}
	keys := make([]string, 0, len(unique))
	for p := range unique { keys = append(keys,string(p)) }
	sort.Strings(keys)
	for _, permission := range keys {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_permission_grants(project_id,user_id,permission,granted_by_user_id,granted_at_us)
			VALUES(?,?,?,?,?)
		`,projectID,targetUserID,permission,actingOwnerID,nowUS); err != nil {
			return fmt.Errorf("grant project permission: %w", err)
		}
	}
	if err := tx.Commit(); err != nil { return fmt.Errorf("commit project grants: %w", err) }
	return nil
}

// Check is an explicit live policy evaluation. No cached session claims:
// disabling a user, changing their role, or revoking one grant immediately
// changes the result. Owner override is fetched from current DB state, not
// trusted from a possibly stale browser session. Unknown projects deny.
// Importantly this method alone does not secure existing HTTP endpoints.
func (s *Service) Check(ctx context.Context, userID, projectID string, permission Permission) error {
	if s == nil || !permissionSet[permission] || strings.TrimSpace(userID)=="" || strings.TrimSpace(projectID)=="" {
		return ErrDenied
	}
	var permitted int
	err := s.db.QueryRowContext(ctx, `
		SELECT CASE WHEN u.role='OWNER' THEN 1
			WHEN u.role IN ('OPERATOR','TECHNICIAN') AND g.permission IS NOT NULL
				AND EXISTS (SELECT 1 FROM project_permission_grants view_grant
					WHERE view_grant.project_id=p.project_id
						AND view_grant.user_id=u.user_id
						AND view_grant.permission='project.view') THEN 1
			WHEN u.role='VIEWER' AND g.permission IS NOT NULL
				AND ? IN ('project.view','media.read')
				AND EXISTS (SELECT 1 FROM project_permission_grants view_grant
					WHERE view_grant.project_id=p.project_id
						AND view_grant.user_id=u.user_id
						AND view_grant.permission='project.view') THEN 1
			ELSE 0 END
		FROM local_users u
		JOIN projects p ON p.project_id = ?
		LEFT JOIN project_permission_grants g ON g.user_id=u.user_id
			AND g.project_id=p.project_id AND g.permission=?
		WHERE u.user_id=? AND u.enabled=1
	`,string(permission),projectID,string(permission),userID).Scan(&permitted)
	if errors.Is(err, sql.ErrNoRows) { return ErrDenied }
	if err != nil { return fmt.Errorf("evaluate project access: %w",err) }
	if permitted != 1 { return ErrDenied }
	return nil
}

// AccessibleProjectIDs is the query contract for a future GET /projects
// replacement: Owner sees all; other roles see ONLY explicit project.view
// grants. This helper must not be confused with current unrestricted routes.
func (s *Service) AccessibleProjectIDs(ctx context.Context, userID string) ([]string,error) {
	if s == nil || strings.TrimSpace(userID)=="" { return nil, ErrDenied }
	rows,err:=s.db.QueryContext(ctx,`
		SELECT p.project_id FROM projects p
		JOIN local_users u ON u.user_id=? AND u.enabled=1
		WHERE u.role='OWNER'
			OR EXISTS (
				SELECT 1 FROM project_permission_grants g
				WHERE g.project_id=p.project_id AND g.user_id=u.user_id
					AND g.permission='project.view'
			)
		ORDER BY p.name COLLATE NOCASE, p.project_id
	`,userID)
	if err!=nil {return nil,fmt.Errorf("list scoped projects: %w",err)}
	defer rows.Close()
	ids:=make([]string,0)
	for rows.Next() {
		var id string
		if err:=rows.Scan(&id);err!=nil{return nil,fmt.Errorf("scan scoped project: %w",err)}
		ids=append(ids,id)
	}
	if err:=rows.Err();err!=nil{return nil,err}
	return ids,nil
}
