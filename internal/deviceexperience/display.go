package deviceexperience

import (
	"context"
	"fmt"
	"strings"
)

func (r *Repository) SetDisplayState(ctx context.Context, state DisplayState) (DisplayState, error) {
	state.DeviceID = strings.TrimSpace(state.DeviceID)
	state.CommandID = strings.TrimSpace(state.CommandID)
	if state.DeviceID == "" || !validDisplayMode(state.Mode) {
		return DisplayState{}, ErrInvalidState
	}
	device, err := r.GetDevice(ctx, state.DeviceID)
	if err != nil {
		return DisplayState{}, err
	}
	if !contains(device.Capabilities, displayCapability(state.Mode)) {
		return DisplayState{}, fmt.Errorf("%w: %s", ErrCapabilityMissing, displayCapability(state.Mode))
	}
	if state.EffectiveAt.IsZero() {
		state.EffectiveAt = r.now().UTC()
	} else {
		state.EffectiveAt = state.EffectiveAt.UTC()
	}
	if state.ExpiresAt != nil {
		expires := state.ExpiresAt.UTC()
		state.ExpiresAt = &expires
		if !expires.After(state.EffectiveAt) {
			return DisplayState{}, ErrCommandExpired
		}
	}
	state.Payload = normalizeJSON(state.Payload, `{}`)
	var command, expires any
	if state.CommandID != "" {
		command = state.CommandID
	}
	if state.ExpiresAt != nil {
		expires = state.ExpiresAt.UnixMicro()
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO stage_display_state (device_id, mode, payload_json, command_id, effective_at_us, expires_at_us)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
		mode=excluded.mode, payload_json=excluded.payload_json, command_id=excluded.command_id,
		effective_at_us=excluded.effective_at_us, expires_at_us=excluded.expires_at_us
	`, state.DeviceID, state.Mode, string(state.Payload), command, state.EffectiveAt.UnixMicro(), expires)
	if err != nil {
		return DisplayState{}, fmt.Errorf("set display state: %w", err)
	}
	return state, nil
}
