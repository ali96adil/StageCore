package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/pluginprotocol"
)

var (
	ErrPluginExited           = errors.New("plugin process exited")
	ErrPluginTimeout          = errors.New("plugin execution timed out")
	ErrPluginPermissionDenied = errors.New("plugin permission denied")
	ErrPluginNotRunning       = errors.New("plugin process is not running")
	ErrPluginWaitInProgress   = errors.New("plugin process wait is already in progress")
)

type Manifest struct {
	PluginID              string
	CapabilityPermissions map[string][]string
	GrantedPermissions    []string
}

type Host struct {
	mu             sync.Mutex
	command        string
	args           []string
	env            []string
	startupTimeout time.Duration
	stderr         io.Writer
	manifest       Manifest

	cmd            *exec.Cmd
	stdin          io.WriteCloser
	scanner        *bufio.Scanner
	ready          *pluginprotocol.Ready
	waitInProgress bool
}

func New(command string, args []string, env []string, stderr io.Writer, manifest Manifest) *Host {
	if stderr == nil {
		stderr = io.Discard
	}
	return &Host{
		command:        command,
		args:           append([]string(nil), args...),
		env:            append([]string(nil), env...),
		startupTimeout: time.Second,
		stderr:         stderr,
		manifest: Manifest{
			PluginID:              manifest.PluginID,
			CapabilityPermissions: clonePermissionMap(manifest.CapabilityPermissions),
			GrantedPermissions:    append([]string(nil), manifest.GrantedPermissions...),
		},
	}
}

func (h *Host) Ready() *pluginprotocol.Ready {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ready == nil {
		return nil
	}
	copy := *h.ready
	copy.Capabilities = append([]string(nil), h.ready.Capabilities...)
	return &copy
}

func (h *Host) Execute(ctx context.Context, req pluginprotocol.ExecutionRequest) (pluginprotocol.ExecutionResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return cancelled(req.ExecutionID), err
	}
	if req.TimeoutMS <= 0 {
		req.TimeoutMS = 500
	}
	if err := h.authorizeLocked(req.Capability); err != nil {
		return failed(req.ExecutionID, "PLUGIN_PERMISSION_DENIED", "PERMISSION", err.Error()), err
	}
	if err := h.ensureStartedLocked(); err != nil {
		return failed(req.ExecutionID, "PLUGIN_FAILURE", "PLUGIN_FAILURE", err.Error()), err
	}
	if err := ctx.Err(); err != nil {
		h.stopLocked()
		return cancelled(req.ExecutionID), err
	}
	if h.ready.PluginID != h.manifest.PluginID {
		err := fmt.Errorf("plugin identity mismatch: got %q want %q", h.ready.PluginID, h.manifest.PluginID)
		h.stopLocked()
		return failed(req.ExecutionID, "PLUGIN_IDENTITY_MISMATCH", "PLUGIN_FAILURE", err.Error()), err
	}
	if !contains(h.ready.Capabilities, req.Capability) {
		err := fmt.Errorf("plugin %s does not advertise capability %s", h.ready.PluginID, req.Capability)
		return failed(req.ExecutionID, "CAPABILITY_UNAVAILABLE", "VALIDATION", err.Error()), err
	}

	line, err := json.Marshal(req)
	if err != nil {
		return failed(req.ExecutionID, "INVALID_PLUGIN_REQUEST", "VALIDATION", err.Error()), err
	}
	if _, err := h.stdin.Write(append(line, '\n')); err != nil {
		h.stopLocked()
		return failed(req.ExecutionID, "PLUGIN_FAILURE", "PLUGIN_FAILURE", err.Error()), fmt.Errorf("write plugin request: %w", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond)
	defer cancel()
	response, err := h.readLineLocked(readCtx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			h.stopLocked()
			return cancelled(req.ExecutionID), context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			h.stopLocked()
			return failed(req.ExecutionID, "TIMEOUT", "TIMEOUT", ErrPluginTimeout.Error()), ErrPluginTimeout
		}
		h.stopLocked()
		return failed(req.ExecutionID, "PLUGIN_FAILURE", "PLUGIN_FAILURE", ErrPluginExited.Error()), ErrPluginExited
	}
	var result pluginprotocol.ExecutionResult
	if err := json.Unmarshal(response, &result); err != nil {
		h.stopLocked()
		return failed(req.ExecutionID, "PLUGIN_PROTOCOL_ERROR", "PLUGIN_FAILURE", "invalid plugin response"), err
	}
	if result.Type != "execution.result" || result.SchemaVersion != pluginprotocol.SchemaVersion || result.ExecutionID != req.ExecutionID {
		h.stopLocked()
		err := errors.New("plugin response contract mismatch")
		return failed(req.ExecutionID, "PLUGIN_PROTOCOL_ERROR", "PLUGIN_FAILURE", err.Error()), err
	}
	return result, nil
}

