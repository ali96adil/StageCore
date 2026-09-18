package lightingnode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
)

const maxFadeMS int64 = 600000

type Submission struct {
	Result     contracts.CommandResult
	Superseded *contracts.CommandResult
}

type activeFade struct {
	commandID string
	startedAt time.Time
	endsAt    time.Time
	from      map[string]float64
	targets   map[string]float64
}

type Simulator struct {
	config      Configuration
	now         func() time.Time
	connected   bool
	levels      map[string]float64
	seen        map[string]contracts.CommandResult
	executions  map[string]int
	active      *activeFade
	lastAccepted string
	lastApplied  string
}

func NewSimulator(config Configuration, now func() time.Time) (*Simulator, error) {
	if config.SchemaVersion == 0 {
		config.SchemaVersion = SchemaVersion1
	}
	if err := ValidateConfiguration(config); err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &Simulator{
		config:     config,
		now:        now,
		connected:  true,
		levels:     BlackoutLevels(config),
		seen:       make(map[string]contracts.CommandResult),
		executions: make(map[string]int),
	}, nil
}

func (s *Simulator) Disconnect() {
	if s != nil {
		s.connected = false
	}
}

func (s *Simulator) Reconnect() {
	if s != nil {
		s.connected = true
	}
}

func (s *Simulator) ExecutionCount(commandID string) int {
	if s == nil {
		return 0
	}
	return s.executions[strings.TrimSpace(commandID)]
}

func (s *Simulator) CurrentLevels() map[string]float64 {
	if s == nil {
		return nil
	}
	return copyLevels(s.levels)
}

func (s *Simulator) Configuration() Configuration {
	if s == nil {
		return Configuration{}
	}
	out := s.config
	out.Channels = append([]ChannelConfig(nil), s.config.Channels...)
	return out
}

func (s *Simulator) Observation() Observation {
	obs := Observation{
		SchemaVersion:         SchemaVersion1,
		CurrentLevels:         s.CurrentLevels(),
		LastAcceptedCommandID: s.lastAccepted,
		LastAppliedCommandID:  s.lastApplied,
		DMXHealthy:            true,
		Authority:             AuthorityStageCore,
	}
	if s.active != nil {
		obs.ActiveFade = &FadeObservation{
			CommandID: s.active.commandID,
			StartedAt: s.active.startedAt.UTC().Format(time.RFC3339Nano),
			EndsAt:    s.active.endsAt.UTC().Format(time.RFC3339Nano),
			Targets:   copyLevels(s.active.targets),
		}
	}
	return obs
}

