package stageaudio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// MPVBackend launches two isolated audio-only mpv players. IPC sockets live
// inside a mode-0700 temporary directory; they are NEVER exposed over TCP.
// This prototype is explicitly started by a local operator; no auto-start,
// SHOW integration, browser command proxy or background Cue replay exists.
type MPVBackend struct {
	dir       string
	sockets   map[string]string
	players   []*exec.Cmd
	waitDone  []chan struct{}
	nextID    atomic.Int64
	closeOnce sync.Once
}

func NewMPVBackend(runtimeRoot string) (*MPVBackend, error) {
	executable, err := exec.LookPath("mpv")
	if err != nil {
		return nil, fmt.Errorf("mpv is not installed (audio stays disabled): %w", err)
	}
	if runtimeRoot == "" {
		return nil, errors.New("private runtime root is required")
	}
	dir, err := os.MkdirTemp(runtimeRoot, "stageaudio-")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	b := &MPVBackend{dir: dir, sockets: map[string]string{}}
	for _, deck := range []string{DeckA, DeckB} {
		socket := filepath.Join(dir, "mpv-"+deck+".sock")
		cmd := exec.Command(executable,
			"--no-config", "--load-scripts=no", "--no-video", "--no-terminal",
			"--idle=yes", "--pause=yes", "--volume=49", "--force-window=no", "--audio-display=no",
			"--input-ipc-server="+socket)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Start(); err != nil {
			_ = b.Close()
			return nil, fmt.Errorf("start local mpv deck %s: %w", deck, err)
		}
		done := make(chan struct{})
		b.players = append(b.players, cmd)
		b.waitDone = append(b.waitDone, done)
		b.sockets[deck] = socket
		go func() { _ = cmd.Wait(); close(done) }()
		if err := waitForMPV(socket, done); err != nil {
			_ = b.Close()
			return nil, fmt.Errorf("deck %s unavailable: %w", deck, err)
		}
	}
	// All backend commands are serialized by Controller.
	return b, nil
}

func waitForMPV(socket string, done <-chan struct{}) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return errors.New("mpv exited before opening its local socket")
		default:
		}
		conn, err := net.DialTimeout("unix", socket, 120*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(40 * time.Millisecond)
	}
	return errors.New("timeout waiting for local mpv IPC")
}

func (b *MPVBackend) SocketDir() string { return b.dir }

func (b *MPVBackend) Send(ctx context.Context, deck string, command []any) error {
	socket, ok := b.sockets[deck]
	if !ok {
		return ErrInvalidCommand
	}
	dialer := net.Dialer{Timeout: 600 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return fmt.Errorf("local mpv unavailable: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	if when, ok := ctx.Deadline(); ok && when.Before(deadline) {
		deadline = when
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	id := b.nextID.Add(1)
	if err := json.NewEncoder(conn).Encode(map[string]any{
		"command": command, "request_id": id,
	}); err != nil {
		return fmt.Errorf("write local mpv command: %w", err)
	}
	decoder := json.NewDecoder(io.LimitReader(conn, 128<<10))
	for {
		var response struct {
			RequestID int64  `json:"request_id"`
			Error     string `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			return fmt.Errorf("read mpv response: %w", err)
		}
		if response.RequestID != id {
			continue // mpv also emits unrelated state events.
		}
		if response.Error != "success" {
			return fmt.Errorf("mpv declined command: %s", response.Error)
		}
		return nil
	}
}

func (b *MPVBackend) Close() error {
	b.closeOnce.Do(func() {
		for _, cmd := range b.players {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}
		for _, done := range b.waitDone {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
			}
		}
		_ = os.RemoveAll(b.dir) // dir is freshly allocated, private and process-owned.
	})
	return nil
}
