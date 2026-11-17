package usbdmx

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// PacketPort is deliberately internal to a local, explicitly configured
// transmitter. The Hub must NOT expose raw packet writing over HTTP.
type PacketPort interface {
	WritePacket(context.Context, [FrameSize]byte) error
	Close() error
}

var (
	ErrNotArmed = errors.New("usbdmx: output not armed")
	ErrQuarantined = errors.New("usbdmx: output quarantined")
	ErrAuthorizationRequired = errors.New("usbdmx: fresh authorization required")
)

// Authorize must verify current Project assignment, Published Snapshot,
// operator permission and runtime fence at the moment of EVERY request.
// No nil callback, cached authorization or implicit re-arming is accepted.
type Authorize func(context.Context) error

// OutputStatus intentionally does not claim DMX fixture state. USB Pro label
// 6 does not acknowledge that lamps responded to a serial packet.
type OutputStatus struct {
	Armed bool `json:"armed"`
	Quarantined bool `json:"quarantined"`
	PhysicalOutputVerified bool `json:"physical_output_verified"`
	LastCommand string `json:"last_command"`
}

// Transmitter is a single-owner, fail-closed, opt-in testable output gate.
// It does not spawn a process, open USB or connect to the Hub by itself.
// After an I/O failure it must be DISCARDED and re-created through explicit
// hardware identity verification; it never replays old nonzero levels.
type Transmitter struct {
	mu sync.Mutex
	port PacketPort
	armed bool
	quarantined bool
	closed bool
	lastCommand string
}

func NewTransmitter(port PacketPort) (*Transmitter, error) {
	if port == nil {
		return nil, fmt.Errorf("%w: serial port required", ErrNotArmed)
	}
	return &Transmitter{port: port, lastCommand: "COLD_QUARANTINE"}, nil
}

func (t *Transmitter) Status() OutputStatus {
	if t == nil { return OutputStatus{Quarantined: true} }
	t.mu.Lock()
	defer t.mu.Unlock()
	return OutputStatus{
		Armed: t.armed, Quarantined: t.quarantined || t.closed,
		PhysicalOutputVerified: false, LastCommand: t.lastCommand,
	}
}

// Arm requires an explicit live authority check and a successful initial ZERO
// serial write. That write is a blackout *request*, not physical confirmation.
// Never call this from automatic USB discovery, restart, or reconnection.
func (t *Transmitter) Arm(ctx context.Context, authorize Authorize) error {
	if t == nil { return ErrQuarantined }
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.quarantined { return ErrQuarantined }
	if t.armed { return fmt.Errorf("%w: already armed", ErrNotArmed) }
	if err := requireAuthority(ctx, authorize); err != nil { return err }
	if err := t.port.WritePacket(ctx, ZeroFrame()); err != nil {
		return t.faultLocked("ZERO_FRAME_FAILED", err)
	}
	t.armed = true
	t.lastCommand = "ZERO_REQUESTED_ARMED"
	return nil
}

// Send replaces ALL 512 channels. It never caches or retries a Cue after a
// serial failure/reconnect, preventing a stale nonzero scene from reappearing.
// The caller must provide a current, verified authorization on every request.
func (t *Transmitter) Send(ctx context.Context, levels Levels, authorize Authorize) error {
	if t == nil { return ErrQuarantined }
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.quarantined { return ErrQuarantined }
	if !t.armed { return ErrNotArmed }
	if err := requireAuthority(ctx, authorize); err != nil { return err }
	if err := t.port.WritePacket(ctx, EncodeDMXFrame(levels)); err != nil {
		return t.faultLocked("DMX_WRITE_FAILED", err)
	}
	t.lastCommand = "DMX_FRAME_SUBMITTED_NOT_PHYSICAL_ACK"
	return nil
}

// Disarm best-effort requests a full zero universe if armed, then closes the
// serial port. A failed zero MUST NOT be reported as a successful blackout.
// No re-arm is possible after closing; a fresh verified instance is required.
func (t *Transmitter) Disarm(ctx context.Context) error {
	if t == nil { return nil }
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed { return nil }
	var zeroErr error
	if t.armed && !t.quarantined {
		zeroErr = t.port.WritePacket(ctx, ZeroFrame())
	}
	t.armed = false
	t.closed = true
	t.quarantined = true
	if zeroErr != nil {
		t.lastCommand = "ZERO_FRAME_FAILED_PHYSICAL_STATE_UNKNOWN"
	} else if t.lastCommand != "DMX_WRITE_FAILED" && t.lastCommand != "ZERO_FRAME_FAILED" {
		t.lastCommand = "CLOSED_PHYSICAL_STATE_UNVERIFIED"
	}
	closeErr := t.port.Close()
	if zeroErr != nil { return fmt.Errorf("usbdmx: zero request failed, physical state unknown: %w", zeroErr) }
	if closeErr != nil { return fmt.Errorf("usbdmx: close serial port: %w", closeErr) }
	return nil
}

func requireAuthority(ctx context.Context, authorize Authorize) error {
	if authorize == nil { return ErrAuthorizationRequired }
	if err := ctx.Err(); err != nil { return err }
	if err := authorize(ctx); err != nil { return fmt.Errorf("%w: %v", ErrAuthorizationRequired, err) }
	if err := ctx.Err(); err != nil { return err }
	return nil
}

func (t *Transmitter) faultLocked(reason string, cause error) error {
	t.armed = false
	t.quarantined = true
	t.closed = true
	t.lastCommand = reason
	_ = t.port.Close()
	return fmt.Errorf("usbdmx: %s, output state unknown, no retry: %w", reason, cause)
}
