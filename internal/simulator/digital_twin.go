package simulator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// FaultScenario describes a deterministic simulation-only execution fault.
// At least one selector (TargetRef or Capability) is required. Uses=0 means
// persistent; Uses>0 consumes the scenario that many times.
type FaultScenario struct {
	TargetRef  string `json:"target_ref,omitempty"`
	Capability string `json:"capability,omitempty"`
	Behavior   string `json:"behavior"`
	DelayMS    int64  `json:"delay_ms,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
	Uses       int    `json:"uses,omitempty"`
}

// TargetState is simulation runtime truth for one logical target. It is never
// copied into or derived from the physical stage_devices/trust runtime.
type TargetState struct {
	TargetRef           string                 `json:"target_ref"`
	LogicalType         string                 `json:"logical_type,omitempty"`
	Online              bool                   `json:"online"`
	ExecutionCount      int64                  `json:"execution_count"`
	LastCapability      string                 `json:"last_capability,omitempty"`
	LastResult          domain.ExecutionResult `json:"last_result,omitempty"`
	LastErrorCode       string                 `json:"last_error_code,omitempty"`
	LastResponseSummary string                 `json:"last_response_summary,omitempty"`
}

// SessionSnapshot is a stable, copied view of one simulation Session.
type SessionSnapshot struct {
	SessionID string          `json:"session_id"`
	Targets   []TargetState   `json:"targets"`
	Faults    []FaultScenario `json:"faults"`
}

type twinSession struct {
	targets map[string]*TargetState
	faults  []FaultScenario
}

// DigitalTwin owns runtime-ephemeral simulation truth. State is keyed by the
// authoritative Session ID so separate simulation runs cannot contaminate one
// another. It intentionally has no dependency on Stage Device runtime state.
type DigitalTwin struct {
	mu       sync.RWMutex
	sessions map[string]*twinSession
	fallback capability.Executor
}

func NewDigitalTwin() *DigitalTwin {
	return &DigitalTwin{
		sessions: make(map[string]*twinSession),
		fallback: Adapter{},
	}
}

func (d *DigitalTwin) ConfigureFault(sessionID string, scenario FaultScenario) error {
	if d == nil {
		return fmt.Errorf("digital twin is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}
	scenario.TargetRef = strings.TrimSpace(scenario.TargetRef)
	scenario.Capability = strings.TrimSpace(scenario.Capability)
	scenario.Behavior = strings.ToUpper(strings.TrimSpace(scenario.Behavior))
	if scenario.TargetRef == "" && scenario.Capability == "" {
		return fmt.Errorf("fault scenario requires target_ref or capability")
	}
	if scenario.Uses < 0 {
		return fmt.Errorf("fault scenario uses cannot be negative")
	}
	if scenario.DelayMS < 0 {
		return fmt.Errorf("fault scenario delay cannot be negative")
	}
	if !validFaultBehavior(scenario.Behavior) {
		return fmt.Errorf("unsupported fault behavior %q", scenario.Behavior)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	session := d.ensureSessionLocked(sessionID)
	for i := range session.faults {
		if sameFaultSelector(session.faults[i], scenario) {
			session.faults[i] = scenario
			return nil
		}
	}
	session.faults = append(session.faults, scenario)
	return nil
}

func (d *DigitalTwin) ClearFault(sessionID, targetRef, capabilityKey string) {
	if d == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRef = strings.TrimSpace(targetRef)
	capabilityKey = strings.TrimSpace(capabilityKey)

	d.mu.Lock()
	defer d.mu.Unlock()
	session := d.sessions[sessionID]
	if session == nil {
		return
	}
	kept := session.faults[:0]
	for _, fault := range session.faults {
		if fault.TargetRef == targetRef && fault.Capability == capabilityKey {
			continue
		}
		kept = append(kept, fault)
	}
	session.faults = kept
}

func (d *DigitalTwin) SetTargetOnline(sessionID, targetRef string, online bool) error {
	if d == nil {
		return fmt.Errorf("digital twin is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRef = strings.TrimSpace(targetRef)
	if sessionID == "" || targetRef == "" {
		return fmt.Errorf("session ID and target reference are required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	session := d.ensureSessionLocked(sessionID)
	state := ensureTargetLocked(session, targetRef, "")
	state.Online = online
	return nil
}

func (d *DigitalTwin) ResetSession(sessionID string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	delete(d.sessions, strings.TrimSpace(sessionID))
	d.mu.Unlock()
}

func (d *DigitalTwin) Snapshot(sessionID string) SessionSnapshot {
	snapshot := SessionSnapshot{SessionID: strings.TrimSpace(sessionID)}
	if d == nil || snapshot.SessionID == "" {
		return snapshot
	}

	d.mu.RLock()
	session := d.sessions[snapshot.SessionID]
	if session != nil {
		for _, target := range session.targets {
			snapshot.Targets = append(snapshot.Targets, *target)
		}
		snapshot.Faults = append(snapshot.Faults, session.faults...)
	}
	d.mu.RUnlock()

	sort.Slice(snapshot.Targets, func(i, j int) bool {
		return snapshot.Targets[i].TargetRef < snapshot.Targets[j].TargetRef
	})
	sort.Slice(snapshot.Faults, func(i, j int) bool {
		if snapshot.Faults[i].TargetRef != snapshot.Faults[j].TargetRef {
			return snapshot.Faults[i].TargetRef < snapshot.Faults[j].TargetRef
		}
		return snapshot.Faults[i].Capability < snapshot.Faults[j].Capability
	})
	return snapshot
}

func (d *DigitalTwin) Execute(ctx context.Context, req capability.Request) capability.Result {
	if d == nil {
		return twinFailure("DIGITAL_TWIN_UNAVAILABLE", "digital twin is unavailable")
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return twinFailure("SIM_SESSION_ID_REQUIRED", "simulation requires authoritative session identity")
	}
	targetRef, logicalType := requestTarget(req)

	var scenario *FaultScenario
	var online bool
	d.mu.Lock()
	session := d.ensureSessionLocked(sessionID)
	state := ensureTargetLocked(session, targetRef, logicalType)
	state.ExecutionCount++
	state.LastCapability = strings.TrimSpace(req.Capability)
	online = state.Online
	if match, index := matchedFault(session.faults, targetRef, req.Capability); index >= 0 {
		copy := match
		scenario = &copy
		if match.Uses > 0 {
			session.faults[index].Uses--
			if session.faults[index].Uses == 0 {
				session.faults = append(session.faults[:index], session.faults[index+1:]...)
			}
		}
	}
	d.mu.Unlock()

	var result capability.Result
	if scenario != nil && scenario.Behavior == "RECONNECT" {
		if interrupted := waitDelay(ctx, scenario.DelayMS); interrupted != nil {
			result = fromContext(interrupted)
		} else {
			d.setOnline(sessionID, targetRef, true)
			result = capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckNone, ResponseSummary: scenarioMessage(*scenario, "simulated reconnect")}
		}
	} else if !online {
		result = twinFailure("SIM_TARGET_OFFLINE", "simulated target is offline")
	} else if scenario != nil {
		result = d.executeScenario(ctx, sessionID, targetRef, *scenario)
	} else {
		result = d.fallback.Execute(ctx, req)
	}

	d.recordResult(sessionID, targetRef, result)
	return result
}

func (d *DigitalTwin) executeScenario(ctx context.Context, sessionID, targetRef string, scenario FaultScenario) capability.Result {
	switch scenario.Behavior {
	case "COMPLETE", "DELAY":
		if interrupted := waitDelay(ctx, scenario.DelayMS); interrupted != nil {
			return fromContext(interrupted)
		}
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckNone, ResponseSummary: scenarioMessage(scenario, "simulated completion")}
	case "FAIL":
		if interrupted := waitDelay(ctx, scenario.DelayMS); interrupted != nil {
			return fromContext(interrupted)
		}
		code := strings.TrimSpace(scenario.ErrorCode)
		if code == "" {
			code = "SIMULATED_FAILURE"
		}
		return twinFailure(code, scenarioMessage(scenario, "simulated failure"))
	case "REJECT":
		if interrupted := waitDelay(ctx, scenario.DelayMS); interrupted != nil {
			return fromContext(interrupted)
		}
		code := strings.TrimSpace(scenario.ErrorCode)
		if code == "" {
			code = "SIM_TARGET_REJECTED"
		}
		return twinFailure(code, scenarioMessage(scenario, "simulated target rejected command"))
	case "OFFLINE", "DISCONNECT":
		if interrupted := waitDelay(ctx, scenario.DelayMS); interrupted != nil {
			return fromContext(interrupted)
		}
		d.setOnline(sessionID, targetRef, false)
		code := strings.TrimSpace(scenario.ErrorCode)
		if code == "" {
			code = "SIM_TARGET_OFFLINE"
		}
		return twinFailure(code, scenarioMessage(scenario, "simulated target disconnected"))
	case "TIMEOUT":
		if _, ok := ctx.Deadline(); !ok {
			return twinFailure("SIM_TIMEOUT_REQUIRES_DEADLINE", "timeout simulation requires timeout_policy.timeout_ms")
		}
		<-ctx.Done()
		return fromContext(ctx.Err())
	default:
		return twinFailure("SIM_UNKNOWN_BEHAVIOR", "unknown digital twin behavior")
	}
}

func (d *DigitalTwin) ensureSessionLocked(sessionID string) *twinSession {
	session := d.sessions[sessionID]
	if session == nil {
		session = &twinSession{targets: make(map[string]*TargetState)}
		d.sessions[sessionID] = session
	}
	return session
}

func ensureTargetLocked(session *twinSession, targetRef, logicalType string) *TargetState {
	state := session.targets[targetRef]
	if state == nil {
		state = &TargetState{TargetRef: targetRef, LogicalType: logicalType, Online: true}
		session.targets[targetRef] = state
	} else if state.LogicalType == "" && logicalType != "" {
		state.LogicalType = logicalType
	}
	return state
}

func (d *DigitalTwin) setOnline(sessionID, targetRef string, online bool) {
	d.mu.Lock()
	session := d.ensureSessionLocked(sessionID)
	ensureTargetLocked(session, targetRef, "").Online = online
	d.mu.Unlock()
}

func (d *DigitalTwin) recordResult(sessionID, targetRef string, result capability.Result) {
	d.mu.Lock()
	session := d.ensureSessionLocked(sessionID)
	state := ensureTargetLocked(session, targetRef, "")
	state.LastResult = result.Result
	state.LastErrorCode = result.ErrorCode
	state.LastResponseSummary = result.ResponseSummary
	d.mu.Unlock()
}

func requestTarget(req capability.Request) (string, string) {
	if req.Target != nil {
		if ref := strings.TrimSpace(req.Target.Ref); ref != "" {
			return ref, strings.TrimSpace(req.Target.LogicalType)
		}
	}
	capabilityKey := strings.TrimSpace(req.Capability)
	if capabilityKey == "" {
		capabilityKey = "unknown"
	}
	return "capability:" + capabilityKey, ""
}

func matchedFault(faults []FaultScenario, targetRef, capabilityKey string) (FaultScenario, int) {
	targetRef = strings.TrimSpace(targetRef)
	capabilityKey = strings.TrimSpace(capabilityKey)
	best := -1
	bestScore := -1
	for i, fault := range faults {
		if fault.TargetRef != "" && fault.TargetRef != targetRef {
			continue
		}
		if fault.Capability != "" && fault.Capability != capabilityKey {
			continue
		}
		score := 0
		if fault.TargetRef != "" {
			score += 2
		}
		if fault.Capability != "" {
			score++
		}
		if score > bestScore {
			best = i
			bestScore = score
		}
	}
	if best < 0 {
		return FaultScenario{}, -1
	}
	return faults[best], best
}

func sameFaultSelector(a, b FaultScenario) bool {
	return a.TargetRef == b.TargetRef && a.Capability == b.Capability
}

func validFaultBehavior(behavior string) bool {
	switch behavior {
	case "COMPLETE", "DELAY", "FAIL", "TIMEOUT", "OFFLINE", "DISCONNECT", "REJECT", "RECONNECT":
		return true
	default:
		return false
	}
}

func scenarioMessage(scenario FaultScenario, fallback string) string {
	if message := strings.TrimSpace(scenario.Message); message != "" {
		return message
	}
	return fallback
}

func twinFailure(code, summary string) capability.Result {
	return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: code, ResponseSummary: summary}
}
