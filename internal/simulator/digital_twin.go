package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

const DigitalTwinStateContractVersion1 = 1

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

// TargetStateTruth separates desired, observed and simulation-verified state.
// Scope is always SIMULATION_ONLY; none of these values are physical-device
// acknowledgements. Restorable stays false until a later checkpoint slice has
// captured enough state to prove reconstruction.
type TargetStateTruth struct {
	Version           int    `json:"version"`
	Scope             string `json:"scope"`
	DesiredOnline     *bool  `json:"desired_online,omitempty"`
	ObservedOnline    *bool  `json:"observed_online,omitempty"`
	VerifiedOnline    *bool  `json:"verified_online,omitempty"`
	Restorable        bool   `json:"restorable"`
	RestorationReason string `json:"restoration_reason,omitempty"`
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
	StateTruth          TargetStateTruth       `json:"state_truth"`
}

// SessionSnapshot is a stable, copied view of one simulation Session. The
// RuntimeSnapshotID binding prevents a Session's virtual state from being
// silently reused against another immutable executable definition.
type SessionSnapshot struct {
	Version           int             `json:"version"`
	SessionID         string          `json:"session_id"`
	RuntimeSnapshotID string          `json:"runtime_snapshot_id,omitempty"`
	Targets           []TargetState   `json:"targets"`
	Faults            []FaultScenario `json:"faults"`
}

type twinSession struct {
	runtimeSnapshotID string
	targets           map[string]*TargetState
	faults            []FaultScenario
}

// SimulationStateStore supplies canonical Session authority and Flight
// Recorder persistence. Store implements this interface without the simulator
// depending on the concrete persistence package.
type SimulationStateStore interface {
	GetSessionFoundation(context.Context, string) (domain.Session, error)
	AppendEvent(context.Context, *string, contracts.EventEnvelope) (contracts.EventEnvelope, error)
}

// DigitalTwin owns runtime-ephemeral simulation truth. State is keyed by the
// authoritative Session ID so separate simulation runs cannot contaminate one
// another. It intentionally has no dependency on Stage Device runtime state.
type DigitalTwin struct {
	mu       sync.RWMutex
	sessions map[string]*twinSession
	fallback capability.Executor
	store    SimulationStateStore
}

func NewDigitalTwin() *DigitalTwin {
	return &DigitalTwin{
		sessions: make(map[string]*twinSession),
		fallback: Adapter{},
	}
}

func NewDigitalTwinWithStateStore(store SimulationStateStore) *DigitalTwin {
	twin := NewDigitalTwin()
	twin.SetStateStore(store)
	return twin
}

// SetStateStore enables canonical simulation observability. It is safe to call
// repeatedly with the same Store while wiring Cue and Routing through one Twin.
func (d *DigitalTwin) SetStateStore(store SimulationStateStore) {
	if d == nil || store == nil {
		return
	}
	d.mu.Lock()
	d.store = store
	d.mu.Unlock()
}

// BindSession pins virtual state to the authoritative immutable Runtime
// Snapshot for one SIMULATION Session. Rebinding the same Session to a different
// Snapshot is rejected instead of silently reusing stale state.
func (d *DigitalTwin) BindSession(session domain.Session) error {
	if d == nil {
		return fmt.Errorf("digital twin is unavailable")
	}
	if strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.RuntimeSnapshotID) == "" {
		return fmt.Errorf("simulation session and runtime snapshot are required")
	}
	if session.Type != domain.SessionSimulation {
		return fmt.Errorf("digital twin can bind only SIMULATION sessions")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	twinSession := d.ensureSessionLocked(strings.TrimSpace(session.ID))
	return bindSnapshotLocked(twinSession, strings.TrimSpace(session.RuntimeSnapshotID))
}

func (d *DigitalTwin) ConfigureFault(sessionID string, scenario FaultScenario) error {
	return d.ConfigureFaultContext(context.Background(), sessionID, scenario)
}

