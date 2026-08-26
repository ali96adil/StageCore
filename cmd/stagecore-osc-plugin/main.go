package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ali96adil/StageCore/internal/osc"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
)

type oscParams struct {
	Address   string         `json:"address"`
	Arguments []osc.Argument `json:"arguments"`
}

const (
	pluginID          = "stagecore.osc"
	capabilityOSCSend = "osc.send"
)

func main() {
	out := bufio.NewWriter(os.Stdout)
	write(out, pluginprotocol.Ready{
		Type:          "plugin.ready",
		SchemaVersion: pluginprotocol.SchemaVersion,
		PluginID:      pluginID,
		PluginVersion: "0.1.0",
		Capabilities:  []string{capabilityOSCSend},
	})

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4*1024), 256*1024)
	for scanner.Scan() {
		var req pluginprotocol.ExecutionRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		started := time.Now()
		result := execute(req)
		result.DurationMS = time.Since(started).Milliseconds()
		write(out, result)
	}
}

func execute(req pluginprotocol.ExecutionRequest) pluginprotocol.ExecutionResult {
	base := pluginprotocol.ExecutionResult{
		Type:          "execution.result",
		SchemaVersion: pluginprotocol.SchemaVersion,
		ExecutionID:   req.ExecutionID,
	}
	if req.Capability != capabilityOSCSend {
		base.Status = "FAILED"
		base.ErrorCode = "CAPABILITY_UNAVAILABLE"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = "unsupported capability"
		return base
	}

	var p oscParams
	if err := json.Unmarshal(req.Parameters, &p); err != nil {
		base.Status = "FAILED"
		base.ErrorCode = "OSC_INVALID_PARAMETERS"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = "invalid OSC parameters"
		return base
	}
	if _, err := osc.EncodeMessage(osc.Message{Address: p.Address, Arguments: p.Arguments}); err != nil {
		base.Status = "FAILED"
		base.ErrorCode = "OSC_INVALID_PARAMETERS"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = err.Error()
		return base
	}

	timeoutMS := req.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 500
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	sendResult, err := (osc.Sender{WriteTimeout: time.Duration(timeoutMS) * time.Millisecond}).Send(
		ctx,
		osc.Endpoint{Host: req.Target.Host, Port: req.Target.Port},
		osc.Message{Address: p.Address, Arguments: p.Arguments},
	)
	if err != nil {
		switch sendResult.Status {
		case "TIMED_OUT":
			base.Status = "TIMED_OUT"
			base.ErrorCode = "TIMEOUT"
			base.ErrorCategory = "TIMEOUT"
		case "CANCELLED":
			base.Status = "CANCELLED"
			base.ErrorCode = "CANCELLED"
			base.ErrorCategory = "NETWORK"
		default:
			base.Status = "FAILED"
			base.ErrorCode = "OSC_SEND_FAILED"
			base.ErrorCategory = "NETWORK"
		}
		base.ErrorMessage = err.Error()
		return base
	}
	base.Status = sendResult.Status
	base.AckLevel = sendResult.AckLevel
	return base
}

func write(out *bufio.Writer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintln(out, string(b))
	_ = out.Flush()
}
