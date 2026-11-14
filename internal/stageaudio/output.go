package stageaudio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AudioOutput describes an actual mpv audio-device that was exposed by the
// current Pi audio stack; do not synthesize sink IDs from Bluetooth addresses.
type AudioOutput struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
}

type audioDeviceReader interface {
	Query(context.Context, string, []any) (json.RawMessage, error)
}

// Outputs enumerates locally available output devices. Some headless Pi OS
// images need a running PipeWire/PulseAudio user service to advertise A2DP.
// Absence is surfaced as unavailable, never silently changed to analogue.
func (c *Controller) Outputs(ctx context.Context) ([]AudioOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outputsLocked(ctx)
}

func (c *Controller) outputsLocked(ctx context.Context) ([]AudioOutput, error) {
	reader, ok := c.sender.(audioDeviceReader)
	if !ok {
		return nil, fmt.Errorf("%w: mpv output discovery unavailable", ErrBackend)
	}
	raw, err := reader.Query(ctx, DeckA, []any{"get_property", "audio-device-list"})
	if err != nil {
		return nil, fmt.Errorf("%w: query playback outputs: %v", ErrBackend, err)
	}
	var devices []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &devices); err != nil {
		return nil, fmt.Errorf("%w: malformed mpv device list: %v", ErrBackend, err)
	}
	seen := map[string]bool{}
	outputs := make([]AudioOutput, 0, len(devices))
	for _, dev := range devices {
		id := strings.TrimSpace(dev.Name)
		if id == "" || id == "auto" || len(id) > 256 || seen[id] ||
			strings.ContainsAny(id, "\n\r\x00") {
			continue
		}
		seen[id] = true
		kind := "OTHER"
		lower := strings.ToLower(id)
		switch {
		case strings.Contains(lower, "bluez") || strings.Contains(lower, "bluetooth"):
			kind = "BLUETOOTH"
		case strings.Contains(lower, "analog") || strings.Contains(lower, "headphone") ||
			strings.Contains(lower, "bcm2835"):
			kind = "ANALOG"
		case strings.Contains(lower, "hdmi"):
			kind = "HDMI"
		case strings.Contains(lower, "usb"):
			kind = "USB"
		}
		label := strings.TrimSpace(dev.Description)
		if label == "" {
			label = id
		}
		if len(label) > 256 {
			label = label[:256]
		}
		outputs = append(outputs, AudioOutput{ID: id, Label: label, Kind: kind})
	}
	return outputs, nil
}

// SelectOutput explicitly routes BOTH stopped decks to an enumerated device.
// Switching with a playing/paused track is intentionally denied because it
// can cause an audible glitch or implicit playback on Bluetooth reconnection.
// No system-wide default-sink mutation is performed.
func (c *Controller) SelectOutput(ctx context.Context, id string) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id = strings.TrimSpace(id)
	for _, name := range []string{DeckA, DeckB} {
		if c.decks[name].Commanded != "STOPPED" {
			return c.statusLocked(), fmt.Errorf("%w: stop BOTH decks before output change", ErrInvalidCommand)
		}
	}
	list, err := c.outputsLocked(ctx)
	if err != nil {
		return c.statusLocked(), err
	}
	matched := false
	for _, output := range list {
		if output.ID == id {
			matched = true
			break
		}
	}
	if !matched {
		return c.statusLocked(), fmt.Errorf("%w: output is not presently enumerated", ErrInvalidCommand)
	}
	prev := c.output
	if err := c.send(ctx, DeckA, []any{"set_property", "audio-device", id}); err != nil {
		return c.statusLocked(), err
	}
	if err := c.send(ctx, DeckB, []any{"set_property", "audio-device", id}); err != nil {
		// A device route may be partially applied. Explicit failure state
		// protects against falsely claiming both decks use the same speaker.
		c.outputError = "OUTPUT_ROUTE_PARTIAL_RECONCILIATION_REQUIRED"
		if prev != "" {
			_ = c.send(ctx, DeckA, []any{"set_property", "audio-device", prev})
		}
		return c.statusLocked(), err
	}
	c.output = id
	c.outputError = ""
	return c.statusLocked(), nil
}
