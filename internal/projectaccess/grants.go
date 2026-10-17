// Package projectaccess stages future project-scoped access rules.
// Staged grants are NOT yet enforced by StageCore's Operator/GO/Media APIs.
// Never interpret a successful staging write as an active access restriction.
package projectaccess

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/userauth"
)

var (
	ErrForbidden = errors.New("projectaccess: only an enabled OWNER may stage grants")
	ErrInvalid = errors.New("projectaccess: invalid staged capabilities")
	ErrNotFound = errors.New("projectaccess: project or user not found")
)

const (
	CapabilityProjectView = "project.view"
	CapabilityProjectRename = "project.rename"
	CapabilityProjectEdit = "project.edit"
	CapabilityMediaRead = "media.read"
	CapabilityMediaUpload = "media.upload"
	CapabilityMediaManage = "media.manage"
	CapabilityCueEdit = "cue.edit"
	CapabilitySnapshotPublish = "snapshot.publish"
	CapabilityRehearsalControl = "rehearsal.control"
	CapabilityShowGO = "show.go"
	CapabilityShowEnterExit = "show.enter_exit"
	CapabilityDeviceConfigure = "device.configure"
	CapabilityProjectExport = "project.export"
)

var allowed = []string{
	CapabilityProjectView, CapabilityProjectRename, CapabilityProjectEdit,
	CapabilityMediaRead, CapabilityMediaUpload, CapabilityMediaManage,
	CapabilityCueEdit, CapabilitySnapshotPublish, CapabilityRehearsalControl,
	CapabilityShowGO, CapabilityShowEnterExit, CapabilityDeviceConfigure,
	CapabilityProjectExport,
}

func AvailableCapabilities() []string {
	return append([]string(nil), allowed...)
}

type Grant struct {
	ProjectID string `json:"project_id"`
	UserID string `json:"user_id"`
	Username string `json:"username"`
	Role string `json:"role"`
	Capabilities []string `json:"capabilities"`
}

type Repository struct {
	db *sql.DB
	now func() time.Time
}

func New(db *sql.DB) *Repository {
	return &Repository{db:db, now:time.Now}
}

// ReplaceStagedGrant atomically replaces all staged capabilities for one
// NON-OWNER user in one Project. No runtime authorization is changed.
// A write is refused if the actor is no longer an enabled OWNER.
func (r *Repository) ReplaceStagedGrant(ctx context.Context, actorID, projectID, targetID string, input []string) (Grant,error) {
	if r == nil || r.db == nil { return Grant{}, errors.New("projectaccess: database unavailable") }
	actorID, projectID, targetID = strings.TrimSpace(actorID), strings.TrimSpace(projectID), strings.TrimSpace(targetID)
	if actorID=="" || projectID=="" || targetID=="" || actorID==targetID {
		return Grant{},ErrInvalid
	}
	caps,err:=normalize(input)
	if err!=nil {return Grant{},err}
	tx,err:=r.db.BeginTx(ctx,nil)
	if err!=nil {return Grant{},fmt.Errorf("begin staged access update: %w",err)}
	defer tx.Rollback()
	var role string
	var enabled int
	err=tx.QueryRowContext(ctx,`SELECT role,enabled FROM local_users WHERE user_id=?`,actorID).Scan(&role,&enabled)
	if errors.Is(err,sql.ErrNoRows) {return Grant{},ErrForbidden}
	if err!=nil{return Grant{},fmt.Errorf("validate staging actor: %w",err)}
	if role!=userauth.RoleOwner||enabled!=1 {return Grant{},ErrForbidden}
	var projectExists int
	err=tx.QueryRowContext(ctx,`SELECT 1 FROM projects WHERE project_id=?`,projectID).Scan(&projectExists)
	if errors.Is(err,sql.ErrNoRows){return Grant{},ErrNotFound}
	if err!=nil{return Grant{},fmt.Errorf("validate target project: %w",err)}
	var username string
	err=tx.QueryRowContext(ctx,`SELECT username,role,enabled FROM local_users WHERE user_id=?`,targetID).Scan(&username,&role,&enabled)
	if errors.Is(err,sql.ErrNoRows){return Grant{},ErrNotFound}
	if err!=nil{return Grant{},fmt.Errorf("validate target account: %w",err)}
	if role==userauth.RoleOwner || enabled!=1 {return Grant{},ErrInvalid}
	prior,err:=capabilitiesForTx(ctx,tx,projectID,targetID)
	if err!=nil{return Grant{},err}
	// A no-op request does not manufacture an audit change or new timestamp.
	if sameCaps(prior,caps) {
		return Grant{ProjectID:projectID,UserID:targetID,Username:username,Role:role,Capabilities:caps},nil
	}
	if _,err:=tx.ExecContext(ctx,`DELETE FROM project_access_grants WHERE project_id=? AND user_id=?`,projectID,targetID);err!=nil {
		return Grant{},fmt.Errorf("replace staged grant: %w",err)
	}
	nowUS:=r.now().UTC().UnixMicro()
	for _,cap:=range caps {
		if _,err:=tx.ExecContext(ctx,`INSERT INTO project_access_grants
			(project_id,user_id,capability,granted_by,granted_at_us) VALUES (?,?,?,?,?)`,
			projectID,targetID,cap,actorID,nowUS);err!=nil {
			return Grant{},fmt.Errorf("write staged capability: %w",err)
		}
	}
	before,_:=json.Marshal(prior)
	after,_:=json.Marshal(caps)
	changeID,err:=stageid.New()
	if err!=nil{return Grant{},err}
	if _,err:=tx.ExecContext(ctx,`INSERT INTO project_access_grant_changes
		(change_id,project_id,target_user_id,actor_user_id,previous_capabilities_json,new_capabilities_json,changed_at_us)
		VALUES (?,?,?,?,?,?,?)`,changeID,projectID,targetID,actorID,string(before),string(after),nowUS);err!=nil{
		return Grant{},fmt.Errorf("audit staged grant: %w",err)
	}
	if err:=tx.Commit();err!=nil{return Grant{},fmt.Errorf("commit staged grant: %w",err)}
	return Grant{ProjectID:projectID,UserID:targetID,Username:username,Role:role,Capabilities:caps},nil
}

