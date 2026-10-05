package stagelaser

import (
	"fmt"
	"strings"
)

const (
	ResultAccepted  = "ACCEPTED"
	ResultRejected  = "REJECTED"
	ResultCompleted = "COMPLETED"
	ResultFailed    = "FAILED"
)

const (
	ErrorNotArmed      = "LASER_NOT_ARMED"
	ErrorUnsafeState   = "LASER_STATE_UNSAFE"
	ErrorStateUnknown  = "LASER_STATE_UNKNOWN"
	ErrorBusy          = "LASER_BUSY"
	ErrorInvalid       = "LASER_COMMAND_INVALID"
)

const commandJournalCapacity = 32

type ResetClass string

const (
	ResetSoftware ResetClass = "SOFTWARE"
	ResetWatchdog ResetClass = "WATCHDOG"
	ResetPowerOn  ResetClass = "POWERON"
	ResetBrownout ResetClass = "BROWNOUT"
	ResetUnknown  ResetClass = "UNKNOWN"
)

type Command struct {
	ID      string
	Type    string
	Flash   *FlashStartPayload
	Resync  *StateResyncPayload
}

type CommandResult struct {
	CommandID string `json:"command_id"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message,omitempty"`
}

type PersistentState struct {
	StableState            LogicalState `json:"stable_state"`
	StateQuality           StateQuality `json:"state_quality"`
	TransitionInProgress   bool         `json:"transition_in_progress"`
	FlashSessionInProgress bool         `json:"flash_session_in_progress"`
}

type pendingPulse struct {
	commandID   string
	commandType string
	target      LogicalState
	finalizeFlash bool
}

type Machine struct {
	armState              ArmState
	logicalState          LogicalState
	stateQuality          StateQuality
	resyncRequired        bool
	pulseInProgress       bool
	relayPulseCount       uint64
	limits                Limits
	activeFlash           *FlashObservation
	lastAcceptedCommandID string
	lastAppliedCommandID  string
	lastCommandType       string
	lastCommandResult     string
	persistent            PersistentState
	pending               *pendingPulse
	journal               map[string]CommandResult
	journalOrder          []string
}

func NewColdPowerUpMachine() *Machine {
	limits := DefaultMechanicalLimits()
	return &Machine{
		armState:     ArmDisarmed,
		logicalState: StateOff,
		stateQuality: StateQualityTracked,
		limits:       limits,
		persistent: PersistentState{
			StableState:  StateOff,
			StateQuality: StateQualityTracked,
		},
		journal: make(map[string]CommandResult),
	}
}

func RestoreMachine(p PersistentState, reset ResetClass, sharedPowerQualified bool) *Machine {
	m := NewColdPowerUpMachine()
	m.persistent = p
	m.armState = ArmDisarmed

	if p.TransitionInProgress || p.FlashSessionInProgress {
		m.enterUnknown()
		return m
	}

	switch reset {
	case ResetSoftware, ResetWatchdog:
		if stableState(p.StableState) && validStateQuality(p.StateQuality) &&
			p.StateQuality != StateQualityUnknown {
			m.logicalState = p.StableState
			m.stateQuality = p.StateQuality
			m.resyncRequired = false
			return m
		}
	case ResetPowerOn, ResetBrownout:
		if sharedPowerQualified {
			m.logicalState = StateOff
			m.stateQuality = StateQualityTracked
			m.resyncRequired = false
			m.persistent = PersistentState{
				StableState:  StateOff,
				StateQuality: StateQualityTracked,
			}
			return m
		}
	}
	m.enterUnknown()
	return m
}

func (m *Machine) Observation() Observation {
	if m == nil {
		return Observation{}
	}
	var active *FlashObservation
	if m.activeFlash != nil {
		copy := *m.activeFlash
		active = &copy
	}
	return Observation{
		SchemaVersion:         SchemaVersion1,
		ControlContractVersion: ControlContractVersion,
		ArmState:              m.armState,
		LogicalState:          m.logicalState,
		StateQuality:          m.stateQuality,
		ResyncRequired:        m.resyncRequired,
		PulseInProgress:       m.pulseInProgress,
		RelayPulseCount:       m.relayPulseCount,
		DriverKind:            DriverMechanicalRelay,
		Limits:                m.limits,
		ActiveFlash:           active,
		LastAcceptedCommandID: m.lastAcceptedCommandID,
		LastAppliedCommandID:  m.lastAppliedCommandID,
		LastCommandType:       m.lastCommandType,
		LastCommandResult:     m.lastCommandResult,
	}
}

func (m *Machine) PersistentState() PersistentState {
	if m == nil {
		return PersistentState{}
	}
	return m.persistent
}

