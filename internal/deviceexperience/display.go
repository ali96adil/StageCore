package deviceexperience

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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

func (r *Repository) GetDisplayState(ctx context.Context, deviceID string) (DisplayState, error) {
	var state DisplayState
	var payload string
	var command sql.NullString
	var effectiveUS int64
	var expiresUS sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT device_id, mode, payload_json, command_id, effective_at_us, expires_at_us
		FROM stage_display_state
		WHERE device_id = ?
	`, strings.TrimSpace(deviceID)).Scan(&state.DeviceID, &state.Mode, &payload, &command, &effectiveUS, &expiresUS)
	if err != nil {
		return DisplayState{}, err
	}
	state.Payload = json.RawMessage(payload)
	state.EffectiveAt = time.UnixMicro(effectiveUS).UTC()
	if command.Valid {
		state.CommandID = command.String
	}
	if expiresUS.Valid {
		value := time.UnixMicro(expiresUS.Int64).UTC()
		state.ExpiresAt = &value
	}
	return state, nil
}

// SafeDisplayStateForReconnect returns presentation state, never a command, so
// reconnect cannot replay an execution identity. Transient alerts/chimes are
// deliberately excluded. Expired message/countdown state is also excluded.
func (r *Repository) SafeDisplayStateForReconnect(ctx context.Context, deviceID string) (DisplayState, bool, error) {
	state, err := r.GetDisplayState(ctx, deviceID)
	if err != nil {
		if err == sql.ErrNoRows {
			return DisplayState{}, false, nil
		}
		return DisplayState{}, false, err
	}
	if state.ExpiresAt != nil && !state.ExpiresAt.After(r.now().UTC()) {
		return DisplayState{}, false, nil
	}
	switch state.Mode {
	case DisplayIdle, DisplayMessage, DisplayCountdown, DisplayBlackout:
		return state, true, nil
	default:
		return DisplayState{}, false, nil
	}
}

func (r *Repository) applyCompletedDisplayCommand(ctx context.Context, command DeviceCommand) error {
	if command.Status != "COMPLETED" {
		return nil
	}
	state := DisplayState{
		DeviceID:  command.DeviceID,
		CommandID: command.Envelope.CommandID,
		Payload:   command.Envelope.Payload,
	}
	if command.CompletedAt != nil {
		state.EffectiveAt = command.CompletedAt.UTC()
	}
	switch command.Envelope.CommandType {
	case "DISPLAY_MESSAGE":
		state.Mode = DisplayMessage
		state.ExpiresAt = payloadExpiry(command.Envelope.Payload, "expires_at")
	case "DISPLAY_COUNTDOWN":
		state.Mode = DisplayCountdown
		state.ExpiresAt = payloadExpiry(command.Envelope.Payload, "target_at")
	case "DISPLAY_CLEAR":
		state.Mode = DisplayIdle
		state.Payload = json.RawMessage(`{}`)
	case "DISPLAY_BLACKOUT":
		state.Mode = DisplayBlackout
		state.Payload = json.RawMessage(`{}`)
	default:
		// DISPLAY_ALERT and DISPLAY_CHIME are intentionally transient and never
		// replace the safe reconnect state underneath them.
		return nil
	}
	_, err := r.SetDisplayState(ctx, state)
	return err
}

func payloadExpiry(payload json.RawMessage, key string) *time.Time {
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil
	}
	raw, _ := object[key].(string)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	value = value.UTC()
	return &value
}
