// Package outputwatch qualifies device observations only. It has no
// output dispatch, authorization, or certified emergency-stop functionality.
package outputwatch

import (
    "errors"
    "strings"
    "sync"
    "time"
)

var ErrUntrusted = errors.New("outputwatch: observation or scope is stale, unsafe or untrusted")
const maxCommandIDs = 512

type Condition string
const (
    Disconnected Condition = "DISCONNECTED"
    AwaitingFreshReport Condition = "AWAITING_FRESH_REPORT"
    SoftwareReportCurrent Condition = "SOFTWARE_REPORT_CURRENT"
    HeartbeatStale Condition = "HEARTBEAT_STALE"
    ClockUncertain Condition = "CLOCK_UNCERTAIN"
)
type CommandEvidence string
const (
    NoCommand CommandEvidence = "NONE"
    SentUnconfirmed CommandEvidence = "SENT_UNCONFIRMED"
    SoftwareAcknowledged CommandEvidence = "SOFTWARE_ACKNOWLEDGED"
    Unconfirmed CommandEvidence = "UNCONFIRMED"
)
type Scope struct {
    DeviceID string
    ProjectID string
    RuntimeSnapshotID string
    AssignmentEpoch int64
    ConnectionGeneration int64
}
func (s Scope) valid() bool {
    return strings.TrimSpace(s.DeviceID)!="" && strings.TrimSpace(s.ProjectID)!="" &&
        strings.TrimSpace(s.RuntimeSnapshotID)!="" && s.AssignmentEpoch>0 && s.ConnectionGeneration>0
}
// View provides diagnostics, not permission to dispatch. A software-zero
// observation never proves physical DMX/LED darkness, speaker silence,
// power isolation or a certified external emergency stop.
type View struct {
    Scope Scope
    Condition Condition
    LastHeartbeatAt time.Time
    LastReportAt time.Time
    SoftwareLevelsKnown bool
    SoftwareZeroReported bool
    PhysicalOutputVerified bool
    CommandsEnabled bool
    LastCommandID string
    LastCommandEvidence CommandEvidence
    OperatorReviewRequired bool
}
// Monitor is concurrency-safe and never invokes network, file or device I/O.
// Calling code MUST validate an authenticated current device socket and its
// assignment before passing data. This struct is not a trust boundary itself.
type Monitor struct {
    mu sync.Mutex
    now func() time.Time
    ttl time.Duration
    bound bool
    scope Scope
    boundAt time.Time
    lastClock time.Time
    heartbeatAt time.Time
    reportAt time.Time
    seq uint64
    condition Condition
    known, zero bool
    pendingID string
    pendingUntil time.Time
    lastCommand string
    lastEvidence CommandEvidence
    seen map[string]struct{}
}
func New(ttl time.Duration, now func() time.Time) (*Monitor,error) {
    if ttl<=0 || ttl>time.Minute { return nil,ErrUntrusted }
    if now==nil {now=time.Now}
    return &Monitor{ttl:ttl,now:now,condition:Disconnected,
        lastEvidence:NoCommand,seen:make(map[string]struct{})},nil
}
// AttachAuthenticated is only called AFTER an external authenticated-socket
// check. Reconnect always needs a larger durable connection generation;
// changed Projects also need a larger assignment epoch.
func (m *Monitor) AttachAuthenticated(s Scope) error {
    if m==nil || !s.valid() {return ErrUntrusted}
    m.mu.Lock();defer m.mu.Unlock()
    if m.bound {
        if s.DeviceID!=m.scope.DeviceID ||
            s.ConnectionGeneration<=m.scope.ConnectionGeneration ||
            s.AssignmentEpoch<m.scope.AssignmentEpoch ||
            (s.ProjectID!=m.scope.ProjectID && s.AssignmentEpoch<=m.scope.AssignmentEpoch) {
            return ErrUntrusted
        }
    }
    if m.pendingID!="" {m.lastEvidence=Unconfirmed;m.pendingID=""}
    now:=m.now()
    m.scope=s;m.bound=true;m.boundAt=now;m.lastClock=now
    m.heartbeatAt=time.Time{};m.reportAt=time.Time{};m.seq=0
    m.condition=AwaitingFreshReport;m.known=false;m.zero=false
    return nil
}
// Detach only accepts the exact socket's scope. Late old-socket cleanup
// cannot clobber a newer connection's observation.
func (m *Monitor) Detach(s Scope) bool {
    if m==nil {return false}
    m.mu.Lock();defer m.mu.Unlock()
    if !m.bound||s!=m.scope {return false}
    m.condition=Disconnected;m.known=false;m.zero=false
    if m.pendingID!="" {m.lastEvidence=Unconfirmed;m.pendingID=""}
    return true
}
func (m *Monitor) checkTime(now time.Time) bool {
    if now.Before(m.lastClock) {
        m.condition=ClockUncertain
        m.known=false;m.zero=false
        if m.pendingID!="" {m.lastEvidence=Unconfirmed;m.pendingID=""}
        return false
    }
    m.lastClock=now
    if m.condition==Disconnected || m.condition==HeartbeatStale ||
        m.condition==ClockUncertain {return false}
    reference:=m.heartbeatAt
    if reference.IsZero() {reference=m.boundAt}
    if now.Sub(reference)>m.ttl {
        m.condition=HeartbeatStale;m.known=false;m.zero=false
        if m.pendingID!="" {m.lastEvidence=Unconfirmed;m.pendingID=""}
        return false
    }
    if !m.reportAt.IsZero() && now.Sub(m.reportAt)>m.ttl {
        m.condition=AwaitingFreshReport;m.known=false;m.zero=false
    }
    if m.pendingID!="" && !now.Before(m.pendingUntil) {
        m.lastEvidence=Unconfirmed;m.pendingID=""
    }
    return true
}
// Heartbeat is liveness only, never applied-output or physical evidence.
func (m *Monitor) Heartbeat(s Scope, seq uint64) bool {
    if m==nil{return false}
    m.mu.Lock();defer m.mu.Unlock()
    at:=m.now()
    if !m.bound||s!=m.scope||seq==0||seq<=m.seq||!m.checkTime(at){return false}
    m.seq=seq;m.heartbeatAt=at
    return true
}
// Report is an authenticated peer's software observation, not a verified
// sensor read. An ack can qualify only an explicitly tracked current ID.
// Old socket, expired, guessed or repeated ACKs never advance evidence.
func (m *Monitor) Report(s Scope, seq uint64, levelsKnown, softwareZero bool,
    ackCommandID string, applied bool) bool {
    if m==nil || (softwareZero&&!levelsKnown) {return false}
    m.mu.Lock();defer m.mu.Unlock()
    at:=m.now()
    if !m.bound||s!=m.scope||seq==0||seq<=m.seq||
        !m.checkTime(at)||m.heartbeatAt.IsZero(){return false}
    if ackCommandID!="" &&
        (ackCommandID!=m.pendingID||!at.Before(m.pendingUntil)){return false}
    m.seq=seq;m.heartbeatAt=at;m.reportAt=at
    m.known=levelsKnown;m.zero=levelsKnown&&softwareZero
    m.condition=SoftwareReportCurrent
    if ackCommandID!=""{
        m.lastEvidence=Unconfirmed
        if applied {m.lastEvidence=SoftwareAcknowledged}
        m.lastCommand=ackCommandID;m.pendingID=""
    }
    return true
}
// TrackSent is diagnostics ONLY AFTER a trusted router has sent an action.
// No command is dispatched or authorized here. Duplicates (also across
// reconnect) and tracking-cap exhaustion fail closed, never evicted/replayed.
func (m *Monitor) TrackSent(s Scope, id string, until time.Time) error {
    if m==nil||strings.TrimSpace(id)=="" {return ErrUntrusted}
    m.mu.Lock();defer m.mu.Unlock()
    at:=m.now()
    if !m.bound||s!=m.scope||!m.checkTime(at)||
        m.condition!=SoftwareReportCurrent||m.pendingID!=""||
        !until.After(at)||until.After(at.Add(time.Minute)){return ErrUntrusted}
    if _,exists:=m.seen[id];exists{return ErrUntrusted}
    if len(m.seen)>=maxCommandIDs{return ErrUntrusted}
    m.seen[id]=struct{}{}
    m.pendingID=id;m.pendingUntil=until
    m.lastCommand=id;m.lastEvidence=SentUnconfirmed
    return nil
}
func (m *Monitor) Snapshot() View {
    if m==nil {return View{Condition:Disconnected,LastCommandEvidence:NoCommand,OperatorReviewRequired:true}}
    m.mu.Lock();defer m.mu.Unlock()
    if m.bound {m.checkTime(m.now())}
    return View{
        Scope:m.scope,Condition:m.condition,LastHeartbeatAt:m.heartbeatAt,
        LastReportAt:m.reportAt,SoftwareLevelsKnown:m.known,SoftwareZeroReported:m.zero,
        PhysicalOutputVerified:false,CommandsEnabled:false,
        LastCommandID:m.lastCommand,LastCommandEvidence:m.lastEvidence,
        OperatorReviewRequired:true,
    }
}