func (m *Machine) Execute(command Command) CommandResult {
	if m == nil {
		return CommandResult{CommandID: command.ID, Status: ResultFailed, ErrorCode: ErrorUnsafeState}
	}
	command.ID = strings.TrimSpace(command.ID)
	command.Type = strings.TrimSpace(command.Type)
	if command.ID == "" || CommandCapability(command.Type) == "" {
		return CommandResult{CommandID: command.ID, Status: ResultRejected, ErrorCode: ErrorInvalid}
	}
	if prior, ok := m.journal[command.ID]; ok {
		return prior
	}

	switch command.Type {
	case CommandArm:
		return m.arm(command)
	case CommandDisarm:
		return m.disarm(command, true)
	case CommandSetOn:
		return m.setOn(command)
	case CommandSetOff:
		return m.setOff(command)
	case CommandFlashStart:
		return m.flashStart(command)
	case CommandFlashStop:
		return m.flashStop(command)
	case CommandSafeOff:
		return m.disarm(command, true)
	case CommandStateRead:
		return m.complete(command, "")
	case CommandStateResync:
		return m.resync(command)
	default:
		return m.reject(command, ErrorInvalid, "unsupported StageLaser command")
	}
}

func (m *Machine) CompletePulse() (CommandResult, error) {
	if m == nil || !m.pulseInProgress || m.pending == nil {
		return CommandResult{}, fmt.Errorf("no StageLaser pulse is in progress")
	}
	pending := *m.pending
	m.pending = nil
	m.pulseInProgress = false
	m.relayPulseCount++
	m.logicalState = pending.target
	m.stateQuality = StateQualityTracked
	m.resyncRequired = false
	m.persistent.TransitionInProgress = false

	if pending.finalizeFlash {
		m.activeFlash = nil
		m.persistent.FlashSessionInProgress = false
		m.logicalState = StateOff
		m.persistStable(StateOff)
	} else if pending.commandType != "" && pending.commandType != CommandFlashStart {
		if stableState(m.logicalState) {
			m.persistStable(m.logicalState)
		}
	}

	if pending.commandID == "" {
		return CommandResult{}, nil
	}

	m.lastAppliedCommandID = pending.commandID
	result := CommandResult{
		CommandID: pending.commandID,
		Status:    ResultCompleted,
		Message:   "output pulse released and logical state committed",
	}
	m.remember(result)
	m.lastCommandResult = result.Status
	return result, nil
}

func (m *Machine) AdvanceFlashPhase() error {
	if m == nil || m.activeFlash == nil {
		return fmt.Errorf("StageLaser flash is not active")
	}
	if m.pulseInProgress {
		return fmt.Errorf("StageLaser pulse is already in progress")
	}
	switch m.logicalState {
	case StateFlashOn:
		m.beginPulse("", "", StateFlashOff, false)
	case StateFlashOff:
		m.beginPulse("", "", StateFlashOn, false)
	default:
		return fmt.Errorf("invalid active flash state %s", m.logicalState)
	}
	return nil
}

func (m *Machine) ExpireFlash() (CommandResult, error) {
	if m == nil || m.activeFlash == nil {
		return CommandResult{}, fmt.Errorf("StageLaser flash is not active")
	}
	if m.pulseInProgress {
		return CommandResult{}, fmt.Errorf("StageLaser pulse is already in progress")
	}
	if m.logicalState == StateFlashOff {
		m.activeFlash = nil
		m.logicalState = StateOff
		m.stateQuality = StateQualityTracked
		m.persistent.FlashSessionInProgress = false
		m.persistStable(StateOff)
		return CommandResult{Status: ResultCompleted, Message: "flash duration expired at OFF"}, nil
	}
	if m.logicalState != StateFlashOn {
		return CommandResult{}, fmt.Errorf("invalid active flash state %s", m.logicalState)
	}
	m.beginPulse("", "", StateOff, true)
	return CommandResult{Status: ResultAccepted, Message: "flash expiry is settling to OFF"}, nil
}

func (m *Machine) arm(command Command) CommandResult {
	if m.busy() {
		return m.reject(command, ErrorBusy, "StageLaser is busy")
	}
	if m.stateQuality == StateQualityUnknown || m.resyncRequired ||
		m.logicalState == StateUnknown || m.logicalState == StateError {
		return m.reject(command, ErrorStateUnknown, "StageLaser state requires resync")
	}
	if m.logicalState != StateOff {
		return m.reject(command, ErrorUnsafeState, "StageLaser must be OFF before ARM")
	}
	m.armState = ArmArmed
	return m.complete(command, "StageLaser armed")
}

