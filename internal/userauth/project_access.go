package userauth

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "slices"
 "strings"
)

type ProjectCapability string
const (
 ProjectView ProjectCapability = "project.view"
 ProjectEdit ProjectCapability = "project.edit"
 ProjectRename ProjectCapability = "project.rename"
 MediaRead ProjectCapability = "media.read"
 MediaUpload ProjectCapability = "media.upload"
 MediaManage ProjectCapability = "media.manage"
 CueEdit ProjectCapability = "cue.edit"
 SnapshotPublish ProjectCapability = "snapshot.publish"
 RehearsalControl ProjectCapability = "rehearsal.control"
 ShowGo ProjectCapability = "show.go"
 ShowEnterExit ProjectCapability = "show.enter_exit"
 DeviceConfigure ProjectCapability = "device.configure"
 ProjectExport ProjectCapability = "project.export"
)
var projectCapabilitySet = []ProjectCapability{
 ProjectView,ProjectEdit,ProjectRename,MediaRead,MediaUpload,
 MediaManage,CueEdit,SnapshotPublish,RehearsalControl,
 ShowGo,ShowEnterExit,DeviceConfigure,ProjectExport,
}
var ErrProjectGrantInvalid = errors.New("invalid Project grant")
var ErrProjectGrantMissing = errors.New("Project grant target not found")

type ProjectGrant struct {
 ProjectID string `json:"project_id"`
 UserID string `json:"user_id"`
 GrantedByUserID string `json:"granted_by_user_id"`
 Capabilities []ProjectCapability `json:"capabilities"`
}

func AvailableProjectCapabilities() []ProjectCapability {
 return slices.Clone(projectCapabilitySet)
}

func validProjectCapability(cap ProjectCapability) bool {
 return slices.Contains(projectCapabilitySet, cap)
}

func checkGrantCaps(values []ProjectCapability, role string) ([]ProjectCapability,error) {
 if len(values)==0||len(values)>len(projectCapabilitySet) {return nil,ErrProjectGrantInvalid}
 seen:=make(map[ProjectCapability]bool,len(values))
 for _,v:=range values {
  if !validProjectCapability(v)||seen[v] {return nil,ErrProjectGrantInvalid}
  if role==RoleViewer && v!=ProjectView && v!=MediaRead {return nil,ErrProjectGrantInvalid}
  seen[v]=true
 }
 if !seen[ProjectView] {return nil,ErrProjectGrantInvalid}
 out:=slices.Clone(values)
 slices.Sort(out)
 return out,nil
}

func (s *Service) ownerCurrent(ctx context.Context,actor string) error {
 var role string
 var enabled int
 err:=s.db.QueryRowContext(ctx,"SELECT role, enabled FROM local_users WHERE user_id = ?",actor).Scan(&role,&enabled)
 if errors.Is(err,sql.ErrNoRows){return ErrForbidden}
 if err!=nil{return fmt.Errorf("read owner: %w",err)}
 if role!=RoleOwner||enabled!=1{return ErrForbidden}
 return nil
}

// CheckProjectAccess is a fresh database-level authorization evaluator, not
// yet wired to existing Operator endpoints. No non-Owner grants are implied.
func (s *Service) CheckProjectAccess(ctx context.Context,userID,projectID string,cap ProjectCapability)error{
 if strings.TrimSpace(userID)==""||strings.TrimSpace(projectID)==""||!validProjectCapability(cap){return ErrForbidden}
 var role string
 var enabled int
 err:=s.db.QueryRowContext(ctx,"SELECT role, enabled FROM local_users WHERE user_id = ?",userID).Scan(&role,&enabled)
 if errors.Is(err,sql.ErrNoRows){return ErrForbidden}
 if err!=nil{return err}
 if enabled!=1{return ErrForbidden}
 if role==RoleOwner{return nil}
 var found int
 err=s.db.QueryRowContext(ctx,`
 SELECT 1 FROM project_user_memberships m
 JOIN project_user_capabilities c ON c.project_id=m.project_id AND c.user_id=m.user_id
 WHERE m.project_id=? AND m.user_id=? AND c.capability=?`,projectID,userID,string(cap)).Scan(&found)
 if errors.Is(err,sql.ErrNoRows){return ErrForbidden}
 if err!=nil{return err}
 return nil
}
func queryGrantCaps(ctx context.Context,q interface{
 QueryContext(context.Context,string,...any)(*sql.Rows,error)
},projectID,userID string)([]ProjectCapability,error){
 rows,err:=q.QueryContext(ctx,`
 SELECT capability FROM project_user_capabilities
 WHERE project_id=? AND user_id=? ORDER BY capability`,projectID,userID)
 if err!=nil{return nil,err}
 defer rows.Close()
 caps:=[]ProjectCapability{}
 for rows.Next(){
  var c ProjectCapability
  if err:=rows.Scan(&c);err!=nil{return nil,err}
  caps=append(caps,c)
 }
 return caps,rows.Err()
}

func auditProjectChange(ctx context.Context,tx *sql.Tx,projectID,userID,actor,operation string,before,after []ProjectCapability,now int64)error{
 prior,_:=json.Marshal(before)
 next,_:=json.Marshal(after)
 _,err:=tx.ExecContext(ctx,`
 INSERT INTO project_user_access_audit
 (project_id,user_id,actor_user_id,operation,before_capabilities_json,after_capabilities_json,created_at_us)
 VALUES(?,?,?,?,?,?,?)`,projectID,userID,actor,operation,string(prior),string(next),now)
 return err
}
