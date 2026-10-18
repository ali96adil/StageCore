package outputwatch

import (
 "errors"
 "sync"
 "testing"
 "time"
)
func receiverFixture()( *ReceiverReceiptTracker,*time.Time,ReceiverScope,ReceiverDispatch ){
 now:=time.Date(2026,10,19,0,0,0,0,time.UTC)
 s:=ReceiverScope{ProjectID:"project-a",RuntimeSnapshotID:"snapshot-a",
  MachineRoleID:"ableton",CompanionID:"companion-a",
  ReceiverInstanceID:"max4live-instrument-a",Kind:ReceiverAbleton,ConnectionGeneration:12}
 d:=ReceiverDispatch{Scope:s,ExecutionID:"exec-a",CueID:"cue-7",
  Challenge:"nonce-unique-abcdef-2026",ExpiresAt:now.Add(5*time.Second)}
 return NewReceiverReceiptTracker(func()time.Time{return now}),&now,s,d
}
func TestReceiverReceiptsRequireExactCueExecutionNonceReceiverAndSequence(t *testing.T){
 r,_,scope,d:=receiverFixture()
 if err:=r.TrackTransportSent(d);!errors.Is(err,ErrReceiptUntrusted){t.Fatalf("accepted without bound receiver: %v",err)}
 if err:=r.BindQualified(scope);err!=nil{t.Fatal(err)}
 if err:=r.TrackTransportSent(d);err!=nil{t.Fatal(err)}
 if got,ok:=r.View(d.ExecutionID);!ok||got.State!=ReceiptTransportOnly||
  got.ApplicationReceiptCorrelated||got.PhysicalOutputVerified||got.CommandsEnabled{
  t.Fatalf("Companion transport send claimed app receipt: %+v %v",got,ok)
 }
 base:=ReceiverNotice{Scope:scope,ExecutionID:d.ExecutionID,
  CueID:d.CueID,Challenge:d.Challenge,Sequence:1,Status:ReceiptAppReceived}
 invalids:=[]struct{name string;edit func(*ReceiverNotice)}{
  {"wrong project",func(n *ReceiverNotice){n.Scope.ProjectID="project-b"}},
  {"wrong snapshot",func(n *ReceiverNotice){n.Scope.RuntimeSnapshotID="snapshot-b"}},
  {"wrong role",func(n *ReceiverNotice){n.Scope.MachineRoleID="vdmx"}},
  {"wrong companion",func(n *ReceiverNotice){n.Scope.CompanionID="other"}},
  {"wrong receiver",func(n *ReceiverNotice){n.Scope.ReceiverInstanceID="spoof"}},
  {"old generation",func(n *ReceiverNotice){n.Scope.ConnectionGeneration=11}},
  {"wrong cue",func(n *ReceiverNotice){n.CueID="other-cue"}},
  {"wrong execution",func(n *ReceiverNotice){n.ExecutionID="other-exec"}},
  {"wrong nonce",func(n *ReceiverNotice){n.Challenge="guessed-nonce"}},
  {"no sequence",func(n *ReceiverNotice){n.Sequence=0}},
  {"skip receive",func(n *ReceiverNotice){n.Status=ReceiptAppApplied}},
 }
 for _,test:=range invalids{
  t.Run(test.name,func(t *testing.T){
   n:=base;test.edit(&n)
   if err:=r.Observe(n);!errors.Is(err,ErrReceiptUntrusted){
    t.Fatalf("accepted unqualified app receipt: %+v",n)
   }
  })
 }
 if err:=r.Observe(base);err!=nil{t.Fatalf("real app RECEIVE rejected: %v",err)}
 if err:=r.Observe(base);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("duplicate sequence accepted")
 }
 if got,_:=r.View(d.ExecutionID);got.State!=ReceiptAppReceived||
  !got.ApplicationReceiptCorrelated||got.PhysicalOutputVerified{
  t.Fatalf("app RECEIVE not distinguished from physical output: %+v",got)
 }
 applied:=base;applied.Sequence=2;applied.Status=ReceiptAppApplied
 if err:=r.Observe(applied);err!=nil{t.Fatalf("app APPLIED rejected: %v",err)}
 if got,_:=r.View(d.ExecutionID);got.State!=ReceiptAppApplied||
  !got.ApplicationReceiptCorrelated||got.PhysicalOutputVerified||got.CommandsEnabled{
  t.Fatalf("self-reported app APPLIED misrepresented: %+v",got)
 }
 if err:=r.Observe(applied);!errors.Is(err,ErrReceiptUntrusted){t.Fatal("replayed applied ack accepted")}
 if err:=r.TrackTransportSent(d);!errors.Is(err,ErrReceiptUntrusted){t.Fatal("reused Execution ID")}
 // A second Cue cannot use the first Cue's receipt, even on the same role.
 second:=d;second.ExecutionID="exec-b";second.CueID="cue-8";second.Challenge="nonce-for-new-cue-654321"
 if err:=r.TrackTransportSent(second);err!=nil{t.Fatal(err)}
 if err:=r.Observe(ReceiverNotice{Scope:scope,ExecutionID:second.ExecutionID,CueID:d.CueID,
  Challenge:second.Challenge,Sequence:3,Status:ReceiptAppReceived});!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("first Cue's ack upgraded second Cue")
 }
}
func TestReceiverReceiptTimeoutDisconnectAndOldReconnectFailClosed(t *testing.T){
 r,clock,old,d:=receiverFixture()
 if err:=r.BindQualified(old);err!=nil{t.Fatal(err)}
 if err:=r.TrackTransportSent(d);err!=nil{t.Fatal(err)}
 *clock=clock.Add(6*time.Second)
 if err:=r.Observe(ReceiverNotice{Scope:old,ExecutionID:d.ExecutionID,CueID:d.CueID,
  Challenge:d.Challenge,Sequence:1,Status:ReceiptAppReceived});!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("expired receiver receipt was trusted")
 }
 if v,_:=r.View(d.ExecutionID);v.State!=ReceiptUnconfirmed{t.Fatalf("expired receipt not unconfirmed: %+v",v)}
 if !r.Disconnect(old){t.Fatal("failed exact session disconnect")}
 if err:=r.BindQualified(old);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("reconnected with same generation")
 }
 newer:=old;newer.ConnectionGeneration++
 if err:=r.BindQualified(newer);err!=nil{t.Fatal(err)}
 if r.Disconnect(old){t.Fatal("old socket invalidated replacement")}
 if err:=r.Observe(ReceiverNotice{Scope:old,ExecutionID:d.ExecutionID,CueID:d.CueID,
  Challenge:d.Challenge,Sequence:800,Status:ReceiptAppReceived});!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("late report from replaced receiver upgraded execution")
 }
 d.Scope=newer;d.ExpiresAt=clock.Add(5*time.Second)
 if err:=r.TrackTransportSent(d);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("reconnect replayed old Execution ID")
 }
 d.ExecutionID="exec-new";d.CueID="cue-new";d.Challenge="fresh-after-reconnect-nonce"
 if err:=r.TrackTransportSent(d);err!=nil{t.Fatal(err)}
 if !r.Disconnect(newer){t.Fatal("did not disconnect new session")}
 if v,_:=r.View(d.ExecutionID);v.State!=ReceiptUnconfirmed||v.PhysicalOutputVerified{
  t.Fatalf("disconnect left pending receipt confirmed: %+v",v)
 }
}
func TestReceiverClockRollbackAndConcurrentGuessedNoticesFailClosed(t *testing.T){
 r,clock,scope,d:=receiverFixture()
 if err:=r.BindQualified(scope);err!=nil{t.Fatal(err)}
 if err:=r.TrackTransportSent(d);err!=nil{t.Fatal(err)}
 var wg sync.WaitGroup
 for i:=0;i<8;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   for j:=0;j<70;j++{
    n:=ReceiverNotice{Scope:scope,ExecutionID:d.ExecutionID,CueID:"foreign-cue",
     Challenge:"wrong",Sequence:uint64(j+1),Status:ReceiptAppReceived}
    _=r.Observe(n);_,_=r.View(d.ExecutionID)
   }
  }()
 }
 wg.Wait()
 if v,_:=r.View(d.ExecutionID);v.State!=ReceiptTransportOnly {
  t.Fatalf("concurrent untrusted notices changed state: %+v",v)
 }
 *clock=clock.Add(-time.Second)
 if err:=r.Observe(ReceiverNotice{Scope:scope,ExecutionID:d.ExecutionID,CueID:d.CueID,
  Challenge:d.Challenge,Sequence:1,Status:ReceiptAppReceived});!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("clock reversal accepted")
 }
 if v,_:=r.View(d.ExecutionID);v.State!=ReceiptUnconfirmed{
  t.Fatalf("clock rollback did not invalidate pending: %+v",v)
 }
 if err:=r.BindQualified(scope);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("clock rollback re-bound without newer generation")
 }
}
func TestReceiverReceiptScopeAndLimits(t *testing.T){
 r,clock,scope,d:=receiverFixture()
 bad:=scope;bad.Kind="OTHER"
 if err:=r.BindQualified(bad);!errors.Is(err,ErrReceiptUntrusted){t.Fatal("unknown Receiver accepted")}
 if err:=r.BindQualified(scope);err!=nil{t.Fatal(err)}
 for i:=0;i<maxReceiverReceiptExecutions;i++{
  item:=d;item.ExecutionID="execution-"+string(rune(0x1000+i))
  item.Challenge="nonce-challenge-for-limit"
  if err:=r.TrackTransportSent(item);err!=nil{t.Fatalf("unexpected limit at %d: %v",i,err)}
 }
 d.ExecutionID="execution-over-limit"
 if err:=r.TrackTransportSent(d);!errors.Is(err,ErrReceiptUntrusted){
  t.Fatal("execution cap evicted replay history")
 }
 *clock=clock.Add(6*time.Second)
 if v,ok:=r.View("execution-"+string(rune(0x1000)));!ok||v.State!=ReceiptUnconfirmed{
  t.Fatalf("pending expiry not reported UNKNOWN: %+v %v",v,ok)
 }
}