func (s *Simulator) Submit(command contracts.CommandEnvelope) Submission {
	command.CommandID = strings.TrimSpace(command.CommandID)
	command.CommandType = strings.TrimSpace(command.CommandType)
	if command.CommandID == "" {
		return Submission{Result: resultWithError("", contracts.CommandRejected, "COMMAND_ID_REQUIRED", "VALIDATION", "command_id is required")}
	}
	if previous, ok := s.seen[command.CommandID]; ok {
		return Submission{Result: previous}
	}

	now := s.now().UTC()
	if command.DeadlineAt != nil && !command.DeadlineAt.UTC().After(now) {
		result := resultWithError(command.CommandID, contracts.CommandRejected, "COMMAND_EXPIRED", "TIMEOUT", "command deadline has expired")
		s.seen[command.CommandID] = result
		return Submission{Result: result}
	}
	if !s.connected {
		result := resultWithError(command.CommandID, contracts.CommandFailed, "NODE_OFFLINE", "NETWORK", "simulated lighting node is disconnected")
		s.seen[command.CommandID] = result
		return Submission{Result: result}
	}
	if CommandCapability(command.CommandType) == "" {
		result := resultWithError(command.CommandID, contracts.CommandRejected, "COMMAND_UNSUPPORTED", "VALIDATION", "unsupported lighting command")
		s.seen[command.CommandID] = result
		return Submission{Result: result}
	}

	s.executions[command.CommandID]++

	switch command.CommandType {
	case CommandChannelsSet:
		var payload ChannelsSetPayload
		if err := decodeStrict(command.Payload, &payload); err != nil {
			return s.reject(command.CommandID, "PAYLOAD_INVALID", err)
		}
		levels, err := NormalizeLevels(s.config, payload.Channels)
		if err != nil {
			return s.reject(command.CommandID, "CHANNEL_LEVEL_INVALID", err)
		}
		superseded := s.cancelActive("superseded by immediate channel set")
		for key, value := range levels {
			s.levels[key] = value
		}
		s.lastAccepted = command.CommandID
		s.lastApplied = command.CommandID
		result := completedResult(command.CommandID, map[string]any{"levels": levels})
		s.seen[command.CommandID] = result
		return Submission{Result: result, Superseded: superseded}

	case CommandChannelsFade:
		var payload ChannelsFadePayload
		if err := decodeStrict(command.Payload, &payload); err != nil {
			return s.reject(command.CommandID, "PAYLOAD_INVALID", err)
		}
		if payload.FadeMS <= 0 || payload.FadeMS > maxFadeMS {
			return s.reject(command.CommandID, "FADE_DURATION_INVALID", fmt.Errorf("fade_ms must be within 1..%d", maxFadeMS))
		}
		targets, err := NormalizeLevels(s.config, payload.Channels)
		if err != nil {
			return s.reject(command.CommandID, "CHANNEL_LEVEL_INVALID", err)
		}
		superseded := s.cancelActive("superseded by newer fade")
		s.active = &activeFade{
			commandID: command.CommandID,
			startedAt: now,
			endsAt:    now.Add(time.Duration(payload.FadeMS) * time.Millisecond),
			from:      selectedLevels(s.levels, targets),
			targets:   targets,
		}
		s.lastAccepted = command.CommandID
		result := contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandAccepted}
		s.seen[command.CommandID] = result
		return Submission{Result: result, Superseded: superseded}

	case CommandBlackout:
		var payload BlackoutPayload
		if err := decodeStrict(command.Payload, &payload); err != nil {
			return s.reject(command.CommandID, "PAYLOAD_INVALID", err)
		}
		if payload.FadeMS < 0 || payload.FadeMS > maxFadeMS {
			return s.reject(command.CommandID, "FADE_DURATION_INVALID", fmt.Errorf("fade_ms must be within 0..%d", maxFadeMS))
		}
		targets := BlackoutLevels(s.config)
		superseded := s.cancelActive("superseded by blackout")
		s.lastAccepted = command.CommandID
		if payload.FadeMS == 0 {
			for key := range targets {
				s.levels[key] = 0
			}
			s.lastApplied = command.CommandID
			result := completedResult(command.CommandID, map[string]any{"blackout": true, "fade_ms": 0})
			s.seen[command.CommandID] = result
			return Submission{Result: result, Superseded: superseded}
		}
		s.active = &activeFade{
			commandID: command.CommandID,
			startedAt: now,
			endsAt:    now.Add(time.Duration(payload.FadeMS) * time.Millisecond),
			from:      selectedLevels(s.levels, targets),
			targets:   targets,
		}
		result := contracts.CommandResult{CommandID: command.CommandID, Status: contracts.CommandAccepted}
		s.seen[command.CommandID] = result
		return Submission{Result: result, Superseded: superseded}

	case CommandStateRead:
		if err := decodeStrict(command.Payload, &struct{}{}); err != nil {
			return s.reject(command.CommandID, "PAYLOAD_INVALID", err)
		}
		s.lastAccepted = command.CommandID
		s.lastApplied = command.CommandID
		result := completedResult(command.CommandID, s.Observation())
		s.seen[command.CommandID] = result
		return Submission{Result: result}

	case CommandIdentify:
		var payload IdentifyPayload
		if err := decodeStrict(command.Payload, &payload); err != nil {
			return s.reject(command.CommandID, "PAYLOAD_INVALID", err)
		}
		channel, ok := ChannelIndex(s.config)[strings.TrimSpace(payload.ChannelKey)]
		if !ok || !channel.Enabled || channel.Kind == ChannelUnused {
			return s.reject(command.CommandID, "CHANNEL_INVALID", fmt.Errorf("identify channel is unavailable"))
		}
		if !validLevel(payload.Level) || payload.DurationMS < 100 || payload.DurationMS > 10000 {
			return s.reject(command.CommandID, "IDENTIFY_INVALID", fmt.Errorf("identify requires level 0..100 and duration_ms 100..10000"))
		}
		s.lastAccepted = command.CommandID
		s.lastApplied = command.CommandID
		result := completedResult(command.CommandID, map[string]any{
			"channel_key": channel.ChannelKey,
			"level":       payload.Level,
			"duration_ms": payload.DurationMS,
			"simulated":   true,
		})
		s.seen[command.CommandID] = result
		return Submission{Result: result}

	case CommandConfigRead:
		if err := decodeStrict(command.Payload, &struct{}{}); err != nil {
			return s.reject(command.CommandID, "PAYLOAD_INVALID", err)
		}
		s.lastAccepted = command.CommandID
		s.lastApplied = command.CommandID
		result := completedResult(command.CommandID, s.Configuration())
		s.seen[command.CommandID] = result
		return Submission{Result: result}

	case CommandConfigApply:
		var payload ConfigApplyPayload
		if err := decodeStrict(command.Payload, &payload); err != nil {
			return s.reject(command.CommandID, "PAYLOAD_INVALID", err)
		}
		if payload.Configuration.SchemaVersion == 0 {
			payload.Configuration.SchemaVersion = SchemaVersion1
		}
		if err := ValidateConfiguration(payload.Configuration); err != nil {
			return s.reject(command.CommandID, "CONFIGURATION_INVALID", err)
		}
		superseded := s.cancelActive("superseded by configuration apply")
		s.config = payload.Configuration
		s.levels = BlackoutLevels(s.config)
		s.lastAccepted = command.CommandID
		s.lastApplied = command.CommandID
		result := completedResult(command.CommandID, map[string]any{"configuration_applied": true, "safe_state": "BLACKOUT"})
		s.seen[command.CommandID] = result
		return Submission{Result: result, Superseded: superseded}
	}

	panic("validated lighting command fell through switch")
}

