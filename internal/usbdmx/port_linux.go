//go:build linux

package usbdmx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// DeviceApproval is a caller-supplied live identity and privilege review.
// It MUST compare the actual USB vendor/product/serial identity with the
// owner-selected DMXking device and exclude unrelated FTDI serial adapters.
// A missing callback is never treated as permission to access hardware.
type DeviceApproval func(context.Context, string) error

// LinuxPort is an exclusively opened FTDI virtual COM serial output.
// Opening cannot be triggered by importing this package or by USB discovery.
// A complete DMX packet is accepted only if every byte is written before
// timeout; no physical DMX fixture ACK is available in USB Pro output mode.
type LinuxPort struct {
	mu sync.Mutex
	fd int
	path string
	closed bool
}

// OpenVerifiedLinuxPort is an explicit, non-automatic opt-in facility for a
// future qualified sidecar. Requires a stable /dev/serial/by-id/* symlink,
// a caller identity check, and an exclusive /dev/ttyUSB* device.
func OpenVerifiedLinuxPort(ctx context.Context, byIDPath string, approval DeviceApproval) (*LinuxPort, error) {
	if approval == nil { return nil, ErrAuthorizationRequired }
	byIDPath = filepath.Clean(strings.TrimSpace(byIDPath))
	if filepath.Dir(byIDPath) != "/dev/serial/by-id" ||
		filepath.Base(byIDPath) == "." || filepath.Base(byIDPath) == ".." {
		return nil, fmt.Errorf("usbdmx: require explicit /dev/serial/by-id/ USB path")
	}
	info, err := os.Lstat(byIDPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return nil, fmt.Errorf("usbdmx: selected serial identity is missing or is not a by-id symlink")
	}
	target, err := filepath.EvalSymlinks(byIDPath)
	if err != nil { return nil, fmt.Errorf("usbdmx: resolve USB identity: %w", err) }
	if filepath.Dir(target) != "/dev" || !strings.HasPrefix(filepath.Base(target), "ttyUSB") {
		return nil, fmt.Errorf("usbdmx: USB DMX requires verified FTDI ttyUSB target")
	}
	if err := ctx.Err(); err != nil { return nil, err }
	if err := approval(ctx, byIDPath); err != nil {
		return nil, fmt.Errorf("%w: FTDI USB identity denied: %v", ErrAuthorizationRequired, err)
	}
	// O_NOFOLLOW prevents a last-moment replacement with an unsafe symlink;
	// TIOCEXCL below also blocks other normal opens of the device.
	fd, err := unix.Open(target, unix.O_RDWR|unix.O_NONBLOCK|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil { return nil, fmt.Errorf("usbdmx: cannot exclusively open USB serial: %w", err) }
	ok := false
	defer func() { if !ok { _ = unix.Close(fd) } }()
	resolvedAgain, err := filepath.EvalSymlinks(byIDPath)
	if err != nil || resolvedAgain != target {
		return nil, fmt.Errorf("usbdmx: FTDI serial identity changed during open")
	}
	if err := unix.IoctlSetInt(fd, unix.TIOCEXCL, 0); err != nil {
		return nil, fmt.Errorf("usbdmx: require exclusive tty ownership: %w", err)
	}
	if err := configureFTDIRaw(fd); err != nil {
		return nil, fmt.Errorf("usbdmx: configure USB Pro serial port: %w", err)
	}
	if err := ctx.Err(); err != nil { return nil, err }
	ok = true
	return &LinuxPort{fd: fd, path: byIDPath}, nil
}

func configureFTDIRaw(fd int) error {
	mode, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil { return err }
	// USB Pro API v1.44 says the VCP baud is dummy; 57600 is a common
	// host-side setting, NOT the DMX wire baud (the widget handles DMX timing).
	mode.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY
	mode.Oflag &^= unix.OPOST
	mode.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	mode.Cflag &^= unix.CSIZE | unix.PARENB | unix.CSTOPB | unix.CRTSCTS | unix.CBAUD
	mode.Cflag |= unix.CS8 | unix.CREAD | unix.CLOCAL | unix.B57600
	mode.Ispeed = unix.B57600
	mode.Ospeed = unix.B57600
	mode.Cc[unix.VMIN], mode.Cc[unix.VTIME] = 0, 0
	return unix.IoctlSetTermios(fd, unix.TCSETS, mode)
}

func (p *LinuxPort) WritePacket(ctx context.Context, packet [FrameSize]byte) error {
	if p == nil { return ErrQuarantined }
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed { return ErrQuarantined }
	// Bound local I/O even without caller deadline, so GO cannot wait on USB.
	local, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	written := 0
	for written < FrameSize {
		if err := local.Err(); err != nil { return fmt.Errorf("usbdmx: incomplete serial frame %d/%d: %w", written, FrameSize, err) }
		n, err := unix.Write(p.fd, packet[written:])
		written += n
		if err == nil && n > 0 { continue }
		if err != nil && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
			return fmt.Errorf("usbdmx: USB serial write %d/%d: %w", written, FrameSize, err)
		}
		if err == nil && n == 0 { return io.ErrNoProgress }
		// Poll in small bounded increments so context cancellation is obeyed.
		fds := []unix.PollFd{{Fd: int32(p.fd), Events: unix.POLLOUT}}
		nReady, pollErr := unix.Poll(fds, 25)
		if pollErr != nil && !errors.Is(pollErr, unix.EINTR) { return fmt.Errorf("usbdmx: serial poll: %w", pollErr) }
		if nReady > 0 && fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			return errors.New("usbdmx: USB port disconnected or faulted")
		}
	}
	// Local serial write completion is not a physical DMX/fixture ACK.
	return nil
}

func (p *LinuxPort) Close() error {
	if p == nil { return nil }
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed { return nil }
	p.closed = true
	return unix.Close(p.fd)
}
