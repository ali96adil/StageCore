package userauth

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"
)

func ownerInTx(ctx context.Context,tx *sql.Tx,actor string)error{
 var role string
 var enabled int
 err:=tx.QueryRowContext(ctx,`SELECT role,enabled FROM local_users WHERE user_id=?`,actor).Scan(&role,&enabled)
 if errors.Is(err,sql.ErrNoRows){return ErrForbidden}
 if err!=nil{return err}
 if role!=RoleOwner||enabled!=1{return ErrForbidden}
 return nil
}

// ReplaceProjectGrant atomically replaces one non-Owner user's complete grant
// for exactly one project and logs the change. OWNER override is immutable.
func (s *Service) ReplaceProjectGrant(ctx context.Context,actor,projectID,userID string,requested []ProjectCapability)(ProjectGrant,error){
 if strings.TrimSpace(actor)==""||strings.TrimSpace(projectID)==""||strings.TrimSpace(userID)==""{
  return ProjectGrant{},ErrProjectGrantInvalid
 }
 tx,err:=s.db.BeginTx(ctx,nil)
 if err!=nil{return ProjectGrant{},err}
 defer tx.Rollback()
 if err:=ownerInTx(ctx,tx,actor);err!=nil{return ProjectGrant{},err}
 var role string
 var enabled int
 err=tx.QueryRowContext(ctx,`SELECT role,enabled FROM local_users WHERE user_id=?`,userID).Scan(&role,&enabled)
 if errors.Is(err,sql.ErrNoRows){return ProjectGrant{},ErrProjectGrantMissing}
 if err!=nil{return ProjectGrant{},err}
 if role==RoleOwner||enabled!=1||!validRole(role){return ProjectGrant{},ErrProjectGrantInvalid}
 caps,err:=checkGrantCaps(requested,role)
 if err!=nil{return ProjectGrant{},err}
 var exists int
 err=tx.QueryRowContext(ctx,`SELECT 1 FROM projects WHERE project_id=?`,projectID).Scan(&exists)
 if errors.Is(err,sql.ErrNoRows){return ProjectGrant{},ErrProjectGrantMissing}
 if err!=nil{return ProjectGrant{},err}
 before,err:=queryGrantCaps(ctx,tx,projectID,userID)
 if err!=nil{return ProjectGrant{},err}
 now:=s.now().UTC().UnixMicro()
 _,err=tx.ExecContext(ctx,`
 INSERT INTO project_user_memberships
 (project_id,user_id,granted_by_user_id,created_at_us,updated_at_us)
 VALUES(?,?,?,?,?)
 ON CONFLICT(project_id,user_id) DO UPDATE SET
 granted_by_user_id=excluded.granted_by_user_id,
 updated_at_us=excluded.updated_at_us`,
 projectID,userID,actor,now,now)
 if err!=nil{return ProjectGrant{},fmt.Errorf("membership: %w",err)}
 if _,err=tx.ExecContext(ctx,`DELETE FROM project_user_capabilities WHERE project_id=? AND user_id=?`,projectID,userID);err!=nil{return ProjectGrant{},err}
 for _,cap:=range caps{
  if _,err=tx.ExecContext(ctx,`
   INSERT INTO project_user_capabilities(project_id,user_id,capability)
   VALUES(?,?,?)`,projectID,userID,string(cap));err!=nil{return ProjectGrant{},err}
 }
 if err:=auditProjectChange(ctx,tx,projectID,userID,actor,"GRANT_REPLACE",before,caps,now);err!=nil{return ProjectGrant{},err}
 if err:=tx.Commit();err!=nil{return ProjectGrant{},err}
 return ProjectGrant{ProjectID:projectID,UserID:userID,GrantedByUserID:actor,Capabilities:caps},nil
}

// RevokeProjectGrant removes membership AND capabilities in a transaction.
// Future routed CheckProjectAccess calls will deny immediately.
func (s *Service) RevokeProjectGrant(ctx context.Context,actor,projectID,userID string)error{
 if strings.TrimSpace(actor)==""||strings.TrimSpace(projectID)==""||strings.TrimSpace(userID)==""{return ErrProjectGrantInvalid}
 tx,err:=s.db.BeginTx(ctx,nil)
 if err!=nil{return err}
 defer tx.Rollback()
 if err:=ownerInTx(ctx,tx,actor);err!=nil{return err}
 before,err:=queryGrantCaps(ctx,tx,projectID,userID)
 if err!=nil{return err}
 res,err:=tx.ExecContext(ctx,`DELETE FROM project_user_memberships WHERE project_id=? AND user_id=?`,projectID,userID)
 if err!=nil{return err}
 count,err:=res.RowsAffected()
 if err!=nil{return err}
 if count!=1{return ErrProjectGrantMissing}
 if err:=auditProjectChange(ctx,tx,projectID,userID,actor,"REVOKE",before,[]ProjectCapability{},s.now().UTC().UnixMicro());err!=nil{return err}
 return tx.Commit()
}

func (s *Service) ListProjectGrants(ctx context.Context,actor,projectID string)([]ProjectGrant,error){
 if strings.TrimSpace(projectID)==""{return nil,ErrProjectGrantInvalid}
 if err:=s.ownerCurrent(ctx,actor);err!=nil{return nil,err}
 var exists int
 if err:=s.db.QueryRowContext(ctx,`SELECT 1 FROM projects WHERE project_id=?`,projectID).Scan(&exists);err!=nil{
  if errors.Is(err,sql.ErrNoRows){return nil,ErrProjectGrantMissing}
  return nil,err
 }
 rows,err:=s.db.QueryContext(ctx,`
 SELECT user_id,granted_by_user_id FROM project_user_memberships
 WHERE project_id=? ORDER BY user_id`,projectID)
 if err!=nil{return nil,err}
 list:=[]ProjectGrant{}
 for rows.Next(){
  grant:=ProjectGrant{ProjectID:projectID}
  if err:=rows.Scan(&grant.UserID,&grant.GrantedByUserID);err!=nil{rows.Close();return nil,err}
  list=append(list,grant)
 }
 err=rows.Err()
 rows.Close()
 if err!=nil{return nil,err}
 for i:=range list{
  list[i].Capabilities,err=queryGrantCaps(ctx,s.db,projectID,list[i].UserID)
  if err!=nil{return nil,err}
 }
 return list,nil
}
