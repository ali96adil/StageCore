package simulator

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

type config struct {
	Simulation struct {
		Behavior  string `json:"behavior"`
		DelayMS   int64  `json:"delay_ms"`
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	} `json:"simulation"`
}

type Adapter struct{}

func (Adapter) Execute(ctx context.Context, req capability.Request) capability.Result {
	var cfg config
	if len(req.Parameters) > 0 {
		if err := json.Unmarshal(req.Parameters, &cfg); err != nil {
			return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: "SIM_INVALID_PARAMETERS", ResponseSummary: "invalid simulator parameters"}
		}
	}
	behavior := strings.ToUpper(strings.TrimSpace(cfg.Simulation.Behavior))
	if behavior == "" {
		behavior = "COMPLETE"
	}
	if cfg.Simulation.DelayMS < 0 {
		return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: "SIM_INVALID_DELAY", ResponseSummary: "negative simulator delay"}
	}

	switch behavior {
	case "COMPLETE":
		if interrupted := waitDelay(ctx, cfg.Simulation.DelayMS); interrupted != nil {
			return fromContext(interrupted)
		}
		summary := cfg.Simulation.Message
		if summary == "" {
			summary = "simulated completion"
		}
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckNone, ResponseSummary: summary}
	case "FAIL":
		if interrupted := waitDelay(ctx, cfg.Simulation.DelayMS); interrupted != nil {
			return fromContext(interrupted)
		}
		code := cfg.Simulation.ErrorCode
		if code == "" {
			code = "SIMULATED_FAILURE"
		}
		summary := cfg.Simulation.Message
		if summary == "" {
			summary = "simulated failure"
		}
		return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: code, ResponseSummary: summary}
	case "TIMEOUT":
		if _, ok := ctx.Deadline(); !ok {
			return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: "SIM_TIMEOUT_REQUIRES_DEADLINE", ResponseSummary: "timeout simulation requires timeout_policy.timeout_ms"}
		}
		<-ctx.Done()
		return fromContext(ctx.Err())
	default:
		return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: "SIM_UNKNOWN_BEHAVIOR", ResponseSummary: "unknown simulator behavior"}
	}
}

func waitDelay(ctx context.Context, delayMS int64) error {
	if delayMS == 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func fromContext(err error) capability.Result {
	if err == context.DeadlineExceeded {
		return capability.Result{Result: domain.ExecutionTimedOut, AckLevel: contracts.AckNone, ErrorCode: "TIMEOUT", ResponseSummary: "simulated timeout"}
	}
	return capability.Result{Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone, ErrorCode: "CANCELLED", ResponseSummary: "simulation cancelled"}
}