// Wait blocks until the currently running Plugin exits. It is intended for a
// supervisor that owns the Host lifetime. Close may be called concurrently;
// Close will kill the child and the active waiter remains responsible for
// reaping it.
func (h *Host) Wait() error {
	h.mu.Lock()
	if h.cmd == nil {
		h.mu.Unlock()
		return ErrPluginNotRunning
	}
	if h.waitInProgress {
		h.mu.Unlock()
		return ErrPluginWaitInProgress
	}
	cmd := h.cmd
	h.waitInProgress = true
	h.mu.Unlock()

	err := cmd.Wait()

	h.mu.Lock()
	if h.cmd == cmd {
		h.cmd, h.stdin, h.scanner, h.ready = nil, nil, nil, nil
	}
	h.waitInProgress = false
	h.mu.Unlock()
	return err
}

func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopLocked()
}

func (h *Host) authorizeLocked(capability string) error {
	required, declared := h.manifest.CapabilityPermissions[capability]
	if !declared {
		return fmt.Errorf("%w: capability %s is not declared by manifest", ErrPluginPermissionDenied, capability)
	}
	for _, permission := range required {
		if !contains(h.manifest.GrantedPermissions, permission) {
			return fmt.Errorf("%w: %s requires %s", ErrPluginPermissionDenied, capability, permission)
		}
	}
	return nil
}

func (h *Host) ensureStartedLocked() error {
	if h.cmd != nil {
		return nil
	}
	if strings.TrimSpace(h.command) == "" {
		return errors.New("plugin command is required")
	}
	cmd := exec.Command(h.command, h.args...)
	if len(h.env) > 0 {
		cmd.Env = append(cmd.Environ(), h.env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = h.stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4*1024), 256*1024)
	h.cmd, h.stdin, h.scanner = cmd, stdin, scanner

	ctx, cancel := context.WithTimeout(context.Background(), h.startupTimeout)
	defer cancel()
	line, err := h.readLineLocked(ctx)
	if err != nil {
		h.stopLocked()
		return fmt.Errorf("plugin startup handshake: %w", err)
	}
	var ready pluginprotocol.Ready
	if err := json.Unmarshal(line, &ready); err != nil {
		h.stopLocked()
		return fmt.Errorf("decode plugin.ready: %w", err)
	}
	if ready.Type != "plugin.ready" || ready.SchemaVersion != pluginprotocol.SchemaVersion || ready.PluginID == "" {
		h.stopLocked()
		return errors.New("invalid plugin.ready handshake")
	}
	if h.manifest.PluginID != "" && ready.PluginID != h.manifest.PluginID {
		h.stopLocked()
		return fmt.Errorf("plugin identity mismatch: got %q want %q", ready.PluginID, h.manifest.PluginID)
	}
	h.ready = &ready
	return nil
}

func (h *Host) readLineLocked(ctx context.Context) ([]byte, error) {
	type answer struct {
		line []byte
		err  error
	}
	ch := make(chan answer, 1)
	scanner := h.scanner
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
	case a := <-ch:
		return a.line, a.err
	}
}

func (h *Host) stopLocked() {
	if h.stdin != nil {
		_ = h.stdin.Close()
	}
	cmd := h.cmd
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		if !h.waitInProgress {
			_ = cmd.Wait()
		}
	}
	if !h.waitInProgress {
		h.cmd, h.stdin, h.scanner, h.ready = nil, nil, nil, nil
	}
}

func failed(id, code, category, message string) pluginprotocol.ExecutionResult {
	return pluginprotocol.ExecutionResult{
		Type:          "execution.result",
		SchemaVersion: pluginprotocol.SchemaVersion,
		ExecutionID:   id,
		Status:        "FAILED",
		ErrorCode:     code,
		ErrorCategory: category,
		ErrorMessage:  message,
	}
}

func cancelled(id string) pluginprotocol.ExecutionResult {
	return pluginprotocol.ExecutionResult{
		Type:          "execution.result",
		SchemaVersion: pluginprotocol.SchemaVersion,
		ExecutionID:   id,
		Status:        "CANCELLED",
		ErrorCode:     "CANCELLED",
		ErrorCategory: "CANCELLED",
		ErrorMessage:  "plugin execution cancelled",
	}
}

func contains(items []string, target string) bool {
	for _, v := range items {
		if v == target {
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
