package livereconcile

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
)

type diagnosticDeviceStore struct {
	device deviceexperience.Device
	assignment deviceexperience.AssignmentRecord
	deviceReads int
	assignmentReads int
	onDevice func(*diagnosticDeviceStore)
	onAssignment func(*diagnosticDeviceStore)
}
func (s *diagnosticDeviceStore) GetDevice(_ context.Context, _ string) (deviceexperience.Device,error) {
	s.deviceReads++
	if s.onDevice != nil { s.onDevice(s) }
	return s.device,nil
}
func (s *diagnosticDeviceStore) GetAssignmentRecord(_ context.Context, _ string) (deviceexperience.AssignmentRecord,error) {
	s.assignmentReads++
	if s.onAssignment != nil { s.onAssignment(s) }
	return s.assignment,nil
}

type diagnosticReporter struct {
	report devicechannel.V2SoftwareLevels
	generation int64
	reportReads int
	generationReads int
	onReport func(*diagnosticReporter)
	onGeneration func(*diagnosticReporter)
}
func (s *diagnosticReporter) LatestV2SoftwareLevels(_ string)(devicechannel.V2SoftwareLevels,bool){
	s.reportReads++
	if s.onReport != nil { s.onReport(s) }
	out:=s.report
	if out.ConnectionGeneration==0 {return out,false}
	out.ChannelLevels=append([]uint8(nil),out.ChannelLevels...)
	return out,true
}
func (s *diagnosticReporter) CurrentV2Generation(_ string)(int64,bool){
	s.generationReads++
	if s.onGeneration!=nil {s.onGeneration(s)}
	return s.generation,s.generation>0
}

func diagnosticReaderFixture(t *testing.T)(*BlockedDiagnosticReader,*memorySessionStore,*diagnosticDeviceStore,*diagnosticReporter){
	t.Helper()
	session:=serviceFixture(t)
	desired, report, now:=blockedDiagnosticFixture(t)
	report.ObservedAt=now.Add(-time.Second)
	_ = desired
	devices:=&diagnosticDeviceStore{
		device:deviceexperience.Device{
			ID:testLightingDevice, ProfileID:lightingnode.ProfileID,
			ProtocolVersion:deviceexperience.ProtocolVersion2,
			Enabled:true,Capabilities:[]string{},
		},
		assignment:deviceexperience.AssignmentRecord{
			DeviceID:testLightingDevice,ProjectID:testProject,Epoch:7,State:"BLOCKED",
		},
	}
	devices.device.Capabilities=[]string{devicechannel.V2LightingStateProbeCapability}
	reporter:=&diagnosticReporter{report:report,generation:21}
	reader:=NewBlockedDiagnosticReader(session,devices,reporter)
	reader.now=func()time.Time{return now}
	return reader,session,devices,reporter
}

func TestBlockedDiagnosticReaderCurrentCueAndScopedSoftwareOnly(t *testing.T) {
	reader,_,devices,reporter:=diagnosticReaderFixture(t)
	got:=reader.Read(context.Background(),testProject,testLightingDevice)
	if got.Status!=SoftwareDiagnosticBlocked || got.CommandsEnabled || got.PhysicalVerified ||
		!reflect.DeepEqual(got.DifferingSlots,[]int{1,2,3}) ||
		got.CueID!="cue-5" || got.CueExecutionID!="execution-5" ||
		devices.deviceReads<2 || devices.assignmentReads<2 ||
		reporter.reportReads<2 || reporter.generationReads<2 {
		t.Fatalf("read-only scoped diagnostic=%+v deviceReads=%d assignmentReads=%d reportReads=%d generationReads=%d",
			got,devices.deviceReads,devices.assignmentReads,reporter.reportReads,reporter.generationReads)
	}
	// A fresh logical report with channel 1 and 3 matching must surface
	// exactly slot 2; because the node is still BLOCKED, this is UNSAFE.
	reporter.report.ChannelLevels[0]=180
	reporter.report.ChannelLevels[2]=60
	reporter.report.ReportedBlackout=false
	reporter.report.UnsafeWhileUnactivated=true
	got=reader.Read(context.Background(),testProject,testLightingDevice)
	if got.Status!=SoftwareDiagnosticUnsafe || got.CommandsEnabled || got.PhysicalVerified ||
		!reflect.DeepEqual(got.DifferingSlots,[]int{2}) {
		t.Fatalf("fresh same-Cue-5 slot-2 diagnostic=%+v",got)
	}
	// Another completed Cue changes targets without consulting last GO ACK.
	cue:="cue-7"
	reader.desired.sessions.(*memorySessionStore).session.CurrentCueID=&cue
	reader.desired.sessions.(*memorySessionStore).session.LastCompletedCueID=&cue
	store:=reader.desired.sessions.(*memorySessionStore)
	finished:=store.executions[0].StartedAt.Add(2*time.Second)
	store.executions=append(store.executions,store.executions[0])
	store.executions[1].ID="execution-7"
	store.executions[1].CueID="cue-7"
	store.executions[1].StartedAt=finished
	store.executions[1].CompletedAt=&finished
	got=reader.Read(context.Background(),testProject,testLightingDevice)
	if got.Status!=SoftwareDiagnosticUnsafe || got.CueID!="cue-7" ||
		!reflect.DeepEqual(got.DesiredSlots,map[int]uint8{1:0,2:51,3:153}) {
		t.Fatalf("missed Cue 5 to 7 was not reflected: %+v",got)
	}
}