func (m *Machine) disarm(command Command, forceDisarm bool) CommandResult {
	if forceDisarm {
		m.armState = ArmDisarmed
	}
	if m.pulseInProgress {
		return m.reject(command, ErrorBusy, "StageLaser pulse is already in progress")
	}
	if m.stateQuality == StateQualityUnknown || m.logicalState == StateUnknown ||
		m.logicalState == StateError {
		m.enterUnknown()
		return m.fail(command, ErrorStateUnknown, "StageLaser is disarmed but physical OFF cannot be proven")
	}
	if m.activeFlash != nil {
		if m.logicalState == StateFlashOff {
			m.activeFlash = nil
			m.logicalState = StateOff
			m.persistent.FlashSessionInProgress = false
			m.persistStable(StateOff)
			return m.complete(command, "flash stopped and StageLaser is OFF")
		}
		if m.logicalState == StateFlashOn {
			return m.acceptPulse(command, StateOff, true)
		}
		return m.fail(command, ErrorUnsafeState, "active flash state is invalid")
	}
	switch m.logicalState {
	case StateOff:
		return m.complete(command, "StageLaser is OFF")
	case StateOn:
		return m.acceptPulse(command, StateOff, false)
	default:
		return m.reject(command, ErrorBusy, "StageLaser is transitioning")
	}
}

func (m *Machine) setOn(command Command) CommandResult {
	if m.armState != ArmArmed {
		return m.reject(command, ErrorNotArmed, "ARM is required before ON")
	}
	if m.busy() {
		return m.reject(command, ErrorBusy, "StageLaser is busy")
	}
	if m.stateQuality == StateQualityUnknown || m.resyncRequired ||
		m.logicalState == StateUnknown || m.logicalState == StateError {
		return m.reject(command, ErrorStateUnknown, "StageLaser state requires resync")
	}
	switch m.logicalState {
	case StateOn:
		return m.complete(command, "StageLaser is already ON")
	case StateOff:
		return m.acceptPulse(command, StateOn, false)
	default:
		return m.reject(command, ErrorBusy, "StageLaser is transitioning")
	}
}

func (m *Machine) setOff(command Command) CommandResult {
	if m.pulseInProgress {
		return m.reject(command, ErrorBusy, "StageLaser pulse is already in progress")
	}
	if m.stateQuality == StateQualityUnknown || m.logicalState == StateUnknown ||
		m.logicalState == StateError {
		return m.fail(command, ErrorStateUnknown, "physical OFF cannot be proven without resync")
	}
	if m.activeFlash != nil {
		return m.flashStop(command)
	}
	switch m.logicalState {
	case StateOff:
		return m.complete(command, "StageLaser is already OFF")
	case StateOn:
		return m.acceptPulse(command, StateOff, false)
	default:
		return m.reject(command, ErrorBusy, "StageLaser is transitioning")
	}
}

func (m *Machine) flashStart(command Command) CommandResult {
	if m.armState != ArmArmed {
		return m.reject(command, ErrorNotArmed, "ARM is required before Flash")
	}
	if m.busy() || m.activeFlash != nil {
		return m.reject(command, ErrorBusy, "StageLaser is busy")
	}
	if m.stateQuality == StateQualityUnknown || m.resyncRequired ||
		m.logicalState != StateOff {
		return m.reject(command, ErrorUnsafeState, "Flash requires a known OFF state")
	}
	if command.Flash == nil {
		return m.reject(command, ErrorInvalid, "flash payload is required")
	}
	if err := ValidateFlashStartPayload(*command.Flash, m.limits); err != nil {
		return m.reject(command, ErrorInvalid, err.Error())
	}
	m.activeFlash = &FlashObservation{
		CommandID:   command.ID,
		FrequencyHz: command.Flash.FrequencyHz,
		DurationMS:  command.Flash.DurationMS,
	}
	m.persistent.FlashSessionInProgress = true
	return m.acceptPulse(command, StateFlashOn, false)
}

func (m *Machine) flashStop(command Command) CommandResult {
	if m.pulseInProgress {
		return m.reject(command, ErrorBusy, "StageLaser pulse is already in progress")
	}
	if m.stateQuality == StateQualityUnknown || m.logicalState == StateUnknown ||
		m.logicalState == StateError {
		return m.fail(command, ErrorStateUnknown, "flash stop cannot prove OFF without resync")
	}
	if m.activeFlash == nil {
		switch m.logicalState {
		case StateOff:
			return m.complete(command, "StageLaser is already OFF")
		case StateOn:
			return m.acceptPulse(command, StateOff, false)
		default:
			return m.reject(command, ErrorBusy, "StageLaser is transitioning")
		}
	}
	switch m.logicalState {
	case StateFlashOff:
		m.activeFlash = nil
		m.logicalState = StateOff
		m.persistent.FlashSessionInProgress = false
		m.persistStable(StateOff)
		return m.complete(command, "flash stopped at OFF")
	case StateFlashOn:
		return m.acceptPulse(command, StateOff, true)
	default:
		return m.reject(command, ErrorBusy, "StageLaser flash is transitioning")
	}
}

