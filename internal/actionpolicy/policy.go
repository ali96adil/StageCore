// Package actionpolicy reads optional execution settings from existing Action JSON policies.
package actionpolicy

import (
	"encoding/json"
	"fmt"
)

type Settings struct {
	StartDelayMS          int64
	RetryBeforeDispatch   bool
	RequireApplicationACK bool
}

func Parse(timeoutPolicy, errorPolicy json.RawMessage) (Settings, error) {
	var settings Settings
	for _, field := range []struct {
		policy json.RawMessage
		key    string
		target any
	}{
		{timeoutPolicy, "start_delay_ms", &settings.StartDelayMS},
		{errorPolicy, "retry_before_dispatch", &settings.RetryBeforeDispatch},
		{errorPolicy, "require_application_ack", &settings.RequireApplicationACK},
	} {
		var object map[string]json.RawMessage
		// Existing policies keep their legacy validation and behavior.
		if json.Unmarshal(field.policy, &object) != nil {
			continue
		}
		if value, ok := object[field.key]; ok {
			if string(value) == "null" || json.Unmarshal(value, field.target) != nil {
				return Settings{}, fmt.Errorf("invalid %s in Action policy", field.key)
			}
		}
	}
	if settings.StartDelayMS < 0 || settings.StartDelayMS > 600_000 {
		return Settings{}, fmt.Errorf("Action start_delay_ms must be between 0 and 600000")
	}
	return settings, nil
}
