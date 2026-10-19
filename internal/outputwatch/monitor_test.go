package outputwatch

import (
    "errors"
    "sync"
    "testing"
    "time"
)

func fixture(t *testing.T) (*Monitor,*time.Time,Scope) {
    t.Helper()
    now:=time.Date(2026,10,18,13,0,0,0,time.UTC)
    m,err:=New(5*time.Second,func() time.Time{return now})
    if err!=nil{t.Fatal(err)}
    scope:=Scope{DeviceID:"lighting-a",ProjectID:"show-a",
        RuntimeSnapshotID:"snapshot-a",AssignmentEpoch:3,ConnectionGeneration:41}
    return m,&now,scope
}

func TestWatchdogObservationsCannotActivateOutputsOrClaimPhysicalBlackout(t *testing.T){
    m,clock,scope:=fixture(t)
    if err:=m.AttachAuthenticated(scope);err!=nil{t.Fatal(err)}
    if m.Report(scope,1,true,true,"",false){
        t.Fatal("device report without fresh heartbeat accepted")
    }
    if err:=m.TrackSent(scope,"cue-1",clock.Add(time.Second));err==nil{
        t.Fatal("tracking command before live observation succeeded")
    }
    if !m.Heartbeat(scope,1)||!m.Report(scope,2,true,true,"",false){
        t.Fatal("current heartbeat/software report rejected")
    }
    view:=m.Snapshot()
    if view.Condition!=SoftwareReportCurrent||!view.SoftwareZeroReported||
        !view.SoftwareLevelsKnown||view.PhysicalOutputVerified||
        view.CommandsEnabled||!view.OperatorReviewRequired{
        t.Fatalf("software zero misrepresented as physical verification: %+v",view)
    }
    if err:=m.TrackSent(scope,"cue-1",clock.Add(3*time.Second));err!=nil{t.Fatal(err)}
    view=m.Snapshot()
    if view.LastCommandEvidence!=SentUnconfirmed{
        t.Fatalf("sent cue falsely acknowledged: %+v",view)
    }
    if m.Report(scope,3,true,true,"guessed-command",true) {
        t.Fatal("unsolicited software ACK accepted")
    }
    if !m.Report(scope,3,true,true,"cue-1",true){
        t.Fatal("matching current software ACK refused")
    }
    view=m.Snapshot()
    if view.LastCommandEvidence!=SoftwareAcknowledged||view.PhysicalOutputVerified||
        view.CommandsEnabled {
        t.Fatalf("software ACK escalated to physical/output authority: %+v",view)
    }
    if m.Report(scope,3,true,true,"cue-1",true) {
        t.Fatal("repeated ACK accepted")
    }
    if err:=m.TrackSent(scope,"cue-1",clock.Add(time.Second));err==nil {
        t.Fatal("duplicate command ID tracked twice")
    }
}

func TestWatchdogDisconnectRejectsOldSocketAndNeverReplaysOldID(t *testing.T){
    m,clock,a:=fixture(t)
    if err:=m.AttachAuthenticated(a);err!=nil{t.Fatal(err)}
    if !m.Heartbeat(a,1)||!m.Report(a,2,true,false,"",false){t.Fatal("first report refused")}
    if err:=m.TrackSent(a,"video-show-1",clock.Add(4*time.Second));err!=nil{t.Fatal(err)}
    if !m.Detach(a){t.Fatal("current socket detach failed")}
    view:=m.Snapshot()
    if view.Condition!=Disconnected||view.LastCommandEvidence!=Unconfirmed||
        view.SoftwareLevelsKnown {t.Fatalf("disconnected output shown as known: %+v",view)}
    if m.Report(a,3,true,true,"video-show-1",true) {
        t.Fatal("late pre-disconnect ACK accepted")
    }
    if err:=m.AttachAuthenticated(a);!errors.Is(err,ErrUntrusted){
        t.Fatalf("same-generation reconnect accepted: %v",err)
    }
    b:=a;b.ConnectionGeneration=42
    if err:=m.AttachAuthenticated(b);err!=nil{t.Fatal(err)}
    if m.Detach(a){t.Fatal("old socket cleared replacement")}
    if m.Heartbeat(a,1000)||m.Report(a,1001,true,true,"",false){
        t.Fatal("pre-replacement observation accepted")
    }
    if err:=m.TrackSent(b,"video-show-1",clock.Add(time.Second));err==nil {
        t.Fatal("new scope accepted old command before a fresh report")
    }
    if !m.Heartbeat(b,1)||!m.Report(b,2,true,false,"",false){t.Fatal("fresh reconnect report rejected")}
    if err:=m.TrackSent(b,"video-show-1",clock.Add(2*time.Second));err==nil {
        t.Fatal("reconnect replayed old command ID")
    }
    if err:=m.TrackSent(b,"video-show-2",clock.Add(2*time.Second));err!=nil{
        t.Fatalf("fresh explicitly tracked ID rejected: %v",err)
    }
    if m.Report(a,4,true,true,"video-show-2",true) {
        t.Fatal("stale socket acknowledged new command")
    }
    if view:=m.Snapshot();view.LastCommandEvidence!=SentUnconfirmed {
        t.Fatalf("old ACK mutated current software evidence: %+v",view)
    }
}

