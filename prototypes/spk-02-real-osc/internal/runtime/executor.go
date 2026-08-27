package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-02-real-osc/internal/osc"
)

type Action struct {
	ID         string      `json:"id"`
	Capability string      `json:"capability"`
	TargetID   string      `json:"target_id"`
	Parameters osc.Message `json:"parameters"`
}

type ExecutionResult struct {
	ActionID   string        `json:"action_id"`
	TargetID   string        `json:"target_id"`
	Capability string        `json:"capability"`
	Status     string        `json:"status"`
	AckLevel   string        `json:"ack_level"`
	BytesSent  int           `json:"bytes_sent"`
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
}

type Executor struct {
	Endpoints map[string]osc.Endpoint
	Sender    osc.Sender
}

func (e Executor) Execute(ctx context.Context, action Action) ExecutionResult {
	result := ExecutionResult{ActionID: action.ID, TargetID: action.TargetID, Capability: action.Capability}
	if action.Capability != "osc.send" {
		result.Status = "FAILED"
		result.AckLevel = "NONE"
		result.Error = "unsupported capability"
		return result
	}
	endpoint, ok := e.Endpoints[action.TargetID]
	if !ok {
		result.Status = "FAILED"
		result.AckLevel = "NONE"
		result.Error = "target not found"
		return result
	}

	send, err := e.Sender.Send(ctx, endpoint, action.Parameters)
	result.Status = send.Status
	result.AckLevel = send.AckLevel
	result.BytesSent = send.BytesSent
	result.Duration = send.Duration
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func ValidateAction(action Action, endpoints map[string]osc.Endpoint) error {
	if action.Capability != "osc.send" {
		return fmt.Errorf("capability %q is not supported by OSC executor", action.Capability)
	}
	if action.TargetID == "" {
		return errors.New("target_id is required")
	}
	if _, ok := endpoints[action.TargetID]; !ok {
		return fmt.Errorf("target %q not found", action.TargetID)
	}
	_, err := osc.EncodeMessage(action.Parameters)
	return err
}
