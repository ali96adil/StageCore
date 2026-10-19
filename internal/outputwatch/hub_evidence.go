package outputwatch

import (
 "context"
 "errors"
 "time"
 "github.com/ali96adil/StageCore/internal/devicechannel"
 "github.com/ali96adil/StageCore/internal/deviceexperience"
)

var ErrDeviceOutOfScope=errors.New("outputwatch: device not in selected Project")

// CurrentHubEvidence reads already-authenticated V2 socket metadata.
// It does not send a probe, command, or infer a physical output state.
type CurrentHubEvidence struct {
 Devices interface {
  GetDevice(context.Context,string)(deviceexperience.Device,error)
  GetAssignmentRecord(context.Context,string)(deviceexperience.AssignmentRecord,error)
 }
 Runtime interface {
  CurrentV2Scope(string)(devicechannel.V2RuntimeScope,bool)
  CurrentV2TransportActivity(string)(time.Time,int64,bool)
  LatestV2SoftwareLevels(string)(devicechannel.V2SoftwareLevels,bool)
 }
 Now func()time.Time
}
type HubDeviceView struct {
 SchemaVersion int `json:"schema_version"`
 DeviceID string `json:"device_id"`
 ProjectID string `json:"project_id"`
 Condition Condition `json:"condition"`
 Reason string `json:"reason"`
 SocketCurrent bool `json:"authenticated_socket_current"`
 TransportActivityRecent bool `json:"transport_activity_recent"`
 TransportActivityAgeSeconds int64 `json:"transport_activity_age_seconds,omitempty"`
 DeviceHeartbeatVerified bool `json:"device_heartbeat_verified"`
 SoftwareReportCurrent bool `json:"software_report_current"`
 SoftwareZeroReported bool `json:"software_zero_reported"`
 PhysicalOutputVerified bool `json:"physical_output_verified"`
 CommandsEnabled bool `json:"commands_enabled"`
 OperatorReviewRequired bool `json:"operator_review_required"`
}
func (h CurrentHubEvidence) Read(ctx context.Context,projectID,deviceID string)(HubDeviceView,error){
 if h.Devices==nil||h.Runtime==nil||projectID==""||deviceID=="" {return HubDeviceView{},ErrDeviceOutOfScope}
 a,err:=h.Devices.GetAssignmentRecord(ctx,deviceID)
 if err!=nil||a.ProjectID!=projectID||a.State!="ACTIVE"||a.Epoch<=0||a.RuntimeSnapshotID==""{
  return HubDeviceView{},ErrDeviceOutOfScope
 }
 d,err:=h.Devices.GetDevice(ctx,deviceID)
 if err!=nil||!d.Enabled||d.ProtocolVersion!=deviceexperience.ProtocolVersion2{
  return HubDeviceView{},ErrDeviceOutOfScope
 }
 v:=HubDeviceView{SchemaVersion:1,DeviceID:deviceID,ProjectID:projectID,Condition:Disconnected,
  Reason:"No current authenticated socket; physical state unknown",OperatorReviewRequired:true}
 scope,connected:=h.Runtime.CurrentV2Scope(deviceID)
 if !connected{return v,nil}
 v.Condition=AwaitingFreshReport
 v.Reason="Current socket is not yet matched to Project and Snapshot"
 if scope.ProjectID!=projectID||scope.AssignmentEpoch!=a.Epoch||scope.RuntimeSnapshotID!=a.RuntimeSnapshotID{
  return v,nil
 }
 v.SocketCurrent=true
 v.Reason="Current socket matched; no verified software report"
 now:=time.Now().UTC()
 if h.Now!=nil {now=h.Now().UTC()}
 activityAt,generation,hasActivity:=h.Runtime.CurrentV2TransportActivity(deviceID)
 if !hasActivity||generation!=scope.Generation{
  v.Reason="No same-generation transport activity; device heartbeat UNKNOWN"
  return v,nil
 }
 activityAge:=now.Sub(activityAt)
 if activityAge<0{
  v.Condition=ClockUncertain
  v.Reason="Socket activity timestamp is in the future; clock uncertain"
  return v,nil
 }
 if activityAge>30*time.Second{
  v.Condition=HeartbeatStale
  v.Reason="Current socket transport activity stale; device heartbeat UNKNOWN"
  return v,nil
 }
 v.TransportActivityRecent=true
 v.TransportActivityAgeSeconds=int64(activityAge/time.Second)
 v.Reason="Recent authenticated-socket transport activity; device heartbeat and outputs UNKNOWN"
 report,ok:=h.Runtime.LatestV2SoftwareLevels(deviceID)
 if !ok||report.ProjectID!=projectID||report.AssignmentEpoch!=a.Epoch||
  report.ConnectionGeneration!=scope.Generation||report.AssignmentState!="ACTIVE"||
  report.PhysicalOutputVerified||report.CommandsEnabled||len(report.ChannelLevels)==0{return v,nil}
 age:=now.Sub(report.ObservedAt)
 if report.ObservedAt.IsZero()||age<0||age>5*time.Second{return v,nil}
 v.Condition=SoftwareReportCurrent
 v.SoftwareReportCurrent=true
 v.SoftwareZeroReported=report.ReportedBlackout
 for _,level:=range report.ChannelLevels{if level!=0{v.SoftwareZeroReported=false}}
 v.Reason="Current software-only report; physical state unknown"
 return v,nil
}
