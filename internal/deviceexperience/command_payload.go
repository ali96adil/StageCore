package deviceexperience

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func canonicalCommandPayload(commandType string, raw json.RawMessage, now time.Time) (json.RawMessage, error) {
	payload := normalizeJSON(raw, `{}`)
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%w: command payload must be a JSON object", ErrInvalidState)
	}

	switch strings.TrimSpace(commandType) {
	case "DISPLAY_MESSAGE":
		message, _ := object["message"].(string)
		message = strings.TrimSpace(message)
		if message == "" || len([]rune(message)) > 240 {
			return nil, fmt.Errorf("%w: DISPLAY_MESSAGE requires message with 1..240 characters", ErrInvalidState)
		}
		object["message"] = message
		if category, ok := optionalEnum(object["category"], []string{"INFO", "STANDBY", "PLACES", "SHOW_START", "WARNING"}, ""); ok {
			if category != "" {
				object["category"] = category
			}
		} else {
			return nil, fmt.Errorf("%w: DISPLAY_MESSAGE category is unsupported", ErrInvalidState)
		}

	case "DISPLAY_COUNTDOWN":
		message, _ := object["message"].(string)
		message = strings.TrimSpace(message)
		if len([]rune(message)) > 240 {
			return nil, fmt.Errorf("%w: countdown message exceeds 240 characters", ErrInvalidState)
		}
		if message != "" {
			object["message"] = message
		} else {
			delete(object, "message")
		}

		target, targetErr := parseCountdownTarget(object["target_at"])
		if targetErr != nil {
			return nil, targetErr
		}
		if target.IsZero() {
			duration, ok := numericSeconds(object["duration_seconds"])
			if !ok || duration <= 0 {
				return nil, fmt.Errorf("%w: DISPLAY_COUNTDOWN requires future target_at or positive duration_seconds", ErrInvalidState)
			}
			target = now.UTC().Add(time.Duration(duration * float64(time.Second)))
		}
		if !target.After(now.UTC()) {
			return nil, fmt.Errorf("%w: DISPLAY_COUNTDOWN target_at must be in the future", ErrCommandExpired)
		}
		object["target_at"] = target.UTC().Format(time.RFC3339Nano)
		delete(object, "duration_seconds")

	case "DISPLAY_ALERT":
		message, _ := object["message"].(string)
		message = strings.TrimSpace(message)
		if len([]rune(message)) > 240 {
			return nil, fmt.Errorf("%w: alert message exceeds 240 characters", ErrInvalidState)
		}
		if message != "" {
			object["message"] = message
		} else {
			delete(object, "message")
		}
		role, ok := optionalEnum(object["role"], []string{"INFO", "STANDBY", "PLACES", "SHOW_START", "WARNING", "CRITICAL"}, "WARNING")
		if !ok {
			return nil, fmt.Errorf("%w: DISPLAY_ALERT role is unsupported", ErrInvalidState)
		}
		object["role"] = role
		motion, ok := optionalEnum(object["motion"], []string{"STEADY", "PULSE", "FLASH"}, "PULSE")
		if !ok {
			return nil, fmt.Errorf("%w: DISPLAY_ALERT motion is unsupported", ErrInvalidState)
		}
		object["motion"] = motion
		intensity := 100.0
		if value, exists := object["intensity_percent"]; exists {
			parsed, ok := numericSeconds(value)
			if !ok || parsed < 0 || parsed > 100 {
				return nil, fmt.Errorf("%w: DISPLAY_ALERT intensity_percent must be between 0 and 100", ErrInvalidState)
			}
			intensity = parsed
		}
		object["intensity_percent"] = intensity
		if value, exists := object["duration_seconds"]; exists {
			duration, ok := numericSeconds(value)
			if !ok || duration <= 0 || duration > 3600 {
				return nil, fmt.Errorf("%w: DISPLAY_ALERT duration_seconds must be between 0 and 3600", ErrInvalidState)
			}
			object["duration_seconds"] = duration
		}
		if chime, exists := object["chime_id"]; exists {
			value, ok := chime.(string)
			value = strings.TrimSpace(value)
			if !ok || value == "" || len([]rune(value)) > 64 {
				return nil, fmt.Errorf("%w: DISPLAY_ALERT chime_id must contain 1..64 characters", ErrInvalidState)
			}
			object["chime_id"] = value
		}

	case "DISPLAY_CHIME":
		chime, _ := object["chime_id"].(string)
		chime = strings.TrimSpace(chime)
		if chime == "" {
			chime = "default"
		}
		if len([]rune(chime)) > 64 {
			return nil, fmt.Errorf("%w: DISPLAY_CHIME chime_id exceeds 64 characters", ErrInvalidState)
		}
		object = map[string]any{"chime_id": chime}

	case "VIDEO_SOURCE_OPEN", "VIDEO_SOURCE_CLOSE", "VIDEO_SOURCE_SELECT", "VIDEO_SOURCE_ROUTE", "VIDEO_SOURCE_INSPECT":
		sourceID, _ := object["source_id"].(string)
		if strings.TrimSpace(sourceID) == "" {
			return nil, fmt.Errorf("%w: %s requires source_id", ErrInvalidState, commandType)
		}
		object["source_id"] = strings.TrimSpace(sourceID)
	}

	canonical, err := json.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("canonicalize Stage Device command payload: %w", err)
	}
	return canonical, nil
}

func optionalEnum(value any, allowed []string, fallback string) (string, bool) {
	if value == nil {
		return fallback, true
	}
	raw, ok := value.(string)
	if !ok {
		return "", false
	}
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return fallback, true
	}
	for _, candidate := range allowed {
		if raw == candidate {
			return raw, true
		}
	}
	return "", false
}

func parseCountdownTarget(value any) (time.Time, error) {
	if value == nil {
		return time.Time{}, nil
	}
	raw, ok := value.(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return time.Time{}, fmt.Errorf("%w: target_at must be an RFC3339 timestamp", ErrInvalidState)
	}
	target, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: target_at must be an RFC3339 timestamp", ErrInvalidState)
	}
	return target.UTC(), nil
}

func numericSeconds(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