func (m *Machine) resync(command Command) CommandResult {
	if m.armState != ArmDisarmed {
		return m.reject(command, ErrorUnsafeState, "resync requires DISARMED")
	}
	if m.busy() || m.activeFlash != nil {
		return m.reject(command, ErrorBusy, "resync requires an idle output")
	}
	if command.Resync == nil {
		return m.reject(command, ErrorInvalid, "resync payload is required")
	}
	if err := ValidateStateResyncPayload(*command.Resync); err != nil {
		return m.reject(command, ErrorInvalid, err.Error())
	}
	m.logicalState = command.Resync.State
	m.stateQuality = StateQualityTracked
	m.resyncRequired = false
	m.persistStable(command.Resync.State)
	return m.complete(command, "StageLaser tracked state resynchronized without output pulse")
}

func (m *Machine) acceptPulse(command Command, target LogicalState, finalizeFlash bool) CommandResult {
	m.beginPulse(command.ID, command.Type, target, finalizeFlash)
	result := CommandResult{
		CommandID: command.ID,
		Status:    ResultAccepted,
		Message:   "output pulse started; state commits after release",
	}
	m.lastAcceptedCommandID = command.ID
	m.lastCommandType = command.Type
	m.lastCommandResult = result.Status
	m.remember(result)
	return result
}

func (m *Machine) beginPulse(commandID, commandType string, target LogicalState, finalizeFlash bool) {
	m.pulseInProgress = true
	m.persistent.TransitionInProgress = true
	if target == StateOn || target == StateFlashOn {
		m.logicalState = StateTurningOn
	} else {
		m.logicalState = StateTurningOff
	}
	m.pending = &pendingPulse{
		commandID: commandID,
		commandType: commandType,
		target: target,
		finalizeFlash: finalizeFlash,
	}
}

func (m *Machine) complete(command Command, message string) CommandResult {
	result := CommandResult{CommandID: command.ID, Status: ResultCompleted, Message: message}
	m.lastAcceptedCommandID = command.ID
	m.lastAppliedCommandID = command.ID
	m.lastCommandType = command.Type
	m.lastCommandResult = result.Status
	m.remember(result)
	return result
}

func (m *Machine) reject(command Command, code, message string) CommandResult {
	result := CommandResult{CommandID: command.ID, Status: ResultRejected, ErrorCode: code, Message: message}
	m.lastCommandType = command.Type
	m.lastCommandResult = result.Status
	m.remember(result)
	return result
}

func (m *Machine) fail(command Command, code, message string) CommandResult {
	result := CommandResult{CommandID: command.ID, Status: ResultFailed, ErrorCode: code, Message: message}
	m.lastCommandType = command.Type
	m.lastCommandResult = result.Status
	m.remember(result)
	return result
}

func (m *Machine) remember(result CommandResult) {
	if result.CommandID == "" {
		return
	}
	if _, exists := m.journal[result.CommandID]; !exists {
		if len(m.journalOrder) == commandJournalCapacity {
			delete(m.journal, m.journalOrder[0])
			m.journalOrder = m.journalOrder[1:]
		}
		m.journalOrder = append(m.journalOrder, result.CommandID)
	}
	m.journal[result.CommandID] = result
}

func (m *Machine) busy() bool {
	return m.pulseInProgress || m.logicalState == StateTurningOn || m.logicalState == StateTurningOff
}

func (m *Machine) persistStable(state LogicalState) {
	m.persistent.StableState = state
	m.persistent.StateQuality = StateQualityTracked
	m.persistent.TransitionInProgress = false
}

func (m *Machine) enterUnknown() {
	m.armState = ArmDisarmed
	m.logicalState = StateUnknown
	m.stateQuality = StateQualityUnknown
	m.resyncRequired = true
	m.pulseInProgress = false
	m.activeFlash = nil
	m.pending = nil
	m.persistent = PersistentState{
		StableState:            StateUnknown,
		StateQuality:           StateQualityUnknown,
		TransitionInProgress:   false,
		FlashSessionInProgress: false,
	}
}

func stableState(state LogicalState) bool {
	return state == StateOff || state == StateOn
}
