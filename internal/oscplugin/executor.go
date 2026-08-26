package oscplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
)

const (
	PluginID             = "stagecore.osc"
	CapabilityOSCSend    = "osc.send"
	InputOSCReceive      = "osc.receive"
	PermissionUDPSend    = "network.udp.send"
	PermissionUDPListen  = "network.udp.listen"
)

type Executor struct {
	host *pluginhost.Host
}

func New(host *pluginhost.Host) *Executor {
	return &Executor{host: host}
}

type targetConfig struct {
	OSC struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"osc"`
}

func (e *Executor) Execute(ctx context.Context, req capability.Request) capability.Result {
	if req.Capability != CapabilityOSCSend {
		return failed("CAPABILITY_UNAVAILABLE", "OSC executor received unsupported capability")
	}
	if e == nil || e.host == nil {
		return failed("PLUGIN_UNAVAILABLE", "OSC plugin host is unavailable")
	}
	if req.Target == nil {
		return failed("TARGET_NOT_FOUND", "logical target is not present in Runtime Snapshot")
	}

	var cfg targetConfig
	if err := json.Unmarshal(req.Target.Configuration, &cfg); err != nil {
		return failed("TARGET_CONFIG_INVALID", "OSC target configuration is invalid")
	}
	if strings.TrimSpace(cfg.OSC.Host) == "" || cfg.OSC.Port < 1 || cfg.OSC.Port > 65535 {
		return failed("TARGET_CONFIG_INVALID", "OSC target requires host and port 1..65535")
	}

	timeoutMS := req.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 500
	}
	result, err := e.host.Execute(ctx, pluginprotocol.ExecutionRequest{
		Type:          "execution.request",
		SchemaVersion: pluginprotocol.SchemaVersion,
		ExecutionID:   req.ExecutionID,
		Capability:    req.Capability,
		Target:        pluginprotocol.ResolvedTarget{Host: cfg.OSC.Host, Port: cfg.OSC.Port},
		Parameters:    req.Parameters,
		Priority:      req.Priority,
		TimeoutMS:     timeoutMS,
		CorrelationID: req.CorrelationID,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return capability.Result{Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone, ErrorCode: "CANCELLED", ResponseSummary: "OSC plugin execution cancelled"}
		}
		if errors.Is(err, pluginhost.ErrPluginTimeout) {
			return capability.Result{Result: domain.ExecutionTimedOut, AckLevel: contracts.AckNone, ErrorCode: "TIMEOUT", ResponseSummary: "OSC plugin execution timed out"}
		}
		if errors.Is(err, pluginhost.ErrPluginPermissionDenied) {
			return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: "PLUGIN_PERMISSION_DENIED", ResponseSummary: "OSC plugin permission denied"}
		}
		code := result.ErrorCode
		if code == "" {
			code = "PLUGIN_FAILURE"
		}
		return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: code, ResponseSummary: safeSummary(result.ErrorMessage, err)}
	}

	switch result.Status {
	case "COMPLETED":
		ack := contracts.AckLevel(result.AckLevel)
		if ack != contracts.AckTransportOnly && ack != contracts.AckNone {
			return failed("PLUGIN_ACK_INVALID", "OSC plugin reported unsupported acknowledgement level")
		}
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: ack, ResponseSummary: "OSC UDP datagram accepted by local transport"}
	case "TIMED_OUT":
		return capability.Result{Result: domain.ExecutionTimedOut, AckLevel: contracts.AckNone, ErrorCode: nonEmpty(result.ErrorCode, "TIMEOUT"), ResponseSummary: safeSummary(result.ErrorMessage, nil)}
	case "CANCELLED":
		return capability.Result{Result: domain.ExecutionCancelled, AckLevel: contracts.AckNone, ErrorCode: nonEmpty(result.ErrorCode, "CANCELLED"), ResponseSummary: safeSummary(result.ErrorMessage, nil)}
	case "FAILED":
		return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: nonEmpty(result.ErrorCode, "OSC_SEND_FAILED"), ResponseSummary: safeSummary(result.ErrorMessage, nil)}
	default:
		return failed("PLUGIN_PROTOCOL_ERROR", fmt.Sprintf("unexpected OSC plugin status %q", result.Status))
	}
}

func failed(code, summary string) capability.Result {
	return capability.Result{Result: domain.ExecutionFailed, AckLevel: contracts.AckNone, ErrorCode: code, ResponseSummary: summary}
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func safeSummary(message string, err error) string {
	if strings.TrimSpace(message) != "" {
		return message
	}
	if err != nil {
		return err.Error()
	}
	return "OSC plugin execution failed"
}