func (s *Simulator) Advance(at time.Time) *contracts.CommandResult {
	if s == nil || s.active == nil {
		return nil
	}
	at = at.UTC()
	fade := s.active
	duration := fade.endsAt.Sub(fade.startedAt)
	var fraction float64
	switch {
	case !at.After(fade.startedAt):
		fraction = 0
	case !at.Before(fade.endsAt):
		fraction = 1
	default:
		fraction = float64(at.Sub(fade.startedAt)) / float64(duration)
	}
	for key, target := range fade.targets {
		start := fade.from[key]
		s.levels[key] = start + (target-start)*fraction
	}
	if fraction < 1 {
		return nil
	}
	result := completedResult(fade.commandID, map[string]any{"levels": copyLevels(fade.targets)})
	s.seen[fade.commandID] = result
	s.lastApplied = fade.commandID
	s.active = nil
	return &result
}

func (s *Simulator) reject(commandID, code string, err error) Submission {
	result := resultWithError(commandID, contracts.CommandRejected, code, "VALIDATION", err.Error())
	s.seen[commandID] = result
	return Submission{Result: result}
}

func (s *Simulator) cancelActive(reason string) *contracts.CommandResult {
	if s.active == nil {
		return nil
	}
	commandID := s.active.commandID
	result := resultWithError(commandID, contracts.CommandCancelled, "COMMAND_SUPERSEDED", "CANCELLED", reason)
	s.seen[commandID] = result
	s.active = nil
	return &result
}

func completedResult(commandID string, payload any) contracts.CommandResult {
	raw, _ := json.Marshal(payload)
	return contracts.CommandResult{CommandID: commandID, Status: contracts.CommandCompleted, Payload: raw}
}

func resultWithError(commandID string, status contracts.CommandStatus, code, category, message string) contracts.CommandResult {
	return contracts.CommandResult{
		CommandID: commandID,
		Status:    status,
		Error: &contracts.ContractError{
			ErrorCode: code,
			Category:  category,
			Message:   message,
			Retryable: false,
		},
	}
}

func decodeStrict(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return fmt.Errorf("multiple JSON values are not allowed: %w", err)
	}
	return nil
}

func selectedLevels(current, targets map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(targets))
	for key := range targets {
		out[key] = current[key]
	}
	return out
}

func copyLevels(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
