package lightingnode

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

const (
	SchemaVersion1 = 1
	MaxChannels    = 12
)

const (
	CapabilityChannelsSet  = "lighting.channels.set"
	CapabilityChannelsFade = "lighting.channels.fade"
	CapabilityBlackout     = "lighting.blackout"
	CapabilityStateRead    = "lighting.state.read"
	CapabilityIdentify     = "lighting.identify"
	CapabilityConfigRead   = "lighting.config.read"
	CapabilityConfigApply  = "lighting.config.apply"
)

const (
	CommandChannelsSet  = "LIGHTING_CHANNELS_SET"
	CommandChannelsFade = "LIGHTING_CHANNELS_FADE"
	CommandBlackout     = "LIGHTING_BLACKOUT"
	CommandStateRead    = "LIGHTING_STATE_READ"
	CommandIdentify     = "LIGHTING_IDENTIFY"
	CommandConfigRead   = "LIGHTING_CONFIG_READ"
	CommandConfigApply  = "LIGHTING_CONFIG_APPLY"
)

type ChannelKind string

const (
	ChannelDimmer    ChannelKind = "DIMMER"
	ChannelWarmWhite ChannelKind = "WARM_WHITE"
	ChannelColdWhite ChannelKind = "COLD_WHITE"
	ChannelRed       ChannelKind = "RED"
	ChannelGreen     ChannelKind = "GREEN"
	ChannelBlue      ChannelKind = "BLUE"
	ChannelUnused    ChannelKind = "UNUSED"
)

type AuthoritySource string

const (
	AuthorityStageCore AuthoritySource = "STAGECORE"
	AuthorityLocalWeb  AuthoritySource = "LOCAL_WEB"
	AuthorityFailsafe  AuthoritySource = "FAILSAFE"
)

type ChannelConfig struct {
	ChannelKey   string      `json:"channel_key"`
	ChannelNumber int         `json:"channel_number"`
	DisplayName  string       `json:"display_name"`
	Kind         ChannelKind  `json:"kind"`
	PhysicalZone string       `json:"physical_zone,omitempty"`
	MinimumLevel float64      `json:"minimum_level"`
	MaximumLevel float64      `json:"maximum_level"`
	Inverted     bool         `json:"inverted"`
	Enabled      bool         `json:"enabled"`
}

type Configuration struct {
	SchemaVersion int             `json:"schema_version"`
	Channels      []ChannelConfig `json:"channels"`
}

type ChannelsSetPayload struct {
	Channels map[string]float64 `json:"channels"`
}

type ChannelsFadePayload struct {
	FadeMS   int64              `json:"fade_ms"`
	Channels map[string]float64 `json:"channels"`
}

type BlackoutPayload struct {
	FadeMS int64 `json:"fade_ms,omitempty"`
}

type IdentifyPayload struct {
	ChannelKey string  `json:"channel_key"`
	Level      float64 `json:"level"`
	DurationMS int64   `json:"duration_ms"`
}

type ConfigApplyPayload struct {
	Configuration Configuration `json:"configuration"`
}

type FadeObservation struct {
	CommandID string             `json:"command_id"`
	StartedAt string             `json:"started_at"`
	EndsAt    string             `json:"ends_at"`
	Targets   map[string]float64 `json:"targets"`
}

type Observation struct {
	SchemaVersion          int               `json:"schema_version"`
	FirmwareVersion        string            `json:"firmware_version,omitempty"`
	UptimeSeconds          int64             `json:"uptime_seconds,omitempty"`
	ResetReason            string            `json:"reset_reason,omitempty"`
	WiFiRSSI               *int              `json:"wifi_rssi_dbm,omitempty"`
	CurrentLevels          map[string]float64 `json:"current_levels"`
	ActiveFade             *FadeObservation  `json:"active_fade,omitempty"`
	LastAcceptedCommandID  string            `json:"last_accepted_command_id,omitempty"`
	LastAppliedCommandID   string            `json:"last_applied_command_id,omitempty"`
	DMXHealthy             bool              `json:"dmx_healthy"`
	ConfigurationHash      string            `json:"configuration_hash,omitempty"`
	BrownoutWarning        bool              `json:"brownout_warning,omitempty"`
	Authority              AuthoritySource   `json:"authority"`
}

var channelKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func CapabilityKeys() []string {
	return []string{
		CapabilityChannelsSet,
		CapabilityChannelsFade,
		CapabilityBlackout,
		CapabilityStateRead,
		CapabilityIdentify,
		CapabilityConfigRead,
		CapabilityConfigApply,
	}
}

func CommandCapability(commandType string) string {
	switch strings.TrimSpace(commandType) {
	case CommandChannelsSet:
		return CapabilityChannelsSet
	case CommandChannelsFade:
		return CapabilityChannelsFade
	case CommandBlackout:
		return CapabilityBlackout
	case CommandStateRead:
		return CapabilityStateRead
	case CommandIdentify:
		return CapabilityIdentify
	case CommandConfigRead:
		return CapabilityConfigRead
	case CommandConfigApply:
		return CapabilityConfigApply
	default:
		return ""
	}
}

