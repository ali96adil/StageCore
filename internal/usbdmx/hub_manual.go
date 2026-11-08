package usbdmx

import (
	"context"
	"errors"
	"sync"
	"time"
)

// HubManual arbitrates one physical output lease against operational session
// admission. It is constructed only for an explicitly configured project.
type HubManual struct {
	ProjectID string
	Output    *ManualOutput

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	fault    error
	blackout bool
}

func (h *HubManual) AcquireRuntime(ctx context.Context) (func(), error) {
	if h == nil {
		return func() {}, ctx.Err()
	}
	h.mu.Lock()
	if h.done != nil || h.fault != nil || ctx.Err() != nil {
		h.mu.Unlock()
		return nil, errors.New("manual USB-DMX output is active or its stop is unconfirmed")
	}
	return h.mu.Unlock, nil
}

func (h *HubManual) Run(ctx context.Context, req ManualRequest, authorize Authorize) error {
	if req.Duration <= 0 || req.Duration > 10*time.Second {
		return errors.New("Hub manual USB-DMX requires 1ms–10s timed output")
	}
	h.mu.Lock()
	if h.done != nil || h.fault != nil || h.blackout {
		h.mu.Unlock()
		return ErrManualBusy
	}
	ctx, cancel := context.WithCancel(ctx)
	h.cancel, h.done = cancel, make(chan struct{})
	h.mu.Unlock()
	err := h.Output.Run(ctx, req, authorize)
	cancel()
	h.mu.Lock()
	defer h.mu.Unlock()
	if errors.Is(err, ErrPhysicalStateUnknown) {
		h.fault = err
	}
	close(h.done)
	h.cancel, h.done = nil, nil
	return err
}

func (h *HubManual) Stop(ctx context.Context, projectID string) error {
	if h == nil || h.ProjectID != projectID {
		return nil
	}
	h.mu.Lock()
	done := h.done
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.fault
}

// SetBlackout prevents a new manual lease while EDIT blackout is engaged.
func (h *HubManual) SetBlackout(ctx context.Context, projectID string, enabled bool) error {
	if h == nil || h.ProjectID != projectID {
		return nil
	}
	h.mu.Lock()
	if !enabled && h.fault != nil {
		err := h.fault
		h.mu.Unlock()
		return err
	}
	h.blackout = enabled
	h.mu.Unlock()
	if enabled {
		return h.Stop(ctx, projectID)
	}
	return nil
}