// ListStagedGrants returns ONLY explicitly staged memberships for one Project.
// It does not grant authorization and is restricted to an enabled OWNER.
func (r *Repository) ListStagedGrants(ctx context.Context, actorID, projectID string) ([]Grant,error) {
	if r==nil||r.db==nil{return nil,errors.New("projectaccess: database unavailable")}
	if err:=r.requireOwner(ctx,actorID);err!=nil{return nil,err}
	var one int
	err:=r.db.QueryRowContext(ctx,`SELECT 1 FROM projects WHERE project_id=?`,strings.TrimSpace(projectID)).Scan(&one)
	if errors.Is(err,sql.ErrNoRows){return nil,ErrNotFound}
	if err!=nil{return nil,fmt.Errorf("read staged project: %w",err)}
	rows,err:=r.db.QueryContext(ctx,`SELECT g.user_id,u.username,u.role,g.capability
		FROM project_access_grants g JOIN local_users u ON u.user_id=g.user_id
		WHERE g.project_id=? AND u.enabled=1
		ORDER BY u.username COLLATE NOCASE,g.capability`,projectID)
	if err!=nil{return nil,fmt.Errorf("read staged memberships: %w",err)}
	defer rows.Close()
	out:=make([]Grant,0)
	for rows.Next(){
		var id,username,role,cap string
		if err:=rows.Scan(&id,&username,&role,&cap);err!=nil{return nil,err}
		if len(out)==0||out[len(out)-1].UserID!=id{
			out=append(out,Grant{ProjectID:projectID,UserID:id,Username:username,Role:role,Capabilities:[]string{}})
		}
		out[len(out)-1].Capabilities=append(out[len(out)-1].Capabilities,cap)
	}
	return out,rows.Err()
}

// Preview checks the future scoped decision. It is NEVER used by current
// Operator routes and must not be described as an active security barrier.
func (r *Repository) Preview(ctx context.Context, actorID, targetID, projectID, capability string) (bool,error) {
	if err:=r.requireOwner(ctx,actorID);err!=nil{return false,err}
	if !validCapability(capability){return false,ErrInvalid}
	var allowed int
	err:=r.db.QueryRowContext(ctx,`SELECT EXISTS(
		SELECT 1 FROM project_access_grants g
		JOIN local_users u ON u.user_id=g.user_id AND u.enabled=1
		WHERE g.project_id=? AND g.user_id=? AND g.capability=?
	)`,projectID,targetID,capability).Scan(&allowed)
	if err!=nil{return false,err}
	return allowed==1,nil
}

func (r *Repository) requireOwner(ctx context.Context, id string) error {
	if r==nil||r.db==nil{return ErrForbidden}
	var role string;var enabled int
	err:=r.db.QueryRowContext(ctx,`SELECT role,enabled FROM local_users WHERE user_id=?`,strings.TrimSpace(id)).Scan(&role,&enabled)
	if err!=nil || role!=userauth.RoleOwner || enabled!=1{return ErrForbidden}
	return nil
}

func capabilitiesForTx(ctx context.Context, tx *sql.Tx, projectID,targetID string)([]string,error){
	rows,err:=tx.QueryContext(ctx,`SELECT capability FROM project_access_grants
		WHERE project_id=? AND user_id=? ORDER BY capability`,projectID,targetID)
	if err!=nil{return nil,err}
	defer rows.Close()
	out:=make([]string,0)
	for rows.Next(){var cap string;if err:=rows.Scan(&cap);err!=nil{return nil,err};out=append(out,cap)}
	return out,rows.Err()
}

func normalize(values []string)([]string,error){
	if len(values)>len(allowed){return nil,ErrInvalid}
	seen:=make(map[string]bool,len(values))
	for _,cap:=range values{
		if cap!=strings.TrimSpace(cap) || !validCapability(cap) || seen[cap]{return nil,ErrInvalid}
		seen[cap]=true
	}
	if len(seen)>0 && !seen[CapabilityProjectView] {return nil,ErrInvalid}
	out:=make([]string,0,len(seen))
	for cap:=range seen {out=append(out,cap)}
	sort.Strings(out)
	return out,nil
}
func validCapability(cap string)bool{
	for _,c:=range allowed{if cap==c{return true}}
	return false
}
func sameCaps(a,b []string)bool {
	if len(a)!=len(b){return false}
	for i:=range a{if a[i]!=b[i]{return false}}
	return true
}