func (d *DigitalTwin) ConfigureFaultContext(ctx context.Context, sessionID string, scenario FaultScenario) error {
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

	session, err := d.resolveMutationSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := d.appendEvent(ctx, session, "simulation.fault.configured", map[string]any{
		"state_contract_version": DigitalTwinStateContractVersion1,
		"fault":                  scenario,
	}); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	twinSession := d.ensureSessionLocked(sessionID)
	for i := range twinSession.faults {
		if sameFaultSelector(twinSession.faults[i], scenario) {
			twinSession.faults[i] = scenario
			return nil
		}
	}
	twinSession.faults = append(twinSession.faults, scenario)
	return nil
}

func (d *DigitalTwin) ClearFault(sessionID, targetRef, capabilityKey string) error {
	return d.ClearFaultContext(context.Background(), sessionID, targetRef, capabilityKey)
}

func (d *DigitalTwin) ClearFaultContext(ctx context.Context, sessionID, targetRef, capabilityKey string) error {
	if d == nil {
		return fmt.Errorf("digital twin is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRef = strings.TrimSpace(targetRef)
	capabilityKey = strings.TrimSpace(capabilityKey)
	if sessionID == "" || (targetRef == "" && capabilityKey == "") {
		return fmt.Errorf("session ID and fault selector are required")
	}

	session, err := d.resolveMutationSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := d.appendEvent(ctx, session, "simulation.fault.cleared", map[string]any{
		"state_contract_version": DigitalTwinStateContractVersion1,
		"target_ref":             targetRef,
		"capability":             capabilityKey,
	}); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	twinSession := d.sessions[sessionID]
	if twinSession == nil {
		return nil
	}
	kept := twinSession.faults[:0]
	for _, fault := range twinSession.faults {
		if fault.TargetRef == targetRef && fault.Capability == capabilityKey {
			continue
		}
		kept = append(kept, fault)
	}
	twinSession.faults = kept
	return nil
}

func (d *DigitalTwin) SetTargetOnline(sessionID, targetRef string, online bool) error {
	return d.SetTargetOnlineContext(context.Background(), sessionID, targetRef, online)
}

func (d *DigitalTwin) SetTargetOnlineContext(ctx context.Context, sessionID, targetRef string, online bool) error {
	if d == nil {
		return fmt.Errorf("digital twin is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	targetRef = strings.TrimSpace(targetRef)
	if sessionID == "" || targetRef == "" {
		return fmt.Errorf("session ID and target reference are required")
	}

	session, err := d.resolveMutationSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := d.appendEvent(ctx, session, "simulation.target.state_changed", map[string]any{
		"state_contract_version": DigitalTwinStateContractVersion1,
		"target_ref":             targetRef,
		"desired_online":         online,
		"scope":                  "SIMULATION_ONLY",
	}); err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	twinSession := d.ensureSessionLocked(sessionID)
	state := ensureTargetLocked(twinSession, targetRef, "")
	setOnlineTruth(state, online, true)
	return nil
}

func (d *DigitalTwin) ResetSession(sessionID string) error {
	return d.ResetSessionContext(context.Background(), sessionID)
}

func (d *DigitalTwin) ResetSessionContext(ctx context.Context, sessionID string) error {
	if d == nil {
		return fmt.Errorf("digital twin is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}
	session, err := d.resolveMutationSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := d.appendEvent(ctx, session, "simulation.twin.reset", map[string]any{
		"state_contract_version": DigitalTwinStateContractVersion1,
	}); err != nil {
		return err
	}
	d.mu.Lock()
	delete(d.sessions, sessionID)
	d.mu.Unlock()
	return nil
}

func (d *DigitalTwin) Snapshot(sessionID string) SessionSnapshot {
	snapshot := SessionSnapshot{
		Version:   DigitalTwinStateContractVersion1,
		SessionID: strings.TrimSpace(sessionID),
	}
	if d == nil || snapshot.SessionID == "" {
		return snapshot
	}

	d.mu.RLock()
	session := d.sessions[snapshot.SessionID]
	if session != nil {
		snapshot.RuntimeSnapshotID = session.runtimeSnapshotID
		for _, target := range session.targets {
			snapshot.Targets = append(snapshot.Targets, cloneTargetState(*target))
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
	var consumed *FaultScenario
	d.mu.Lock()
	session := d.ensureSessionLocked(sessionID)
	if snapshotID := strings.TrimSpace(req.RuntimeSnapshotID); snapshotID != "" {
		if err := bindSnapshotLocked(session, snapshotID); err != nil {
			d.mu.Unlock()
			return twinFailure("SIM_RUNTIME_SNAPSHOT_MISMATCH", err.Error())
		}
	}
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
				consumedCopy := match
				consumed = &consumedCopy
				session.faults = append(session.faults[:index], session.faults[index+1:]...)
			}
		}
	}
	d.mu.Unlock()

	if consumed != nil {
		_ = d.appendExecutionEvent(ctx, req, "simulation.fault.consumed", map[string]any{
			"state_contract_version": DigitalTwinStateContractVersion1,
			"fault":                  *consumed,
		})
	}

	var result capability.Result
	if scenario != nil && scenario.Behavior == "RECONNECT" {
		if interrupted := waitDelay(ctx, scenario.DelayMS); interrupted != nil {
			result = fromContext(interrupted)
		} else {
			d.setObservedOnline(sessionID, targetRef, true)
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
	_ = d.appendExecutionOutcome(ctx, req, targetRef, logicalType, result)
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
		d.setObservedOnline(sessionID, targetRef, false)
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

func (d *DigitalTwin) resolveMutationSession(ctx context.Context, sessionID string) (domain.Session, error) {
	d.mu.RLock()
	stateStore := d.store
	d.mu.RUnlock()
	if stateStore == nil {
		return domain.Session{ID: sessionID, Type: domain.SessionSimulation, Status: domain.SessionActive}, nil
	}
	session, err := stateStore.GetSessionFoundation(ctx, sessionID)
	if err != nil {
		return domain.Session{}, fmt.Errorf("resolve simulation session: %w", err)
	}
	if session.Type != domain.SessionSimulation {
		return domain.Session{}, fmt.Errorf("simulation state mutation requires SIMULATION session")
	}
	if session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return domain.Session{}, fmt.Errorf("simulation state mutation requires ACTIVE session")
	}
	if err := d.BindSession(session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}

func (d *DigitalTwin) appendEvent(ctx context.Context, session domain.Session, eventType string, payload any) error {
	d.mu.RLock()
	stateStore := d.store
	d.mu.RUnlock()
	if stateStore == nil {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal simulation event: %w", err)
	}
	sessionID := session.ID
	_, err = stateStore.AppendEvent(ctx, &sessionID, contracts.EventEnvelope{
		EventType:         eventType,
		SchemaVersion:     contracts.SchemaVersion1,
		Source:            "stagecore.simulator.digital_twin",
		ProjectID:         session.ProjectID,
		RuntimeSnapshotID: session.RuntimeSnapshotID,
		Priority:          "P2",
		TraceContext:      json.RawMessage(`{}`),
		Payload:           body,
	})
	if err != nil {
		return fmt.Errorf("append simulation event: %w", err)
	}
	return nil
}

func (d *DigitalTwin) appendExecutionEvent(ctx context.Context, req capability.Request, eventType string, payload any) error {
	session := domain.Session{
		ID:                strings.TrimSpace(req.SessionID),
		ProjectID:         strings.TrimSpace(req.ProjectID),
		RuntimeSnapshotID: strings.TrimSpace(req.RuntimeSnapshotID),
		Type:              domain.SessionSimulation,
		Status:            domain.SessionActive,
	}
	return d.appendEvent(ctx, session, eventType, payload)
}

func (d *DigitalTwin) appendExecutionOutcome(ctx context.Context, req capability.Request, targetRef, logicalType string, result capability.Result) error {
	eventType := "simulation.execution.failed"
	switch result.Result {
	case domain.ExecutionCompleted:
		eventType = "simulation.execution.completed"
	case domain.ExecutionTimedOut:
		eventType = "simulation.execution.timed_out"
	case domain.ExecutionCancelled:
		eventType = "simulation.execution.cancelled"
	}
	return d.appendExecutionEvent(ctx, req, eventType, map[string]any{
		"state_contract_version": DigitalTwinStateContractVersion1,
		"scope":                  "SIMULATION_ONLY",
		"target_ref":             targetRef,
		"logical_type":           logicalType,
		"capability":             strings.TrimSpace(req.Capability),
		"result":                 result.Result,
		"ack_level":              result.AckLevel,
		"error_code":             result.ErrorCode,
		"response_summary":       result.ResponseSummary,
	})
}

func (d *DigitalTwin) ensureSessionLocked(sessionID string) *twinSession {
	session := d.sessions[sessionID]
	if session == nil {
		session = &twinSession{targets: make(map[string]*TargetState)}
		d.sessions[sessionID] = session
	}
	return session
}

func bindSnapshotLocked(session *twinSession, snapshotID string) error {
	if session == nil || snapshotID == "" {
		return fmt.Errorf("simulation runtime snapshot is required")
	}
	if session.runtimeSnapshotID == "" {
		session.runtimeSnapshotID = snapshotID
		return nil
	}
	if session.runtimeSnapshotID != snapshotID {
		return fmt.Errorf("simulation session is already bound to runtime snapshot %s", session.runtimeSnapshotID)
	}
	return nil
}

func ensureTargetLocked(session *twinSession, targetRef, logicalType string) *TargetState {
	state := session.targets[targetRef]
	if state == nil {
		observed := true
		verified := true
		state = &TargetState{
			TargetRef:   targetRef,
			LogicalType: logicalType,
			Online:      true,
			StateTruth: TargetStateTruth{
				Version:           DigitalTwinStateContractVersion1,
				Scope:             "SIMULATION_ONLY",
				ObservedOnline:    &observed,
				VerifiedOnline:    &verified,
				Restorable:        false,
				RestorationReason: "checkpoint_not_captured",
			},
		}
		session.targets[targetRef] = state
	} else if state.LogicalType == "" && logicalType != "" {
		state.LogicalType = logicalType
	}
	return state
}

func setOnlineTruth(state *TargetState, online bool, desired bool) {
	if state == nil {
		return
	}
	state.Online = online
	observed := online
	verified := online
	state.StateTruth.Version = DigitalTwinStateContractVersion1
	state.StateTruth.Scope = "SIMULATION_ONLY"
	state.StateTruth.ObservedOnline = &observed
	state.StateTruth.VerifiedOnline = &verified
	if desired {
		desiredValue := online
		state.StateTruth.DesiredOnline = &desiredValue
	}
	state.StateTruth.Restorable = false
	state.StateTruth.RestorationReason = "checkpoint_not_captured"
}

func (d *DigitalTwin) setObservedOnline(sessionID, targetRef string, online bool) {
	d.mu.Lock()
	session := d.ensureSessionLocked(sessionID)
	state := ensureTargetLocked(session, targetRef, "")
	setOnlineTruth(state, online, false)
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

func cloneTargetState(state TargetState) TargetState {
	cloneBool := func(source *bool) *bool {
		if source == nil {
			return nil
		}
		value := *source
		return &value
	}
	state.StateTruth.DesiredOnline = cloneBool(state.StateTruth.DesiredOnline)
	state.StateTruth.ObservedOnline = cloneBool(state.StateTruth.ObservedOnline)
	state.StateTruth.VerifiedOnline = cloneBool(state.StateTruth.VerifiedOnline)
	return state
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
