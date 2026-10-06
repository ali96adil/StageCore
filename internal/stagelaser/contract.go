package stagelaser

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const (
	SchemaVersion1            = 1
	ProfileID                 = "stagecore.esp32-stagelaser"
	StageDeviceProtocolVersion = "stagecore.device/2"
	ControlContractVersion    = "stagecore.stagelaser/1"
	LogicalTargetType         = "stage_device"
)

const (
	CapabilityArm         = "laser.arm"
	CapabilityDisarm      = "laser.disarm"
	CapabilityStateSet    = "laser.state.set"
	CapabilityFlashStart  = "laser.flash.start"
	CapabilityFlashStop   = "laser.flash.stop"
	CapabilitySafeOff     = "laser.safe_off"
	CapabilityStateRead   = "laser.state.read"
	CapabilityStateResync = "laser.state.resync"
)

const (
	CommandArm         = "LASER_ARM"
	CommandDisarm      = "LASER_DISARM"
	CommandSetOn       = "LASER_SET_ON"
	CommandSetOff      = "LASER_SET_OFF"
	CommandFlashStart  = "LASER_FLASH_START"
	CommandFlashStop   = "LASER_FLASH_STOP"
	CommandSafeOff     = "LASER_SAFE_OFF"
	CommandStateRead   = "LASER_STATE_READ"
	CommandStateResync = "LASER_STATE_RESYNC"
)

type ArmState string

const (
	ArmDisarmed ArmState = "DISARMED"
	ArmArmed    ArmState = "ARMED"
)

type LogicalState string

const (
	StateOff        LogicalState = "OFF"
	StateTurningOn  LogicalState = "TURNING_ON"
	StateOn         LogicalState = "ON"
	StateTurningOff LogicalState = "TURNING_OFF"
	StateFlashOn    LogicalState = "FLASH_ON"
	StateFlashOff   LogicalState = "FLASH_OFF"
	StateUnknown    LogicalState = "UNKNOWN"
	StateError      LogicalState = "ERROR"
)

type StateQuality string

const (
	StateQualityTracked   StateQuality = "TRACKED"
	StateQualityConfirmed StateQuality = "CONFIRMED"
	StateQualityUnknown   StateQuality = "UNKNOWN"
)

type DriverKind string

const (
	DriverMechanicalRelay DriverKind = "MECHANICAL_RELAY"
	DriverElectronic      DriverKind = "ELECTRONIC_SWITCH"
)

type Limits struct {
	PulseMS            int64   `json:"pulse_ms"`
	MinimumRestMS      int64   `json:"minimum_rest_ms"`
	MinimumFlashHz     float64 `json:"minimum_flash_hz"`
	MaximumFlashHz     float64 `json:"maximum_flash_hz"`
	MaximumDurationMS  int64   `json:"maximum_duration_ms"`
}

func DefaultMechanicalLimits() Limits {
	return Limits{
		PulseMS:           180,
		MinimumRestMS:     250,
		MinimumFlashHz:    0.1,
		MaximumFlashHz:    1.0,
		MaximumDurationMS: 60_000,
	}
}

type FlashStartPayload struct {
	FrequencyHz float64 `json:"frequency_hz"`
	DurationMS  int64   `json:"duration_ms"`
}

type StateResyncPayload struct {
	State LogicalState `json:"state"`
}

type FlashObservation struct {
	CommandID   string  `json:"command_id"`
	FrequencyHz float64 `json:"frequency_hz"`
	DurationMS  int64   `json:"duration_ms"`
	StartedAt   string  `json:"started_at,omitempty"`
	EndsAt      string  `json:"ends_at,omitempty"`
}

