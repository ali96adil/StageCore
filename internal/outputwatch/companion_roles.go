package outputwatch

import (
 "context"
 "errors"
 "strings"
 "time"

 "github.com/ali96adil/StageCore/internal/companion"
 "github.com/ali96adil/StageCore/internal/domain"
)

var ErrCompanionRolesOutOfScope=errors.New("outputwatch: Companion Project Role inventory unavailable")

const maxCompanionRoleWatch=64

type CompanionRoleRepository interface {
 ListMachineRoles(context.Context,string)([]domain.MachineRole,error)
 GetActiveRoleAssignment(context.Context,string)(domain.RoleAssignment,error)
 GetCompanion(context.Context,string)(domain.Companion,error)
}
type CompanionPresence interface {IsConnected(string)bool}

type CompanionRoleEvidence struct {
 Roles CompanionRoleRepository
 Transport CompanionPresence
 Now func()time.Time
 HeartbeatTimeout time.Duration
}
type CompanionRoleView struct {
 MachineRoleID string `json:"machine_role_id"`
 RoleKey string `json:"role_key"`
 DisplayName string `json:"display_name"`
 OutputClass string `json:"output_class"`
 Required bool `json:"required"`
 Condition string `json:"condition"`
 TransportConnected bool `json:"authenticated_transport_connected"`
 ReportFresh bool `json:"companion_report_fresh"`
 ScopeVerified bool `json:"machine_role_scope_verified"`
 MIDIExecutionConfirmed bool `json:"midi_execution_confirmed"`
 OSCDeliveryConfirmed bool `json:"osc_delivery_confirmed"`
 PhysicalOutputVerified bool `json:"physical_output_verified"`
 Reason string `json:"reason"`
}
type CompanionProjectView struct {
 SchemaVersion int `json:"schema_version"`
 ProjectID string `json:"project_id"`
 Roles []CompanionRoleView `json:"roles"`
 ReadOnly bool `json:"read_only"`
 PhysicalStatus string `json:"physical_status"`
 AutomatedReplay bool `json:"automated_replay"`
}
func classifyCompanionRole(key string)string{
 switch {
 case strings.HasPrefix(strings.ToUpper(strings.TrimSpace(key)),"AUDIO-"):
  return "AUDIO"
 case strings.HasPrefix(strings.ToUpper(strings.TrimSpace(key)),"VIDEO-"):
  return "VIDEO"
 default:
  return "OTHER"
 }
}
// ReadProject queries only role IDs belonging to the requested Project.
// It never calls ListCompanions (which would expose unrelated Machines),
// sends OSC/MIDI, reports execution success, or queues a retry.
func (e CompanionRoleEvidence) ReadProject(ctx context.Context,projectID string)(CompanionProjectView,error){
 if e.Roles==nil || e.Transport==nil || strings.TrimSpace(projectID)==""||
  e.HeartbeatTimeout<=0 || e.HeartbeatTimeout>10*time.Minute {
  return CompanionProjectView{},ErrCompanionRolesOutOfScope
 }
 roles,err:=e.Roles.ListMachineRoles(ctx,projectID)
 if err!=nil{return CompanionProjectView{},err}
 if len(roles)>maxCompanionRoleWatch{return CompanionProjectView{},ErrCompanionRolesOutOfScope}
 now:=time.Now().UTC()
 if e.Now!=nil{now=e.Now().UTC()}
 out:=CompanionProjectView{SchemaVersion:1,ProjectID:projectID,
  Roles:make([]CompanionRoleView,0,len(roles)),ReadOnly:true,
  PhysicalStatus:"UNKNOWN",AutomatedReplay:false}
 for _,role:=range roles{
  if err:=ctx.Err();err!=nil{return CompanionProjectView{},err}
  if role.ProjectID!=projectID{return CompanionProjectView{},ErrCompanionRolesOutOfScope}
  if role.Retired{continue}
  v:=CompanionRoleView{MachineRoleID:role.ID,RoleKey:role.RoleKey,
   DisplayName:role.DisplayName,OutputClass:classifyCompanionRole(role.RoleKey),
   Required:role.Required,Condition:"UNASSIGNED",
   Reason:"No active Project Role assignment; output UNKNOWN"}
  assignment,err:=e.Roles.GetActiveRoleAssignment(ctx,role.ID)
  if errors.Is(err,domain.ErrNotFound){
   out.Roles=append(out.Roles,v);continue
  }
  if err!=nil{return CompanionProjectView{},err}
  if assignment.MachineRoleID!=role.ID ||
   assignment.CompanionID==""||assignment.State==domain.RoleReleased{
   return CompanionProjectView{},ErrCompanionRolesOutOfScope
  }
  v.Condition="UNKNOWN"
  v.Reason="Assigned Companion state unverified; output UNKNOWN"
  c,err:=e.Roles.GetCompanion(ctx,assignment.CompanionID)
  if errors.Is(err,domain.ErrNotFound){
   out.Roles=append(out.Roles,v);continue
  }
  if err!=nil{return CompanionProjectView{},err}
  if c.ID!=assignment.CompanionID||c.TrustState!=domain.CompanionTrusted{
   v.Condition="UNTRUSTED"
   v.Reason="Companion identity is not trusted"
   out.Roles=append(out.Roles,v);continue
  }
  v.TransportConnected=e.Transport.IsConnected(c.ID)
  if !v.TransportConnected{
   v.Condition="OFFLINE"
   v.Reason="Authenticated Companion channel is disconnected"
   out.Roles=append(out.Roles,v);continue
  }
  age:=now.Sub(c.LastSeenAt)
  if c.LastSeenAt.IsZero()||age<0{
   v.Condition="CLOCK_UNCERTAIN"
   v.Reason="Companion report timestamp is missing or in the future"
   out.Roles=append(out.Roles,v);continue
  }
  if age>e.HeartbeatTimeout{
   v.Condition="STALE"
   v.Reason="Companion last report exceeds configured heartbeat threshold"
   out.Roles=append(out.Roles,v);continue
  }
  v.ReportFresh=true
  evaluation:=companion.EvaluateRole(role,c,now,e.HeartbeatTimeout)
  if role.RequiredRuntimeSnapshotID!=nil && (c.AppliedRuntimeSnapshotID==nil||
   *role.RequiredRuntimeSnapshotID!=*c.AppliedRuntimeSnapshotID) {
   v.Condition="MISMATCH"
   v.Reason="Required published Runtime Snapshot not applied"
  } else if evaluation.Readiness!=domain.CompanionReadinessReady ||
   assignment.State!=domain.RoleReady {
   v.Condition="DEGRADED"
   v.Reason="Companion or assigned role is not READY"
  }else {
   v.ScopeVerified=true
   v.Condition="COMPANION_READY"
   v.Reason="Authenticated Companion reports READY for role; MIDI/OSC destination execution UNKNOWN"
  }
  out.Roles=append(out.Roles,v)
 }
 return out,nil
}
