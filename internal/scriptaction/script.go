package scriptaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

const CapabilityKey = "script.run"

const (
	defaultTimeout = 5 * time.Second
	maxTimeout     = 30 * time.Second
	maxArguments   = 128
)

type Executor struct{}

type targetConfig struct {
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments,omitempty"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	SecretRef        string   `json:"secret_ref,omitempty"`
}

type parameters struct {
	Arguments []string `json:"arguments,omitempty"`
	SecretRef string   `json:"secret_ref,omitempty"`
}

func New() Executor { return Executor{} }

func (Executor) Execute(ctx context.Context, req capability.Request) capability.Result {
	if req.Target == nil {
		return scriptFailure("SCRIPT_TARGET_REQUIRED", "Script Action requires a configured target", contracts.AckNone)
	}
	var target targetConfig
	if err := json.Unmarshal(req.Target.Configuration, &target); err != nil {
		return scriptFailure("SCRIPT_TARGET_INVALID", "Script target configuration is invalid", contracts.AckNone)
	}
	executable := strings.TrimSpace(target.Executable)
	if executable == "" || !filepath.IsAbs(executable) {
		return scriptFailure("SCRIPT_TARGET_INVALID", "Script executable must be an absolute path", contracts.AckNone)
	}
	if strings.TrimSpace(target.SecretRef) != "" {
		return scriptFailure("SECRET_STORE_REQUIRED", "Script secrets require the StageCore Secret Store", contracts.AckNone)
	}
	var p parameters
	if len(req.Parameters) == 0 {
		req.Parameters = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(req.Parameters, &p); err != nil {
		return scriptFailure("SCRIPT_INVALID_PARAMETERS", "Script Action parameters are invalid", contracts.AckNone)
	}
	if strings.TrimSpace(p.SecretRef) != "" {
		return scriptFailure("SECRET_STORE_REQUIRED", "Script secrets require the StageCore Secret Store", contracts.AckNone)
	}
	arguments := append(append([]string{}, target.Arguments...), p.Arguments...)
	if len(arguments) > maxArguments {
		return scriptFailure("SCRIPT_INVALID_PARAMETERS", "Script Action has too many arguments", contracts.AckNone)
	}

	timeout := scriptTimeout(req.TimeoutMS)
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, executable, arguments...)
	cmd.Env = []string{}
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if directory := strings.TrimSpace(target.WorkingDirectory); directory != "" {
		if !filepath.IsAbs(directory) {
			return scriptFailure("SCRIPT_TARGET_INVALID", "Script working directory must be an absolute path", contracts.AckNone)
		}
		cmd.Dir = directory
	}
	if err := cmd.Start(); err != nil {
		return scriptFailure("SCRIPT_START_FAILED", "Script child process could not be started", contracts.AckNone)
	}
	err := cmd.Wait()
	if err == nil {
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckAccepted, ResponseSummary: "Script child process exited successfully"}
	}
	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		return capability.Result{Result: domain.ExecutionTimedOut, AckLevel: contracts.AckAccepted, ErrorCode: "SCRIPT_TIMEOUT", ResponseSummary: "Script child process exceeded its bounded timeout and was terminated"}
	}
	if errors.Is(execCtx.Err(), context.Canceled) {
		return capability.Result{Result: domain.ExecutionCancelled, AckLevel: contracts.AckAccepted, ErrorCode: "SCRIPT_CANCELLED", ResponseSummary: "Script child process was cancelled and terminated"}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return scriptFailure(fmt.Sprintf("SCRIPT_EXIT_%d", exitErr.ExitCode()), fmt.Sprintf("Script child process exited with status %d", exitErr.ExitCode()), contracts.AckAccepted)
	}
	return scriptFailure("SCRIPT_EXECUTION_FAILED", "Script child process failed", contracts.AckAccepted)
}

func scriptTimeout(timeoutMS int64) time.Duration {
	if timeoutMS <= 0 {
		return defaultTimeout
	}
	value := time.Duration(timeoutMS) * time.Millisecond
	if value > maxTimeout {
		return maxTimeout
	}
	return value
}

func scriptFailure(code, summary string, ack contracts.AckLevel) capability.Result {
	return capability.Result{Result: domain.ExecutionFailed, AckLevel: ack, ErrorCode: code, ResponseSummary: summary}
}