type Observation struct {
	SchemaVersion         int               `json:"schema_version"`
	FirmwareVersion       string            `json:"firmware_version,omitempty"`
	ControlContractVersion string            `json:"control_contract_version,omitempty"`
	BootID                string            `json:"boot_id,omitempty"`
	UptimeSeconds         int64             `json:"uptime_seconds,omitempty"`
	ResetReason           string            `json:"reset_reason,omitempty"`
	WiFiRSSI              *int              `json:"wifi_rssi_dbm,omitempty"`
	IPAddress             string            `json:"ip_address,omitempty"`
	ArmState              ArmState          `json:"arm_state"`
	LogicalState          LogicalState      `json:"logical_state"`
	StateQuality          StateQuality      `json:"state_quality"`
	ResyncRequired        bool              `json:"resync_required"`
	PulseInProgress       bool              `json:"pulse_in_progress"`
	RelayPulseCount       uint64            `json:"relay_pulse_count"`
	DriverKind            DriverKind        `json:"driver_kind,omitempty"`
	Limits                Limits            `json:"limits"`
	ActiveFlash           *FlashObservation `json:"active_flash,omitempty"`
	LastAcceptedCommandID string            `json:"last_accepted_command_id,omitempty"`
	LastAppliedCommandID  string            `json:"last_applied_command_id,omitempty"`
	LastCommandType       string            `json:"last_command_type,omitempty"`
	LastCommandResult     string            `json:"last_command_result,omitempty"`
}

func CapabilityKeys() []string {
	return []string{
		CapabilityArm,
		CapabilityDisarm,
		CapabilityStateSet,
		CapabilityFlashStart,
		CapabilityFlashStop,
		CapabilitySafeOff,
		CapabilityStateRead,
		CapabilityStateResync,
	}
}

func CommandCapability(commandType string) string {
	switch strings.TrimSpace(commandType) {
	case CommandArm:
		return CapabilityArm
	case CommandDisarm:
		return CapabilityDisarm
	case CommandSetOn, CommandSetOff:
		return CapabilityStateSet
	case CommandFlashStart:
		return CapabilityFlashStart
	case CommandFlashStop:
		return CapabilityFlashStop
	case CommandSafeOff:
		return CapabilitySafeOff
	case CommandStateRead:
		return CapabilityStateRead
	case CommandStateResync:
		return CapabilityStateResync
	default:
		return ""
	}
}

func CueSafeCommand(commandType string) bool {
	switch strings.TrimSpace(commandType) {
	case CommandArm, CommandDisarm, CommandSetOn, CommandSetOff,
		CommandFlashStart, CommandFlashStop, CommandSafeOff:
		return true
	default:
		return false
	}
}

func ValidateLimits(limits Limits) error {
	if limits.PulseMS <= 0 || limits.PulseMS > 10_000 {
		return fmt.Errorf("pulse_ms must be within 1..10000")
	}
	if limits.MinimumRestMS < 0 || limits.MinimumRestMS > 60_000 {
		return fmt.Errorf("minimum_rest_ms must be within 0..60000")
	}
	if !finitePositive(limits.MinimumFlashHz) || !finitePositive(limits.MaximumFlashHz) ||
		limits.MinimumFlashHz > limits.MaximumFlashHz {
		return fmt.Errorf("flash frequency limits must be finite, positive and ordered")
	}
	if limits.MaximumDurationMS <= 0 || limits.MaximumDurationMS > 3_600_000 {
		return fmt.Errorf("maximum_duration_ms must be within 1..3600000")
	}
	return nil
}

func ValidateFlashStartPayload(payload FlashStartPayload, limits Limits) error {
	if err := ValidateLimits(limits); err != nil {
		return err
	}
	if math.IsNaN(payload.FrequencyHz) || math.IsInf(payload.FrequencyHz, 0) ||
		payload.FrequencyHz < limits.MinimumFlashHz || payload.FrequencyHz > limits.MaximumFlashHz {
		return fmt.Errorf(
			"frequency_hz must be within %.3f..%.3f",
			limits.MinimumFlashHz,
			limits.MaximumFlashHz,
		)
	}
	if payload.DurationMS <= 0 || payload.DurationMS > limits.MaximumDurationMS {
		return fmt.Errorf("duration_ms must be within 1..%d", limits.MaximumDurationMS)
	}
	return nil
}

func ValidateStateResyncPayload(payload StateResyncPayload) error {
	switch payload.State {
	case StateOff, StateOn:
		return nil
	default:
		return fmt.Errorf("state resync accepts only OFF or ON")
	}
}

