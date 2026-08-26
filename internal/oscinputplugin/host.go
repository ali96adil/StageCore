package oscinputplugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/oscplugin"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
	"github.com/ali96adil/StageCore/internal/routing"
)

var (
	ErrPermissionDenied = errors.New("OSC input plugin permission denied")
	ErrPluginExited     = errors.New("OSC input plugin process exited")
	ErrProtocol         = errors.New("OSC input plugin protocol error")
)

type Manifest struct {
	PluginID           string
	InputPermissions   map[string][]string
	GrantedPermissions []string
}

type Host struct {
	mu             sync.Mutex
	command        string
	listenAddress  string
	stderr         io.Writer
	startupTimeout time.Duration
	manifest       Manifest
	engine         *routing.Engine
	sessionID      string

	cmd     *exec.Cmd
	scanner *bufio.Scanner
	ready   *pluginprotocol.Ready
}

func New(command, listenAddress string, stderr io.Writer, manifest Manifest, engine *routing.Engine, sessionID string) *Host {
	if stderr == nil {
		stderr = io.Discard
	}
	return &Host{
		command:        command,
		listenAddress:  listenAddress,
		stderr:         stderr,
		startupTimeout: time.Second,
		manifest: Manifest{
			PluginID:           manifest.PluginID,
			InputPermissions:   clonePermissionMap(manifest.InputPermissions),
			GrantedPermissions: append([]string(nil), manifest.GrantedPermissions...),
		},
		engine:    engine,
		sessionID: sessionID,
	}
}

func (h *Host) Start(ctx context.Context) error {
	if h == nil {
		return errors.New("OSC input plugin host is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cmd != nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.engine == nil || strings.TrimSpace(h.sessionID) == "" {
		return errors.New("routing engine and session are required")
	}
	if err := validateLoopbackListen(h.listenAddress); err != nil {
		return err
	}
	if err := h.authorizeLocked(oscplugin.InputOSCReceive); err != nil {
		return err
	}
	if strings.TrimSpace(h.command) == "" {
		return errors.New("OSC plugin command is required")
	}

	cmd := exec.Command(h.command, "--receive", "--listen", h.listenAddress)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("OSC input plugin stdout: %w", err)
	}
	cmd.Stderr = h.stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start OSC input plugin: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4*1024), 256*1024)
	h.cmd, h.scanner = cmd, scanner

	startupCtx, cancel := context.WithTimeout(ctx, h.startupTimeout)
	defer cancel()
	line, err := readLine(startupCtx, scanner)
	if err != nil {
		h.stopLocked()
		return fmt.Errorf("OSC input plugin startup handshake: %w", err)
	}
	var ready pluginprotocol.Ready
	if err := json.Unmarshal(line, &ready); err != nil {
		h.stopLocked()
		return fmt.Errorf("%w: invalid plugin.ready JSON", ErrProtocol)
	}
	if ready.Type != "plugin.ready" || ready.SchemaVersion != pluginprotocol.SchemaVersion || ready.PluginID == "" {
		h.stopLocked()
		return fmt.Errorf("%w: invalid plugin.ready handshake", ErrProtocol)
	}
	if h.manifest.PluginID != "" && ready.PluginID != h.manifest.PluginID {
		h.stopLocked()
		return fmt.Errorf("%w: plugin identity mismatch", ErrProtocol)
	}
	if !contains(ready.Inputs, oscplugin.InputOSCReceive) {
		h.stopLocked()
		return fmt.Errorf("%w: plugin does not advertise %s", ErrProtocol, oscplugin.InputOSCReceive)
	}
	if err := validateLoopbackListen(ready.ListenAddress); err != nil {
		h.stopLocked()
		return fmt.Errorf("%w: plugin reported unsafe listen address: %v", ErrProtocol, err)
	}
	h.ready = &ready
	return nil
}

func (h *Host) LocalAddr() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ready == nil {
		return ""
	}
	return h.ready.ListenAddress
}

func (h *Host) Serve(ctx context.Context) error {
	if err := h.Start(ctx); err != nil {
		return err
	}
	h.mu.Lock()
	scanner := h.scanner
	h.mu.Unlock()
	if scanner == nil {
		return ErrPluginExited
	}

	type scannedLine struct {
		line []byte
		err  error
	}
	lines := make(chan scannedLine, 1)
	go func() {
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- scannedLine{line: line}:
			case <-ctx.Done():
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case lines <- scannedLine{err: err}:
		case <-ctx.Done():
		}
	}()

	for {
		select {
		case <-ctx.Done():
			h.Close()
			return nil
		case item := <-lines:
			if item.err != nil {
				h.Close()
				return fmt.Errorf("%w: %v", ErrPluginExited, item.err)
			}
			var event pluginprotocol.InputEvent
			if err := json.Unmarshal(item.line, &event); err != nil {
				h.Close()
				return fmt.Errorf("%w: invalid input event JSON", ErrProtocol)
			}
			if event.Type != "input.event" || event.SchemaVersion != pluginprotocol.SchemaVersion || event.InputType != oscplugin.InputOSCReceive || strings.TrimSpace(event.Source) == "" || !json.Valid(event.Value) {
				h.Close()
				return fmt.Errorf("%w: input event contract mismatch", ErrProtocol)
			}
			var value any
			if err := json.Unmarshal(event.Value, &value); err != nil {
				h.Close()
				return fmt.Errorf("%w: decode input value", ErrProtocol)
			}
			if _, err := h.engine.ReceiveOSC(ctx, h.sessionID, event.Source, []any{value}); err != nil {
				return fmt.Errorf("route external OSC input: %w", err)
			}
		}
	}
}

func (h *Host) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopLocked()
}

func (h *Host) authorizeLocked(inputType string) error {
	required, declared := h.manifest.InputPermissions[inputType]
	if !declared {
		return fmt.Errorf("%w: input %s is not declared by manifest", ErrPermissionDenied, inputType)
	}
	for _, permission := range required {
		if !contains(h.manifest.GrantedPermissions, permission) {
			return fmt.Errorf("%w: %s requires %s", ErrPermissionDenied, inputType, permission)
		}
	}
	return nil
}

func (h *Host) stopLocked() {
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		_ = h.cmd.Wait()
	}
	h.cmd, h.scanner, h.ready = nil, nil, nil
}

func validateLoopbackListen(address string) error {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return fmt.Errorf("OSC input listen address must be host:port: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("OSC input is loopback-only until SEC0-SEC2 Stage LAN security gate passes")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return errors.New("OSC input listen port must be 0..65535")
	}
	return nil
}

func readLine(ctx context.Context, scanner *bufio.Scanner) ([]byte, error) {
	type answer struct {
		line []byte
		err  error
	}
	ch := make(chan answer, 1)
	go func() {
		if scanner.Scan() {
			ch <- answer{line: append([]byte(nil), scanner.Bytes()...)}
			return
		}
		if err := scanner.Err(); err != nil {
			ch <- answer{err: err}
			return
		}
		ch <- answer{err: io.EOF}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case item := <-ch:
		return item.line, item.err
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func clonePermissionMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}
