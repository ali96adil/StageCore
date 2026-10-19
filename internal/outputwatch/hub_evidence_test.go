package outputwatch

import(
 "context"
 "errors"
 "testing"
 "time"
 "github.com/ali96adil/StageCore/internal/devicechannel"
 "github.com/ali96adil/StageCore/internal/deviceexperience"
)

type hubFakeDevices struct{
 device deviceexperience.Device
 assignment deviceexperience.AssignmentRecord
}
func(f *hubFakeDevices)GetDevice(_ context.Context,_ string)(deviceexperience.Device,error){return f.device,nil}
func(f *hubFakeDevices)GetAssignmentRecord(_ context.Context,_ string)(deviceexperience.AssignmentRecord,error){return f.assignment,nil}

type hubFakeRuntime struct{
 scope devicechannel.V2RuntimeScope
 online bool
 report devicechannel.V2SoftwareLevels
 hasReport bool
 activityAt time.Time
 activityGeneration int64
 hasActivity bool
}
func(f *hubFakeRuntime)CurrentV2Scope(string)(devicechannel.V2RuntimeScope,bool){return f.scope,f.online}
func(f *hubFakeRuntime)LatestV2SoftwareLevels(string)(devicechannel.V2SoftwareLevels,bool){return f.report,f.hasReport}
func(f *hubFakeRuntime)CurrentV2TransportActivity(string)(time.Time,int64,bool){return f.activityAt,f.activityGeneration,f.hasActivity}

func TestHubEvidenceDoesNotInventPhysicalSafetyOrHeartbeat(t *testing.T){
 now:=time.Date(2026,10,18,12,0,0,0,time.UTC)
 d:=&hubFakeDevices{
  device:deviceexperience.Device{Enabled:true,ProtocolVersion:deviceexperience.ProtocolVersion2},
  assignment:deviceexperience.AssignmentRecord{DeviceID:"dmx-a",ProjectID:"project-a",State:"ACTIVE",
   Epoch:4,RuntimeSnapshotID:"snap-a"},
 }
 r:=&hubFakeRuntime{}
 h:=CurrentHubEvidence{Devices:d,Runtime:r,Now:func()time.Time{return now}}
 ctx:=context.Background()
 v,err:=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.Condition!=Disconnected||v.SoftwareReportCurrent||v.CommandsEnabled||v.PhysicalOutputVerified{
  t.Fatalf("disconnected view claims readiness: %+v %v",v,err)
 }
 if _,err:=h.Read(ctx,"project-b","dmx-a");!errors.Is(err,ErrDeviceOutOfScope){
  t.Fatalf("foreign Project metadata leaked: %v",err)
 }
 r.scope=devicechannel.V2RuntimeScope{Generation:11,ProjectID:"project-a",
  AssignmentEpoch:4,RuntimeSnapshotID:"snap-a",CommandsEnabled:true}
 r.online=true
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.Condition!=AwaitingFreshReport||!v.SocketCurrent||
  v.CommandsEnabled||v.SoftwareReportCurrent{
  t.Fatalf("socket only claimed applied output state: %+v %v",v,err)
 }
 if v.TransportActivityRecent||v.DeviceHeartbeatVerified {t.Fatalf("socket alone marked heartbeat fresh: %+v",v)}
 r.activityAt=now
 r.activityGeneration=11
 r.hasActivity=true
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||!v.TransportActivityRecent||v.DeviceHeartbeatVerified||v.SoftwareReportCurrent{
  t.Fatalf("recent activity must not verify heartbeat or outputs: %+v %v",v,err)
 }
 r.hasReport=true
 r.report=devicechannel.V2SoftwareLevels{
  DeviceID:"dmx-a",ProjectID:"project-a",AssignmentState:"ACTIVE",
  AssignmentEpoch:4,ConnectionGeneration:11,ObservedAt:now,
  ReportedBlackout:true,ChannelLevels:[]uint8{0,0,0},
 }
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.Condition!=SoftwareReportCurrent||!v.SoftwareZeroReported||
  !v.SoftwareReportCurrent||v.CommandsEnabled||v.PhysicalOutputVerified{
  t.Fatalf("fresh logical zero erroneously treated as physical: %+v %v",v,err)
 }
 r.report.ChannelLevels[2]=50
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.SoftwareZeroReported{t.Fatalf("nonzero software levels called blackout: %+v %v",v,err)}
 r.report.ObservedAt=now.Add(-6*time.Second)
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.SoftwareReportCurrent{t.Fatalf("expired software evidence accepted: %+v %v",v,err)}
 r.report.ObservedAt=now.Add(time.Second)
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.SoftwareReportCurrent{t.Fatalf("future/clock-skew software evidence accepted: %+v %v",v,err)}
 r.report.ObservedAt=now
 r.report.ConnectionGeneration=10
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.SoftwareReportCurrent{t.Fatalf("previous socket report accepted: %+v %v",v,err)}
 r.report.ConnectionGeneration=11
 r.activityGeneration=10
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.TransportActivityRecent||v.SoftwareReportCurrent {
  t.Fatalf("old connection activity accepted: %+v %v",v,err)
 }
 r.activityGeneration=11
 r.activityAt=now.Add(-31*time.Second)
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.Condition!=HeartbeatStale||v.TransportActivityRecent||v.SoftwareReportCurrent{
  t.Fatalf("stale socket activity accepted: %+v %v",v,err)
 }
 r.activityAt=now.Add(time.Second)
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.Condition!=ClockUncertain||v.TransportActivityRecent{
  t.Fatalf("future socket activity trusted: %+v %v",v,err)
 }
 r.activityAt=now
 r.scope.ProjectID="project-b"
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.SocketCurrent||v.SoftwareReportCurrent||v.CommandsEnabled{
  t.Fatalf("foreign socket authority leaked: %+v %v",v,err)
 }
 r.scope.ProjectID="project-a"
 r.scope.RuntimeSnapshotID="other-snap"
 v,err=h.Read(ctx,"project-a","dmx-a")
 if err!=nil||v.SocketCurrent||v.SoftwareReportCurrent{
  t.Fatalf("foreign snapshot scope accepted: %+v %v",v,err)
 }
 d.assignment.State="BLOCKED"
 if _,err=h.Read(ctx,"project-a","dmx-a");!errors.Is(err,ErrDeviceOutOfScope){
  t.Fatalf("non-ACTIVE device allowed in Project observation: %v",err)
 }
}