func ValidateConfiguration(config Configuration) error {
	if config.SchemaVersion == 0 {
		config.SchemaVersion = SchemaVersion1
	}
	if config.SchemaVersion != SchemaVersion1 {
		return fmt.Errorf("unsupported lighting configuration schema version %d", config.SchemaVersion)
	}
	if len(config.Channels) == 0 || len(config.Channels) > MaxChannels {
		return fmt.Errorf("lighting configuration requires 1..%d channels", MaxChannels)
	}

	keys := make(map[string]struct{}, len(config.Channels))
	numbers := make(map[int]struct{}, len(config.Channels))
	for _, channel := range config.Channels {
		key := strings.TrimSpace(channel.ChannelKey)
		if !channelKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid channel_key %q", channel.ChannelKey)
		}
		if _, exists := keys[key]; exists {
			return fmt.Errorf("duplicate channel_key %q", key)
		}
		keys[key] = struct{}{}

		if channel.ChannelNumber < 1 || channel.ChannelNumber > MaxChannels {
			return fmt.Errorf("channel_number %d outside 1..%d", channel.ChannelNumber, MaxChannels)
		}
		if _, exists := numbers[channel.ChannelNumber]; exists {
			return fmt.Errorf("duplicate channel_number %d", channel.ChannelNumber)
		}
		numbers[channel.ChannelNumber] = struct{}{}

		if !validChannelKind(channel.Kind) {
			return fmt.Errorf("unsupported channel kind %q", channel.Kind)
		}
		if strings.TrimSpace(channel.DisplayName) == "" {
			return fmt.Errorf("channel %q requires display_name", key)
		}
		if !validLevel(channel.MinimumLevel) || !validLevel(channel.MaximumLevel) || channel.MinimumLevel > channel.MaximumLevel {
			return fmt.Errorf("channel %q requires 0..100 minimum/maximum with minimum <= maximum", key)
		}
		if channel.Kind == ChannelUnused && channel.Enabled {
			return fmt.Errorf("channel %q cannot be enabled with UNUSED kind", key)
		}
	}
	return nil
}

func NormalizeLevels(config Configuration, requested map[string]float64) (map[string]float64, error) {
	if err := ValidateConfiguration(config); err != nil {
		return nil, err
	}
	if len(requested) == 0 {
		return nil, errors.New("at least one channel level is required")
	}
	index := ChannelIndex(config)
	out := make(map[string]float64, len(requested))
	for key, level := range requested {
		key = strings.TrimSpace(key)
		channel, ok := index[key]
		if !ok {
			return nil, fmt.Errorf("unknown lighting channel %q", key)
		}
		if !channel.Enabled || channel.Kind == ChannelUnused {
			return nil, fmt.Errorf("lighting channel %q is disabled", key)
		}
		if !validLevel(level) {
			return nil, fmt.Errorf("lighting channel %q level must be within 0..100", key)
		}
		out[key] = clamp(level, channel.MinimumLevel, channel.MaximumLevel)
	}
	return out, nil
}

func ChannelIndex(config Configuration) map[string]ChannelConfig {
	out := make(map[string]ChannelConfig, len(config.Channels))
	for _, channel := range config.Channels {
		out[strings.TrimSpace(channel.ChannelKey)] = channel
	}
	return out
}

func BlackoutLevels(config Configuration) map[string]float64 {
	out := make(map[string]float64)
	for _, channel := range config.Channels {
		if channel.Enabled && channel.Kind != ChannelUnused {
			out[channel.ChannelKey] = 0
		}
	}
	return out
}

func LevelToDMX(channel ChannelConfig, logicalLevel float64) (uint8, error) {
	if !validLevel(logicalLevel) {
		return 0, fmt.Errorf("logical level must be within 0..100")
	}
	if !validLevel(channel.MinimumLevel) || !validLevel(channel.MaximumLevel) || channel.MinimumLevel > channel.MaximumLevel {
		return 0, fmt.Errorf("invalid channel limits")
	}
	level := clamp(logicalLevel, channel.MinimumLevel, channel.MaximumLevel)
	value := int(math.Round((level / 100.0) * 255.0))
	if channel.Inverted {
		value = 255 - value
	}
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}
	return uint8(value), nil
}

func CanonicalConfiguration(config Configuration) ([]byte, error) {
	if config.SchemaVersion == 0 {
		config.SchemaVersion = SchemaVersion1
	}
	if err := ValidateConfiguration(config); err != nil {
		return nil, err
	}
	copied := append([]ChannelConfig(nil), config.Channels...)
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].ChannelNumber < copied[j].ChannelNumber
	})
	config.Channels = copied
	return json.Marshal(config)
}

func validChannelKind(kind ChannelKind) bool {
	switch kind {
	case ChannelDimmer, ChannelWarmWhite, ChannelColdWhite, ChannelRed, ChannelGreen, ChannelBlue, ChannelUnused:
		return true
	default:
		return false
	}
}

func validLevel(level float64) bool {
	return !math.IsNaN(level) && !math.IsInf(level, 0) && level >= 0 && level <= 100
}

func clamp(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
