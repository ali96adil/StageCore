package lightingnode

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CueSetPayload struct {
	Aliases map[string]float64 `json:"aliases"`
}

type CueFadePayload struct {
	FadeMS  int64              `json:"fade_ms"`
	Aliases map[string]float64 `json:"aliases"`
}

func ResolveCueCommandPayload(bindings []ProjectBinding, deviceID, commandType string, raw json.RawMessage) (json.RawMessage, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, fmt.Errorf("lighting Cue target device_id is required")
	}
	switch strings.TrimSpace(commandType) {
	case CommandChannelsSet:
		var payload CueSetPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		channels, err := resolveCueAliases(bindings, deviceID, payload.Aliases)
		if err != nil {
			return nil, err
		}
		return json.Marshal(ChannelsSetPayload{Channels: channels})

	case CommandChannelsFade:
		var payload CueFadePayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		if payload.FadeMS <= 0 || payload.FadeMS > maxFadeMS {
			return nil, fmt.Errorf("fade_ms must be within 1..%d", maxFadeMS)
		}
		channels, err := resolveCueAliases(bindings, deviceID, payload.Aliases)
		if err != nil {
			return nil, err
		}
		return json.Marshal(ChannelsFadePayload{FadeMS: payload.FadeMS, Channels: channels})

	case CommandBlackout:
		return CanonicalCommandPayload(CommandBlackout, raw)

	default:
		return nil, fmt.Errorf("lighting command %q is not Cue-safe", commandType)
	}
}

func resolveCueAliases(bindings []ProjectBinding, deviceID string, aliases map[string]float64) (map[string]float64, error) {
	if len(aliases) == 0 {
		return nil, fmt.Errorf("at least one logical lighting alias is required")
	}
	out := make(map[string]float64, len(aliases))
	for rawAlias, level := range aliases {
		alias := strings.TrimSpace(rawAlias)
		if !logicalAliasPattern.MatchString(alias) {
			return nil, fmt.Errorf("invalid lighting alias %q", alias)
		}
		if !validLevel(level) {
			return nil, fmt.Errorf("lighting alias %q level must be within 0..100", alias)
		}
		resolved, err := ResolveAlias(bindings, alias)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(resolved.DeviceID) != deviceID {
			return nil, fmt.Errorf("lighting alias %q belongs to device %s, not Cue target %s", alias, resolved.DeviceID, deviceID)
		}
		if _, exists := out[resolved.ChannelKey]; exists {
			return nil, fmt.Errorf("lighting alias %q resolves to duplicate channel %q", alias, resolved.ChannelKey)
		}
		out[resolved.ChannelKey] = level
	}
	return out, nil
}
