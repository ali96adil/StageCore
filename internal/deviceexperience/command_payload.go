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
