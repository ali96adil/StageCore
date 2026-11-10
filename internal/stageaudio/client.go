package stageaudio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
)

// LocalReply is the typed answer from the manually started StageAudio sidecar.
// A successful response means only local command acknowledgment, not sound
// reaching the physical speaker or readiness for SHOW.
type LocalReply struct {
	Status           Status            `json:"status"`
	Outputs          []AudioOutput     `json:"outputs,omitempty"`
	BluetoothDevices []BluetoothDevice `json:"bluetooth_devices,omitempty"`
	Media            []string          `json:"media,omitempty"`
	Error            string            `json:"error,omitempty"`
}

type LocalClient struct{ socket string }

// ErrInFlightAuthorityRevoked means an audio command may already have reached
// the sidecar. Closing the IPC connection does not prove that it stopped.
var ErrInFlightAuthorityRevoked = fmt.Errorf("StageAudio reply withdrawn: %w", capability.ErrDispatchAuthorityRevoked)

// NewLocalClient deliberately does not discover /tmp sockets: the Hub must
// be explicitly configured with a trusted same-host Unix socket path.
func NewLocalClient(socket string) (*LocalClient, error) {
	socket = strings.TrimSpace(socket)
	if socket == "" || !filepath.IsAbs(socket) || len(socket) > 1024 {
		return nil, fmt.Errorf("StageAudio socket must be a configured absolute path")
	}
	return &LocalClient{socket: socket}, nil
}

func (c *LocalClient) Call(ctx context.Context, request Request) (LocalReply, error) {
	if c == nil {
		return LocalReply{}, errors.New("StageAudio sidecar not configured")
	}
	// Local IPC only; no TCP, remote URL, or fallback address is accepted.
	info, err := os.Lstat(c.socket)
	if err != nil {
		return LocalReply{}, fmt.Errorf("StageAudio service unavailable: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 {
		return LocalReply{}, errors.New("configured StageAudio path is not a local Unix socket")
	}
	dialer := net.Dialer{Timeout: 450 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return LocalReply{}, fmt.Errorf("connect local StageAudio: %w", err)
	}
	defer conn.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancellation()
	deadline := time.Now().Add(27 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return LocalReply{}, err
	}
	line, err := json.Marshal(request)
	if err != nil || len(line) > 4090 {
		return LocalReply{}, ErrInvalidCommand
	}
	line = append(line, '\n')
	if err := capability.CheckDispatch(ctx); err != nil {
		return LocalReply{}, err
	}
	if written, err := conn.Write(line); err != nil || written != len(line) {
		if ctx.Err() != nil {
			return LocalReply{}, ctx.Err()
		}
		if err == nil {
			err = io.ErrShortWrite
		}
		return LocalReply{}, fmt.Errorf("send local audio: %w", err)
	}
	raw, err := waitLocalReply(ctx, conn)
	if err != nil {
		return LocalReply{}, err
	}
	var result LocalReply
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&result); err != nil {
		return LocalReply{}, fmt.Errorf("decode local audio response: %w", err)
	}
	if len(result.Error) > 0 {
		return result, fmt.Errorf("%w: %s", ErrBackend, result.Error)
	}
	return result, nil
}

func waitLocalReply(ctx context.Context, conn net.Conn) ([]byte, error) {
	pending, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stop := context.AfterFunc(pending, func() { _ = conn.Close() })
	defer stop()
	go func() {
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-pending.Done():
				return
			case <-tick.C:
				if err := capability.CheckDispatch(pending); err != nil {
					if errors.Is(err, capability.ErrDispatchAuthorityRevoked) {
						err = ErrInFlightAuthorityRevoked
					}
					cancel(err)
					return
				}
			}
		}
	}()
	raw, err := bufio.NewReader(io.LimitReader(conn, 128<<10)).ReadBytes('\n')
	if cause := context.Cause(pending); cause != nil {
		return nil, cause
	}
	if checkErr := capability.CheckDispatch(pending); checkErr != nil {
		if cause := context.Cause(pending); cause != nil {
			return nil, cause
		}
		if errors.Is(checkErr, capability.ErrDispatchAuthorityRevoked) {
			return nil, ErrInFlightAuthorityRevoked
		}
		return nil, checkErr
	}
	if err != nil {
		return nil, fmt.Errorf("receive local audio: %w", err)
	}
	return raw, nil
}