func TestWatchdogHeartbeatExpiryClockRollbackAndExpiredAckFailClosed(t *testing.T){
    m,clock,a:=fixture(t)
    if err:=m.AttachAuthenticated(a);err!=nil{t.Fatal(err)}
    *clock=clock.Add(6*time.Second)
    if m.Heartbeat(a,1) {t.Fatal("late first heartbeat revived stale connection")}
    if got:=m.Snapshot();got.Condition!=HeartbeatStale||got.CommandsEnabled{
        t.Fatalf("stale socket shown current: %+v",got)
    }
    b:=a;b.ConnectionGeneration++
    if err:=m.AttachAuthenticated(b);err!=nil{t.Fatal(err)}
    if !m.Heartbeat(b,1)||!m.Report(b,2,true,true,"",false){t.Fatal("replacement not ready")}
    if err:=m.TrackSent(b,"audio-1",clock.Add(time.Second));err!=nil{t.Fatal(err)}
    *clock=clock.Add(2*time.Second)
    if m.Report(b,3,true,true,"audio-1",true) {
        t.Fatal("expired command ACK accepted")
    }
    if got:=m.Snapshot();got.LastCommandEvidence!=Unconfirmed {
        t.Fatalf("expired command falsely acknowledged: %+v",got)
    }
    *clock=clock.Add(-10*time.Second)
    if m.Heartbeat(b,4){t.Fatal("clock reversal accepted")}
    if got:=m.Snapshot();got.Condition!=ClockUncertain||got.SoftwareLevelsKnown{
        t.Fatalf("clock reversal not fail-closed: %+v",got)
    }
    *clock=clock.Add(11*time.Second)
    if m.Heartbeat(b,5){t.Fatal("clock-uncertain socket revived without reconnect")}
    c:=b;c.ConnectionGeneration++
    if err:=m.AttachAuthenticated(c);err!=nil{t.Fatal(err)}
    if !m.Heartbeat(c,1)||!m.Report(c,2,true,false,"",false){
        t.Fatal("trusted fresh replacement after clock correction refused")
    }
    *clock=clock.Add(3*time.Second)
    if !m.Heartbeat(c,3){t.Fatal("heartbeat unexpectedly refused")}
    *clock=clock.Add(3*time.Second)
    if !m.Heartbeat(c,4){t.Fatal("current heartbeat unexpectedly refused")}
    if got:=m.Snapshot();got.Condition!=AwaitingFreshReport||
        got.SoftwareLevelsKnown||got.SoftwareZeroReported{
        t.Fatalf("fresh heartbeat preserved stale output observation: %+v",got)
    }
}

func TestWatchdogAssignmentChangeMustAdvanceEpoch(t *testing.T){
    m,_,a:=fixture(t)
    if err:=m.AttachAuthenticated(a);err!=nil{t.Fatal(err)}
    changed:=a;changed.ProjectID="show-b";changed.ConnectionGeneration++
    if err:=m.AttachAuthenticated(changed);!errors.Is(err,ErrUntrusted){
        t.Fatalf("Project swapped at same epoch: %v",err)
    }
    changed.AssignmentEpoch++
    if err:=m.AttachAuthenticated(changed);err!=nil{t.Fatal(err)}
    if !m.Heartbeat(changed,1){t.Fatal("valid new Project epoch heartbeat refused")}
    if got:=m.Snapshot();got.Condition!=AwaitingFreshReport||got.SoftwareLevelsKnown||
        got.CommandsEnabled {t.Fatalf("Project transfer reactivated old report: %+v",got)}
}

func TestWatchdogConcurrentOldSocketCallbacksCannotDisplaceNewScope(t *testing.T){
    m,_,old:=fixture(t)
    if err:=m.AttachAuthenticated(old);err!=nil{t.Fatal(err)}
    current:=old;current.ConnectionGeneration++
    if err:=m.AttachAuthenticated(current);err!=nil{t.Fatal(err)}
    var group sync.WaitGroup
    for i:=0;i<8;i++{
        group.Add(1)
        go func(){
            defer group.Done()
            for j:=0;j<50;j++ {
                m.Detach(old)
                m.Heartbeat(old,uint64(j+1))
                m.Report(old,uint64(j+2),true,true,"",false)
                _=m.Snapshot()
            }
        }()
    }
    group.Wait()
    if got:=m.Snapshot();got.Scope!=current||got.Condition!=AwaitingFreshReport{
        t.Fatalf("late callbacks displaced active connection: %+v",got)
    }
}
