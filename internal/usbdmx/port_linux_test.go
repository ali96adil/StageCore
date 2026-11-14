//go:build linux

package usbdmx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxSerialOpenRequiresIdentityApprovalBeforeTouchingDevices(t *testing.T) {
	// No real USB access in CI: verify fail-closed path and authorization
	// checks before Open() is allowed to be reached.
	ctx := context.Background()
	for _, path := range []string{
		"/dev/ttyUSB0", "/dev/serial/by-path/other",
		"/tmp/unverified-ttyUSB0", "/dev/serial/by-id/../../ttyUSB0",
	} {
		called := false
		p, err := OpenVerifiedLinuxPort(ctx, path, func(context.Context, string) error {
			called = true
			return nil
		})
		if p != nil || err == nil || called {
			t.Fatalf("invalid serial path %q accepted or authorized: %+v %v", path, p, err)
		}
	}
	p, err := OpenVerifiedLinuxPort(ctx, "/dev/serial/by-id/no-such-device", nil)
	if p != nil || !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatalf("missing identity callback not rejected: %+v %v", p, err)
	}
}

func TestLinuxPortNilCannotWriteOrClaimSuccess(t *testing.T) {
	var p *LinuxPort
	if err := p.WritePacket(context.Background(), ZeroFrame()); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("nil Linux port wrote an output: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxPortWritePacketCompletesShortWritesAcrossEAGAINAndEINTR(t *testing.T) {
	var packet [FrameSize]byte
	for i := range packet {
		packet[i] = byte(i % 251)
	}
	var output []byte
	writeCalls, pollCalls := 0, 0
	p := &LinuxPort{fd: 17}
	err := p.writePacket(context.Background(), packet,
		func(fd int, remaining []byte) (int, error) {
			if fd != 17 {
				t.Fatalf("unexpected fd: %d", fd)
			}
			writeCalls++
			switch writeCalls {
			case 1:
				output = append(output, remaining[:31]...)
				return 31, nil
			case 2:
				return 0, unix.EINTR
			case 3:
				return 0, unix.EAGAIN
			default:
				n := len(remaining)
				if n > 73 {
					n = 73
				}
				output = append(output, remaining[:n]...)
				return n, nil
			}
		},
		func(fd, timeout int) (int, int16, error) {
			if fd != 17 || timeout != 25 {
				t.Fatalf("unexpected poll arguments: fd=%d timeout=%d", fd, timeout)
			}
			pollCalls++
			return 1, unix.POLLOUT, nil
		})
	if err != nil {
		t.Fatalf("write packet: %v", err)
	}
	if !bytes.Equal(output, packet[:]) {
		t.Fatalf("serial packet bytes differ: got %d bytes, want %d", len(output), len(packet))
	}
	if writeCalls < 3 || pollCalls != 2 {
		t.Fatalf("retry behavior mismatch: writes=%d polls=%d", writeCalls, pollCalls)
	}
}

func TestLinuxPortWritePacketZeroProgressFails(t *testing.T) {
	p := &LinuxPort{fd: 18}
	polls := 0
	err := p.writePacket(context.Background(), ZeroFrame(),
		func(int, []byte) (int, error) { return 0, nil },
		func(int, int) (int, int16, error) {
			polls++
			return 0, 0, nil
		})
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("zero-progress write error = %v, want %v", err, io.ErrNoProgress)
	}
	if polls != 0 {
		t.Fatalf("polled after zero-progress write: %d", polls)
	}
}

func TestLinuxPortWritePacketDisconnectEventsStopRetry(t *testing.T) {
	for _, event := range []int16{unix.POLLHUP, unix.POLLERR, unix.POLLNVAL} {
		t.Run(unixEventName(event), func(t *testing.T) {
			p := &LinuxPort{fd: 19}
			writes := 0
			err := p.writePacket(context.Background(), ZeroFrame(),
				func(int, []byte) (int, error) {
					writes++
					return 0, unix.EAGAIN
				},
				func(int, int) (int, int16, error) { return 1, event, nil })
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte("disconnected or faulted")) {
				t.Fatalf("disconnect event error = %v", err)
			}
			if writes != 1 {
				t.Fatalf("continued writing after disconnect event: %d writes", writes)
			}
		})
	}
}

func TestLinuxPortWritePacketCancellationAfterPartialWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &LinuxPort{fd: 20}
	var output []byte
	writes := 0
	err := p.writePacket(ctx, ZeroFrame(),
		func(_ int, remaining []byte) (int, error) {
			writes++
			output = append(output, remaining[:17]...)
			cancel()
			return 17, nil
		},
		func(int, int) (int, int16, error) {
			t.Fatal("polled after caller cancellation")
			return 0, 0, nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled partial write error = %v", err)
	}
	if len(output) != 17 || writes != 1 {
		t.Fatalf("unexpected post-cancel output: bytes=%d writes=%d", len(output), writes)
	}
}

func TestLinuxPortWritePacketPollFailureIsReturned(t *testing.T) {
	p := &LinuxPort{fd: 21}
	writeCalls := 0
	err := p.writePacket(context.Background(), ZeroFrame(),
		func(int, []byte) (int, error) {
			writeCalls++
			return 0, unix.EAGAIN
		},
		func(int, int) (int, int16, error) { return 0, 0, unix.EBADF })
	if !errors.Is(err, unix.EBADF) {
		t.Fatalf("poll failure = %v, want EBADF", err)
	}
	if writeCalls != 1 {
		t.Fatalf("continued writing after poll failure: %d writes", writeCalls)
	}
}

func unixEventName(event int16) string {
	switch event {
	case unix.POLLHUP:
		return "POLLHUP"
	case unix.POLLERR:
		return "POLLERR"
	default:
		return "POLLNVAL"
	}
}
