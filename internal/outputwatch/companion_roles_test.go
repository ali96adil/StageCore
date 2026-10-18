package outputwatch

import(
 "context"
 "errors"
 "testing"
 "time"
 "github.com/ali96adil/StageCore/internal/domain"
)

type fakeCompanionRoleStore struct {
 roles []domain.MachineRole
 assigns map[string]domain.RoleAssignment
 companions map[string]domain.Companion
}
func(f *fakeCompanionRoleStore)ListMachineRoles(_ context.Context,_ string)([]domain.MachineRole,error){return f.roles,nil}
func(f *fakeCompanionRoleStore)GetActiveRoleAssignment(_ context.Context,id string)(domain.RoleAssignment,error){
 v,ok:=f.assigns[id];if !ok{return domain.RoleAssignment{},domain.ErrNotFound};return v,nil
}
func(f *fakeCompanionRoleStore)GetCompanion(_ context.Context,id string)(domain.Companion,error){
 v,ok:=f.companions[id];if !ok{return domain.Companion{},domain.ErrNotFound};return v,nil
}
type fakeCompanionSockets map[string]bool
func(f fakeCompanionSockets)IsConnected(id string)bool{return f[id]}

func TestCompanionProjectEvidenceSeparatesTransportHeartbeatReadinessAndMIDIOSC(t *testing.T){
 now:=time.Date(2026,10,19,2,0,0,0,time.UTC)
 snapshot:="snapshot-a"
 role:=domain.MachineRole{ID:"r-audio",ProjectID:"project-a",RoleKey:"AUDIO-ABLETON",
  DisplayName:"Ableton Mac",Required:true,RequiredRuntimeSnapshotID:&snapshot}
 video:=domain.MachineRole{ID:"r-video",ProjectID:"project-a",RoleKey:"VIDEO-VDMX",
  DisplayName:"VDMX Mac",Required:true}
 store:=&fakeCompanionRoleStore{
  roles:[]domain.MachineRole{role,video},
  assigns:map[string]domain.RoleAssignment{
   role.ID:{MachineRoleID:role.ID,CompanionID:"companion-a",State:domain.RoleReady},
   video.ID:{MachineRoleID:video.ID,CompanionID:"companion-b",State:domain.RoleReady},
  },
  companions:map[string]domain.Companion{
   "companion-a":{ID:"companion-a",TrustState:domain.CompanionTrusted,Readiness:domain.CompanionReadinessReady,
    LastSeenAt:now,AppliedRuntimeSnapshotID:&snapshot},
   "companion-b":{ID:"companion-b",TrustState:domain.CompanionTrusted,Readiness:domain.CompanionReadinessReady,
    LastSeenAt:now},
  },
 }
 sockets:=fakeCompanionSockets{"companion-a":true,"companion-b":true}
 reader:=CompanionRoleEvidence{Roles:store,Transport:sockets,HeartbeatTimeout:10*time.Second,Now:func()time.Time{return now}}
 get:=func()CompanionProjectView{
  t.Helper()
  v,err:=reader.ReadProject(context.Background(),"project-a")
  if err!=nil{t.Fatal(err)}
  if !v.ReadOnly||v.PhysicalStatus!="UNKNOWN"||v.AutomatedReplay||len(v.Roles)!=2{
   t.Fatalf("bad role envelope %+v",v)
  }
  for _,role:=range v.Roles{
   if role.MIDIExecutionConfirmed||role.OSCDeliveryConfirmed||role.PhysicalOutputVerified{
    t.Fatalf("transport turned into claimed output execution %+v",role)
   }
  }
  return v
 }
 v:=get()
 if v.Roles[0].Condition!="COMPANION_READY"||!v.Roles[0].ReportFresh||
  !v.Roles[0].ScopeVerified||v.Roles[0].OutputClass!="AUDIO"{
  t.Fatalf("AUDIO view incorrect %+v",v.Roles[0])
 }
 if v.Roles[1].Condition!="COMPANION_READY"||v.Roles[1].OutputClass!="VIDEO"{
  t.Fatalf("VIDEO view incorrect %+v",v.Roles[1])
 }
 sockets["companion-a"]=false
 v=get()
 if v.Roles[0].Condition!="OFFLINE"||v.Roles[0].ReportFresh{
  t.Fatalf("last READY survived missing socket %+v",v.Roles[0])
 }
 sockets["companion-a"]=true
 c:=store.companions["companion-a"]
 c.LastSeenAt=now.Add(-11*time.Second)
 store.companions["companion-a"]=c
 if v=get().Roles[0];v.Condition!="STALE"||v.ScopeVerified{
  t.Fatalf("stale Companion accepted %+v",v)
 }
 c.LastSeenAt=now.Add(time.Second)
 store.companions["companion-a"]=c
 if v=get().Roles[0];v.Condition!="CLOCK_UNCERTAIN"{
  t.Fatalf("future timestamp accepted %+v",v)
 }
 c.LastSeenAt=now
 other:="snapshot-b"
 c.AppliedRuntimeSnapshotID=&other
 store.companions["companion-a"]=c
 if v=get().Roles[0];v.Condition!="MISMATCH"||v.ScopeVerified{
  t.Fatalf("wrong Snapshot accepted %+v",v)
 }
 c.AppliedRuntimeSnapshotID=&snapshot
 c.Readiness=domain.CompanionReadinessDegraded
 store.companions["companion-a"]=c
 if v=get().Roles[0];v.Condition!="DEGRADED"||v.ScopeVerified{
  t.Fatalf("degraded Companion counted ready %+v",v)
 }
 c.Readiness=domain.CompanionReadinessReady
 c.TrustState=domain.CompanionRevoked
 store.companions["companion-a"]=c
 if v=get().Roles[0];v.Condition!="UNTRUSTED"||v.TransportConnected{
  t.Fatalf("revoked Companion retains trusted state %+v",v)
 }
}
func TestCompanionRoleInventoryExcludesForeignProjectsAndTruncation(t *testing.T){
 now:=time.Now()
 roles:=&fakeCompanionRoleStore{
  roles:[]domain.MachineRole{{ID:"foreign",ProjectID:"project-b"}},
 }
 reader:=CompanionRoleEvidence{Roles:roles,Transport:fakeCompanionSockets{},
  HeartbeatTimeout:5*time.Second,Now:func()time.Time{return now}}
 if _,err:=reader.ReadProject(context.Background(),"project-a");!errors.Is(err,ErrCompanionRolesOutOfScope){
  t.Fatalf("foreign Role returned %+v",err)
 }
 roles.roles=make([]domain.MachineRole,maxCompanionRoleWatch+1)
 if _,err:=reader.ReadProject(context.Background(),"project-a");!errors.Is(err,ErrCompanionRolesOutOfScope){
  t.Fatalf("truncated inventory silently allowed: %v",err)
 }
 roles.roles=[]domain.MachineRole{{ID:"r1",ProjectID:"project-a",RoleKey:"OTHER"}}
 v,err:=reader.ReadProject(context.Background(),"project-a")
 if err!=nil||len(v.Roles)!=1||v.Roles[0].Condition!="UNASSIGNED"{
  t.Fatalf("unassigned Role must report UNKNOWN: %+v %v",v,err)
 }
}
