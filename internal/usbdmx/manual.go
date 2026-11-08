package usbdmx

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

var ErrManualDisabled = errors.New("usbdmx: manual output disabled")
var ErrPhysicalStateUnknown = errors.New("usbdmx: physical output state unknown")
var ErrManualBusy = errors.New("usbdmx: manual output already active")

type ChannelValue struct {
	Channel int `json:"channel"`
	Value   int `json:"value"`
}

type ManualRequest struct {
	Channels []ChannelValue
	// Zero holds the frame until the caller cancels the lease.
	Duration time.Duration
}

// ManualOutput holds one explicitly enabled and authorized output lease. Its
// caller owns cancellation (STOP, disconnect or lost authority). The factory
// must return a freshly identity-verified port; it is never invoked by discovery.
type ManualOutput struct {
	enabled bool
	open    func(context.Context) (PacketPort, error)
	active  atomic.Bool
}

func NewManualOutput(enabled bool, open func(context.Context) (PacketPort, error)) *ManualOutput {
	return &ManualOutput{enabled: enabled, open: open}
}

func (m *ManualOutput) Run(ctx context.Context, req ManualRequest, authorize Authorize) (err error) {
	if m == nil || !m.enabled || m.open == nil {
		return ErrManualDisabled
	}
	if req.Duration < 0 || req.Duration > 10*time.Minute || len(req.Channels) == 0 || len(req.Channels) > UniverseChannels {
		return errors.New("usbdmx: invalid manual duration or channel count")
	}
	var frame Levels
	var used [UniverseChannels]bool
	for _, channel := range req.Channels {
		if channel.Channel < 1 || channel.Channel > UniverseChannels || channel.Value < 0 || channel.Value > 255 {
			return errors.New("usbdmx: channel must be 1..512 and value must be 0..255")
		}
		index := channel.Channel - 1
		if used[index] {
			return fmt.Errorf("usbdmx: duplicate channel %d", channel.Channel)
		}
		used[index], frame[index] = true, byte(channel.Value)
	}
	if !m.active.CompareAndSwap(false, true) {
		return ErrManualBusy
	}
	defer m.active.Store(false)
	if err := requireAuthority(ctx, authorize); err != nil {
		return err
	}
	port, err := m.open(ctx)
	if err != nil {
		return err
	}
	tx, err := NewTransmitter(port)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if stopErr := tx.Disarm(cleanup); stopErr != nil {
			err = errors.Join(err, ErrPhysicalStateUnknown, stopErr)
		}
	}()
	if err := tx.Arm(ctx, authorize); err != nil {
		if tx.Status().Quarantined {
			return errors.Join(ErrPhysicalStateUnknown, err)
		}
		return err
	}
	if err := tx.Send(ctx, frame, authorize); err != nil {
		if tx.Status().Quarantined {
			return errors.Join(ErrPhysicalStateUnknown, err)
		}
		return err
	}
	var expiry <-chan time.Time
	if req.Duration > 0 {
		timer := time.NewTimer(req.Duration)
		defer timer.Stop()
		expiry = timer.C
	}
	check := time.NewTicker(250 * time.Millisecond)
	defer check.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-expiry:
			return nil
		case <-check.C:
			if err := requireAuthority(ctx, authorize); err != nil {
				return err
			}
		}
	}
}
