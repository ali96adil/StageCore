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
)

// LocalReply is the typed answer from the manually started StageAudio sidecar.
// A successful response means only local command acknowledgment, not sound
// reaching the physical speaker or readiness for SHOW.
type LocalReply struct {
	Status           Status           `json:"status"`
	Outputs          []AudioOutput    `json:"outputs,omitempty"`
	BluetoothDevices []BluetoothDevice `json:"bluetooth_devices,omitempty"`
	Media            []string         `json:"media,omitempty"`
	Error            string           `json:"error,omitempty"`
}

type LocalClient struct { socket string }

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
	if c == nil { return LocalReply{}, errors.New("StageAudio sidecar not configured") }
	// Local IPC only; no TCP, remote URL, or fallback address is accepted.
	info, err := os.Lstat(c.socket)
	if err != nil { return LocalReply{}, fmt.Errorf("StageAudio service unavailable: %w", err) }
	if info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 {
		return LocalReply{}, errors.New("configured StageAudio path is not a local Unix socket")
	}
	dialer := net.Dialer{Timeout: 450*time.Millisecond}
	conn, err := dialer.DialContext(ctx, "unix", c.socket)
	if err != nil { return LocalReply{}, fmt.Errorf("connect local StageAudio: %w", err) }
	defer conn.Close()
	deadline := time.Now().Add(27*time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) { deadline = d }
	if err := conn.SetDeadline(deadline); err != nil { return LocalReply{}, err }
	line, err := json.Marshal(request)
	if err != nil || len(line) > 4090 { return LocalReply{}, ErrInvalidCommand }
	line = append(line, '\n')
	if _, err := conn.Write(line); err != nil { return LocalReply{}, fmt.Errorf("send local audio: %w", err) }
	raw, err := bufio.NewReader(io.LimitReader(conn, 128<<10)).ReadBytes('\n')
	if err != nil { return LocalReply{}, fmt.Errorf("receive local audio: %w", err) }
	var result LocalReply
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&result); err != nil {
		return LocalReply{}, fmt.Errorf("decode local audio response: %w", err)
	}
	if len(result.Error) > 0 {
		return result, fmt.Errorf("%w: %s", ErrBackend, result.Error)
	}
	return result, nil
}
