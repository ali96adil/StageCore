package pluginhost

import (
	"context"
	"errors"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
)

// Probe starts the configured Plugin, validates its plugin.ready handshake through
// the normal Host startup path, and returns a defensive copy of the ready state.
// The caller owns the Host lifetime and must Close it when the bounded probe ends.
func (h *Host) Probe(ctx context.Context) (*pluginprotocol.Ready, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := capability.CheckDispatch(ctx); err != nil {
		return nil, err
	}
	if err := h.ensureStartedLocked(ctx); err != nil {
		return nil, err
	}
	if err := capability.CheckDispatch(ctx); err != nil {
		h.stopLocked()
		return nil, err
	}
	if h.ready == nil {
		h.stopLocked()
		return nil, errors.New("plugin did not provide ready state")
	}
	ready := *h.ready
	ready.Capabilities = append([]string(nil), h.ready.Capabilities...)
	ready.Inputs = append([]string(nil), h.ready.Inputs...)
	return &ready, nil
}
