package pluginhost

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-04-plugin-ipc/internal/protocol"
)

var (
	ErrPluginExited  = errors.New("plugin process exited")
	ErrPluginTimeout = errors.New("plugin execution timed out")
)

type Host struct {
	mu             sync.Mutex
	command        string
	args           []string
	env            []string
	startupTimeout time.Duration
	stderr         io.Writer

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	ready   *protocol.Ready
}

func New(command string, args []string, env []string, stderr io.Writer) *Host {
	if stderr == nil {
		stderr = io.Discard
	}
	return &Host{command: command, args: args, env: env, startupTimeout: time.Second, stderr: stderr}
}

func (h *Host) Ready() *protocol.Ready {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ready == nil {
		return nil
	}
	copy := *h.ready
	copy.Capabilities = append([]string(nil), h.ready.Capabilities...)
	return &copy
}

func (h *Host) Execute(ctx context.Context, req protocol.ExecutionRequest) (protocol.ExecutionResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if req.TimeoutMS <= 0 {
		req.TimeoutMS = 500
	}
	if err := h.ensureStartedLocked(); err != nil {
		return failed(req.ExecutionID, "PLUGIN_FAILURE", err.Error()), err
	}
	if !contains(h.ready.Capabilities, req.Capability) {
		err := fmt.Errorf("plugin %s does not advertise capability %s", h.ready.PluginID, req.Capability)
		return failed(req.ExecutionID, "VALIDATION", err.Error()), err
	}

	line, err := json.Marshal(req)
	if err != nil {
		return failed(req.ExecutionID, "VALIDATION", err.Error()), err
	}
	if _, err := h.stdin.Write(append(line, '\n')); err != nil {
		h.stopLocked()
		return failed(req.ExecutionID, "PLUGIN_FAILURE", err.Error()), fmt.Errorf("write plugin request: %w", err)
	}

	deadline := time.Duration(req.TimeoutMS) * time.Millisecond
	readCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	response, err := h.readLineLocked(readCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			h.stopLocked()
			return failed(req.ExecutionID, "TIMEOUT", ErrPluginTimeout.Error()), ErrPluginTimeout
		}
		h.stopLocked()
		return failed(req.ExecutionID, "PLUGIN_FAILURE", ErrPluginExited.Error()), ErrPluginExited
	}
	var result protocol.ExecutionResult
	if err := json.Unmarshal(response, &result); err != nil {
		h.stopLocked()
		return failed(req.ExecutionID, "PLUGIN_FAILURE", "invalid plugin response"), err
	}
	if result.Type != "execution.result" || result.SchemaVersion != protocol.SchemaVersion || result.ExecutionID != req.ExecutionID {
		h.stopLocked()
		err := errors.New("plugin response contract mismatch")
		return failed(req.ExecutionID, "PLUGIN_FAILURE", err.Error()), err
	}
	return result, nil
}

func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopLocked()
}

func (h *Host) ensureStartedLocked() error {
	if h.cmd != nil {
		return nil
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
	h.cmd, h.stdin, h.scanner = cmd, stdin, bufio.NewScanner(stdout)

	ctx, cancel := context.WithTimeout(context.Background(), h.startupTimeout)
	defer cancel()
	line, err := h.readLineLocked(ctx)
	if err != nil {
		h.stopLocked()
		return fmt.Errorf("plugin startup handshake: %w", err)
	}
	var ready protocol.Ready
	if err := json.Unmarshal(line, &ready); err != nil {
		h.stopLocked()
		return err
	}
	if ready.Type != "plugin.ready" || ready.SchemaVersion != protocol.SchemaVersion || ready.PluginID == "" {
		h.stopLocked()
		return errors.New("invalid plugin.ready handshake")
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
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		_ = h.cmd.Wait()
	}
	h.cmd, h.stdin, h.scanner, h.ready = nil, nil, nil, nil
}

func failed(id, category, message string) protocol.ExecutionResult {
	return protocol.ExecutionResult{Type: "execution.result", SchemaVersion: protocol.SchemaVersion, ExecutionID: id, Status: "FAILED", ErrorCategory: category, ErrorMessage: message}
}

func contains(items []string, target string) bool {
	for _, v := range items {
		if v == target {
			return true
		}
	}
	return false
}
