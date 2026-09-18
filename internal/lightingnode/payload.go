package lightingnode

import (
	"encoding/json"
	"fmt"
	"strings"
)

func CanonicalCommandPayload(commandType string, raw json.RawMessage) (json.RawMessage, error) {
	switch strings.TrimSpace(commandType) {
	case CommandChannelsSet:
		var payload ChannelsSetPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		if err := validateRequestedLevels(payload.Channels); err != nil {
			return nil, err
		}
		return json.Marshal(payload)

	case CommandChannelsFade:
		var payload ChannelsFadePayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		if payload.FadeMS <= 0 || payload.FadeMS > maxFadeMS {
			return nil, fmt.Errorf("fade_ms must be within 1..%d", maxFadeMS)
		}
		if err := validateRequestedLevels(payload.Channels); err != nil {
			return nil, err
		}
		return json.Marshal(payload)

	case CommandBlackout:
		var payload BlackoutPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		if payload.FadeMS < 0 || payload.FadeMS > maxFadeMS {
			return nil, fmt.Errorf("fade_ms must be within 0..%d", maxFadeMS)
		}
		return json.Marshal(payload)

	case CommandStateRead, CommandConfigRead:
		var payload struct{}
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		return json.RawMessage(`{}`), nil

	case CommandIdentify:
		var payload IdentifyPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		payload.ChannelKey = strings.TrimSpace(payload.ChannelKey)
		if !channelKeyPattern.MatchString(payload.ChannelKey) {
			return nil, fmt.Errorf("invalid channel_key %q", payload.ChannelKey)
		}
		if !validLevel(payload.Level) {
			return nil, fmt.Errorf("identify level must be within 0..100")
		}
		if payload.DurationMS < 100 || payload.DurationMS > 10000 {
			return nil, fmt.Errorf("duration_ms must be within 100..10000")
		}
		return json.Marshal(payload)

	case CommandConfigApply:
		var payload ConfigApplyPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		if payload.Configuration.SchemaVersion == 0 {
			payload.Configuration.SchemaVersion = SchemaVersion1
		}
		canonicalConfig, err := CanonicalConfiguration(payload.Configuration)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(canonicalConfig, &payload.Configuration); err != nil {
			return nil, fmt.Errorf("decode canonical configuration: %w", err)
		}
		return json.Marshal(payload)

	default:
		return nil, fmt.Errorf("unsupported lighting command %q", commandType)
	}
}

func validateRequestedLevels(channels map[string]float64) error {
	if len(channels) == 0 {
		return fmt.Errorf("at least one lighting channel level is required")
	}
	for key, level := range channels {
		key = strings.TrimSpace(key)
		if !channelKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid channel_key %q", key)
		}
		if !validLevel(level) {
			return fmt.Errorf("lighting channel %q level must be within 0..100", key)
		}
	}
	return nil
}