func TestBlockedDiagnosticReaderFailsClosedOnMidReadScopeChanges(t *testing.T) {
	for _,tc:=range []struct{
		name string
		mutate func(*memorySessionStore,*diagnosticDeviceStore,*diagnosticReporter)
	}{
		{"new socket",func(_ *memorySessionStore,_ *diagnosticDeviceStore,r *diagnosticReporter){
			r.onGeneration=func(r *diagnosticReporter){if r.generationReads==2{r.generation++}}
		}},
		{"replaced device report",func(_ *memorySessionStore,_ *diagnosticDeviceStore,r *diagnosticReporter){
			r.onReport=func(r *diagnosticReporter){if r.reportReads==2 {r.report.ChannelLevels[1]=50;r.report.ObservedAt=r.report.ObservedAt.Add(time.Millisecond)}}
		}},
		{"assignment transfer",func(_ *memorySessionStore,d *diagnosticDeviceStore,_ *diagnosticReporter){
			d.onAssignment=func(d *diagnosticDeviceStore){if d.assignmentReads==2 {d.assignment.Epoch++;d.assignment.ProjectID="other"}}
		}},
		{"device revoked",func(_ *memorySessionStore,d *diagnosticDeviceStore,_ *diagnosticReporter){
			d.onDevice=func(d *diagnosticDeviceStore){if d.deviceReads==2 {d.device.Enabled=false}}
		}},
		{"same Cue new failed attempt",func(s *memorySessionStore,_ *diagnosticDeviceStore,_ *diagnosticReporter){
			s.afterExecutionRead=func(s *memorySessionStore){if s.listCalls==4 {
				started:=s.executions[0].StartedAt.Add(10*time.Second)
				s.executions=append(s.executions,domain.CueExecution{
					ID:"repeated-failed",SessionID:s.session.ID,CueID:"cue-5",
					StartedAt:started,Result:domain.ExecutionFailed,
				})
			}}
		}},
		{"Cue advance",func(s *memorySessionStore,_ *diagnosticDeviceStore,_ *diagnosticReporter){
			s.afterRead=func(current *domain.Session){if s.getCalls==2 {cue:="cue-7";current.CurrentCueID=&cue}}
		}},
	}{
		t.Run(tc.name,func(t *testing.T){
			reader,sessions,devices,reporter:=diagnosticReaderFixture(t)
			tc.mutate(sessions,devices,reporter)
			got:=reader.Read(context.Background(),testProject,testLightingDevice)
			if got.Status!=SoftwareDiagnosticUnknown || got.CommandsEnabled || got.PhysicalVerified ||
				len(got.DifferingSlots)!=0 || len(got.DesiredSlots)!=0 {
				t.Fatalf("mid-read change not fenced: %+v",got)
			}
		})
	}
}

func TestBlockedDiagnosticReaderFailsClosedOnMissingUntrustedOrExpiredInput(t *testing.T) {
	for _,tc:=range []struct{
		name string
		mutate func(*BlockedDiagnosticReader,*memorySessionStore,*diagnosticDeviceStore,*diagnosticReporter)
	}{
		{"no report",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,_ *diagnosticDeviceStore,r *diagnosticReporter){r.report.ConnectionGeneration=0}},
		{"stale age",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,_ *diagnosticDeviceStore,r *diagnosticReporter){r.report.ObservedAt=r.report.ObservedAt.Add(-10*time.Second)}},
		{"missing channel",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,_ *diagnosticDeviceStore,r *diagnosticReporter){r.report.ChannelLevels=r.report.ChannelLevels[:11]}},
		{"different device report",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,_ *diagnosticDeviceStore,r *diagnosticReporter){r.report.DeviceID="other"}},
		{"unassigned",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,d *diagnosticDeviceStore,_ *diagnosticReporter){d.assignment.State="UNASSIGNED";d.assignment.ProjectID=""}},
		{"v2 ACTIVE without physical proof",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,d *diagnosticDeviceStore,_ *diagnosticReporter){d.assignment.State="ACTIVE"}},
		{"unexpected snapshot activation",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,d *diagnosticDeviceStore,_ *diagnosticReporter){d.assignment.RuntimeSnapshotID="snapshot-1"}},
		{"not negotiated",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,d *diagnosticDeviceStore,_ *diagnosticReporter){d.device.Capabilities=nil}},
		{"wrong profile",func(_ *BlockedDiagnosticReader,_ *memorySessionStore,d *diagnosticDeviceStore,_ *diagnosticReporter){d.device.ProfileID="another"}},
		{"SIMULATION",func(_ *BlockedDiagnosticReader,s *memorySessionStore,_ *diagnosticDeviceStore,_ *diagnosticReporter){s.session.Type=domain.SessionSimulation}},
		{"manual confirmation",func(_ *BlockedDiagnosticReader,s *memorySessionStore,_ *diagnosticDeviceStore,_ *diagnosticReporter){s.session.StateTruth.ManualConfirmationRequired=true}},
	}{
		t.Run(tc.name,func(t *testing.T){
			reader,sessions,devices,reporter:=diagnosticReaderFixture(t)
			tc.mutate(reader,sessions,devices,reporter)
			got:=reader.Read(context.Background(),testProject,testLightingDevice)
			if got.Status!=SoftwareDiagnosticUnknown || got.CommandsEnabled || got.PhysicalVerified ||
				len(got.DifferingSlots)!=0 {
				t.Fatalf("unsafe diagnostic input accepted: %+v",got)
			}
		})
	}
}
