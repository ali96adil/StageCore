package outputwatch

import (
 "errors"
 "strings"
 "sync"
 "time"
)

// ReceiverReceiptTracker models *future* application-generated receipts.
// It has no network adapter or production hooks. An external adapter must
// authenticate a trusted receiver instance, session and message before
// calling Bind or Observe. A nonce echoed by the app is correlation,
// NOT authentication and NEVER physical output verification.
var ErrReceiptUntrusted = errors.New("outputwatch: receiver receipt rejected")

type ReceiverKind string
const (
 ReceiverAbleton ReceiverKind = "ABLETON"
 ReceiverVDMX ReceiverKind = "VDMX"
)
type ReceiverReceiptState string
const (
 ReceiptTransportOnly ReceiverReceiptState = "TRANSPORT_ONLY"
 ReceiptUnconfirmed ReceiverReceiptState = "UNCONFIRMED"
 ReceiptAppReceived ReceiverReceiptState = "APP_REPORTED_RECEIVED"
 ReceiptAppApplied ReceiverReceiptState = "APP_REPORTED_APPLIED"
)
type ReceiverScope struct {
 ProjectID string
 RuntimeSnapshotID string
 MachineRoleID string
 CompanionID string
 ReceiverInstanceID string
 Kind ReceiverKind
 ConnectionGeneration int64
}
func(s ReceiverScope) valid()bool{
 return s.ProjectID!=""&&s.RuntimeSnapshotID!=""&&s.MachineRoleID!=""&&
  s.CompanionID!=""&&s.ReceiverInstanceID!=""&&s.ConnectionGeneration>0&&
  (s.Kind==ReceiverAbleton||s.Kind==ReceiverVDMX)
}
type ReceiverDispatch struct{
 Scope ReceiverScope
 ExecutionID string
 CueID string
 Challenge string
 ExpiresAt time.Time
}
type ReceiverNotice struct{
 Scope ReceiverScope
 ExecutionID string
 CueID string
 Challenge string
 Sequence uint64
 Status ReceiverReceiptState
}
type ReceiptView struct{
 ExecutionID string `json:"execution_id"`
 CueID string `json:"cue_id"`
 State ReceiverReceiptState `json:"state"`
 ReceiverKind ReceiverKind `json:"receiver_kind"`
 ProjectID string `json:"project_id"`
 RuntimeSnapshotID string `json:"runtime_snapshot_id"`
 ApplicationReceiptCorrelated bool `json:"application_receipt_correlated"`
 PhysicalOutputVerified bool `json:"physical_output_verified"`
 CommandsEnabled bool `json:"commands_enabled"`
 OperatorReviewRequired bool `json:"operator_review_required"`
}
type pendingReceiverReceipt struct{
 dispatch ReceiverDispatch
 state ReceiverReceiptState
}
type ReceiverReceiptTracker struct{
 mu sync.Mutex
 now func()time.Time
 scope ReceiverScope
 bound bool
 lastTime time.Time
 sequence uint64
 entries map[string]*pendingReceiverReceipt
}
const maxReceiverReceiptExecutions = 512
func NewReceiverReceiptTracker(now func()time.Time)*ReceiverReceiptTracker{
 if now==nil{now=time.Now}
 return &ReceiverReceiptTracker{now:now,entries:make(map[string]*pendingReceiverReceipt)}
}
// BindQualified is for a receiver session authenticated/authorized by a
// separate trusted adapter. Reconnection requires a strictly newer connection
// generation; all former pending receipts become UNCONFIRMED.
func(t *ReceiverReceiptTracker)BindQualified(s ReceiverScope)error{
 if t==nil||!s.valid(){return ErrReceiptUntrusted}
 t.mu.Lock();defer t.mu.Unlock()
 if t.scope.ConnectionGeneration>0&&(s.ConnectionGeneration<=t.scope.ConnectionGeneration||
  s.ProjectID!=t.scope.ProjectID||s.CompanionID!=t.scope.CompanionID||
  s.ReceiverInstanceID!=t.scope.ReceiverInstanceID||
  s.Kind!=t.scope.Kind||s.MachineRoleID!=t.scope.MachineRoleID){
  return ErrReceiptUntrusted
 }
 for _,e:=range t.entries{
  if e.state==ReceiptTransportOnly||e.state==ReceiptAppReceived{
   e.state=ReceiptUnconfirmed
  }
 }
 t.scope=s;t.bound=true;t.lastTime=t.now();t.sequence=0
 return nil
}
func(t *ReceiverReceiptTracker)clockValid(now time.Time)bool{
 if now.Before(t.lastTime){
  for _,e:=range t.entries{
   if e.state==ReceiptTransportOnly||e.state==ReceiptAppReceived{
    e.state=ReceiptUnconfirmed
   }
  }
  t.bound=false
  return false
 }
 t.lastTime=now
 return true
}
// TrackTransportSent records an already successful local MIDI/OSC send.
// Callers supply a cryptographically unpredictable challenge; a challenge
// is not a substitute for authenticated application receiver identity.
// No existing MIDI/OSC packet format is modified by this type.
func(t *ReceiverReceiptTracker)TrackTransportSent(d ReceiverDispatch)error{
 if t==nil||!d.Scope.valid()||strings.TrimSpace(d.ExecutionID)==""||
  strings.TrimSpace(d.CueID)==""||len(d.Challenge)<16 {return ErrReceiptUntrusted}
 t.mu.Lock();defer t.mu.Unlock()
 now:=t.now()
 if !t.bound||d.Scope!=t.scope||!t.clockValid(now)||
  !d.ExpiresAt.After(now)||d.ExpiresAt.After(now.Add(time.Minute))||
  len(t.entries)>=maxReceiverReceiptExecutions {return ErrReceiptUntrusted}
 if _,exists:=t.entries[d.ExecutionID];exists{return ErrReceiptUntrusted}
 t.entries[d.ExecutionID]=&pendingReceiverReceipt{dispatch:d,state:ReceiptTransportOnly}
 return nil
}
// Observe accepts only a strictly increasing application-report sequence,
// current exact Project/Snapshot/Role/Cue/Execution/nonce and unexpired
// receiver generation. RECEIVED must precede APPLIED. It does not dispatch
// a cue, clear blackout or upgrade any physical verification flag.
func(t *ReceiverReceiptTracker)Observe(n ReceiverNotice)error{
 if t==nil{return ErrReceiptUntrusted}
 t.mu.Lock();defer t.mu.Unlock()
 now:=t.now()
 if !t.bound||n.Scope!=t.scope||n.Sequence==0||n.Sequence<=t.sequence||
  !t.clockValid(now){return ErrReceiptUntrusted}
 e:=t.entries[n.ExecutionID]
 if e==nil||e.dispatch.Scope!=n.Scope||e.dispatch.CueID!=n.CueID||
  e.dispatch.Challenge!=n.Challenge||!now.Before(e.dispatch.ExpiresAt)||
  e.state==ReceiptUnconfirmed||e.state==ReceiptAppApplied{
  return ErrReceiptUntrusted
 }
 if n.Status==ReceiptAppReceived&&e.state==ReceiptTransportOnly{
  e.state=ReceiptAppReceived
 }else if n.Status==ReceiptAppApplied&&e.state==ReceiptAppReceived{
  e.state=ReceiptAppApplied
 }else{
  return ErrReceiptUntrusted
 }
 t.sequence=n.Sequence
 return nil
}
func(t *ReceiverReceiptTracker)Disconnect(s ReceiverScope)bool{
 if t==nil{return false}
 t.mu.Lock();defer t.mu.Unlock()
 if !t.bound||t.scope!=s{return false}
 t.bound=false
 for _,e:=range t.entries{
  if e.state==ReceiptTransportOnly||e.state==ReceiptAppReceived{
   e.state=ReceiptUnconfirmed
  }
 }
 return true
}
func(t *ReceiverReceiptTracker)View(executionID string)(ReceiptView,bool){
 if t==nil{return ReceiptView{},false}
 t.mu.Lock();defer t.mu.Unlock()
 e:=t.entries[executionID]
 if e==nil{return ReceiptView{},false}
 now:=t.now()
 if !t.clockValid(now)||!t.bound||
  (e.state!=ReceiptAppApplied&&!now.Before(e.dispatch.ExpiresAt)){
  if e.state==ReceiptTransportOnly||e.state==ReceiptAppReceived{
   e.state=ReceiptUnconfirmed
  }
 }
 d:=e.dispatch
 return ReceiptView{
  ExecutionID:d.ExecutionID,CueID:d.CueID,State:e.state,
  ReceiverKind:d.Scope.Kind,ProjectID:d.Scope.ProjectID,
  RuntimeSnapshotID:d.Scope.RuntimeSnapshotID,
  ApplicationReceiptCorrelated:e.state==ReceiptAppReceived||e.state==ReceiptAppApplied,
  PhysicalOutputVerified:false,CommandsEnabled:false,
  OperatorReviewRequired:true,
 },true
}