func ValidateObservation(observation Observation) error {
	if observation.SchemaVersion != SchemaVersion1 {
		return fmt.Errorf("unsupported StageLaser observation schema version %d", observation.SchemaVersion)
	}
	if !validArmState(observation.ArmState) {
		return fmt.Errorf("invalid arm_state %q", observation.ArmState)
	}
	if !validLogicalState(observation.LogicalState) {
		return fmt.Errorf("invalid logical_state %q", observation.LogicalState)
	}
	if !validStateQuality(observation.StateQuality) {
		return fmt.Errorf("invalid state_quality %q", observation.StateQuality)
	}
	if observation.StateQuality == StateQualityConfirmed && observation.ResyncRequired {
		return fmt.Errorf("confirmed state cannot require resync")
	}
	if observation.PulseInProgress &&
		observation.LogicalState != StateTurningOn &&
		observation.LogicalState != StateTurningOff {
		return fmt.Errorf("pulse_in_progress requires TURNING_ON or TURNING_OFF")
	}
	if observation.ResyncRequired && observation.StateQuality != StateQualityUnknown {
		return fmt.Errorf("resync_required requires UNKNOWN state quality")
	}
	if observation.ResyncRequired && observation.LogicalState != StateUnknown &&
		observation.LogicalState != StateError {
		return fmt.Errorf("resync_required requires UNKNOWN or ERROR logical state")
	}
	if err := ValidateLimits(observation.Limits); err != nil {
		return fmt.Errorf("invalid StageLaser limits: %w", err)
	}
	return nil
}

func CanonicalCommandPayload(commandType string, raw json.RawMessage) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = CanonicalEmptyPayload()
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("StageLaser command payload must be a JSON object")
	}

	switch strings.TrimSpace(commandType) {
	case CommandArm, CommandDisarm, CommandSetOn, CommandSetOff,
		CommandFlashStop, CommandSafeOff, CommandStateRead:
		if len(object) != 0 {
			return nil, fmt.Errorf("%s does not accept parameters", commandType)
		}
		return CanonicalEmptyPayload(), nil
	case CommandFlashStart:
		if len(object) != 2 {
			return nil, fmt.Errorf("%s requires only frequency_hz and duration_ms", commandType)
		}
		frequency, ok := object["frequency_hz"].(float64)
		if !ok {
			return nil, fmt.Errorf("frequency_hz must be numeric")
		}
		durationNumber, ok := object["duration_ms"].(float64)
		if !ok || math.IsNaN(durationNumber) || math.IsInf(durationNumber, 0) ||
			durationNumber != math.Trunc(durationNumber) ||
			durationNumber < 1 || durationNumber > float64(DefaultMechanicalLimits().MaximumDurationMS) {
			return nil, fmt.Errorf("duration_ms must be an integer within the StageLaser limit")
		}
		payload := FlashStartPayload{FrequencyHz: frequency, DurationMS: int64(durationNumber)}
		if err := ValidateFlashStartPayload(payload, DefaultMechanicalLimits()); err != nil {
			return nil, err
		}
		return json.Marshal(payload)
	case CommandStateResync:
		if len(object) != 1 {
			return nil, fmt.Errorf("%s requires only state", commandType)
		}
		state, ok := object["state"].(string)
		if !ok {
			return nil, fmt.Errorf("state must be OFF or ON")
		}
		payload := StateResyncPayload{State: LogicalState(strings.ToUpper(strings.TrimSpace(state)))}
		if err := ValidateStateResyncPayload(payload); err != nil {
			return nil, err
		}
		return json.Marshal(payload)
	default:
		return nil, fmt.Errorf("unsupported StageLaser command %q", commandType)
	}
}

func CanonicalEmptyPayload() json.RawMessage {
	return json.RawMessage(`{}`)
}

func validArmState(state ArmState) bool {
	switch state {
	case ArmDisarmed, ArmArmed:
		return true
	default:
		return false
	}
}

func validLogicalState(state LogicalState) bool {
	switch state {
	case StateOff, StateTurningOn, StateOn, StateTurningOff,
		StateFlashOn, StateFlashOff, StateUnknown, StateError:
		return true
	default:
		return false
	}
}

func validStateQuality(quality StateQuality) bool {
	switch quality {
	case StateQualityTracked, StateQualityConfirmed, StateQualityUnknown:
		return true
	default:
		return false
	}
}

func finitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
