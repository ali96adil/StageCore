package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
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
	inputOSCReceive   = "osc.receive"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--receive" {
		if err := runReceive(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	runSend()
}

func runSend() {
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

func runReceive(args []string) error {
	flags := flag.NewFlagSet("receive", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	listen := flags.String("listen", "127.0.0.1:0", "OSC UDP listen address")
	if err := flags.Parse(args); err != nil {
		return err
	}

	address, err := net.ResolveUDPAddr("udp", *listen)
	if err != nil {
		return fmt.Errorf("resolve OSC receive address: %w", err)
	}
	if address.IP == nil || !address.IP.IsLoopback() {
		return fmt.Errorf("OSC receive is loopback-only until Stage LAN security gate passes")
	}
	conn, err := net.ListenUDP("udp", address)
	if err != nil {
		return fmt.Errorf("listen OSC receive: %w", err)
	}
	defer conn.Close()

	out := bufio.NewWriter(os.Stdout)
	write(out, pluginprotocol.Ready{
		Type:          "plugin.ready",
		SchemaVersion: pluginprotocol.SchemaVersion,
		PluginID:      pluginID,
		PluginVersion: "0.1.0",
		Inputs:        []string{inputOSCReceive},
		ListenAddress: conn.LocalAddr().String(),
	})

	buffer := make([]byte, 64*1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return fmt.Errorf("read OSC receive datagram: %w", err)
		}
		message, err := osc.DecodeMessage(buffer[:n])
		if err != nil {
			// A malformed UDP datagram is isolated to itself. It must not kill the
			// external Plugin process or invent an input event.
			continue
		}
		value, err := inputValue(message.Arguments)
		if err != nil {
			continue
		}
		write(out, pluginprotocol.InputEvent{
			Type:          "input.event",
			SchemaVersion: pluginprotocol.SchemaVersion,
			InputType:     inputOSCReceive,
			Source:        message.Address,
			Value:         value,
		})
	}
}

func inputValue(arguments []osc.Argument) (json.RawMessage, error) {
	values := make([]any, 0, len(arguments))
	for _, argument := range arguments {
		values = append(values, argument.Value)
	}
	var value any
	switch len(values) {
	case 0:
		value = true
	case 1:
		value = values[0]
	default:
		value = values
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return raw, nil
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
