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

func main() {
	out := bufio.NewWriter(os.Stdout)
	write(out, pluginprotocol.Ready{
		Type:          "plugin.ready",
		SchemaVersion: pluginprotocol.SchemaVersion,
		PluginID:      "stagecore.osc",
		PluginVersion: "test-helper",
		Capabilities:  []string{"osc.send"},
	})

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req pluginprotocol.ExecutionRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		applyFaultOnce()
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
	var p oscParams
	if err := json.Unmarshal(req.Parameters, &p); err != nil {
		base.Status = "FAILED"
		base.ErrorCode = "OSC_INVALID_PARAMETERS"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = "invalid OSC parameters"
		return base
	}

	timeoutMS := req.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 500
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	sent, err := (osc.Sender{WriteTimeout: time.Duration(timeoutMS) * time.Millisecond}).Send(
		ctx,
		osc.Endpoint{Host: req.Target.Host, Port: req.Target.Port},
		osc.Message{Address: p.Address, Arguments: p.Arguments},
	)
	if err != nil {
		base.Status = "FAILED"
		base.ErrorCode = "OSC_SEND_FAILED"
		base.ErrorCategory = "NETWORK"
		base.ErrorMessage = err.Error()
		return base
	}
	base.Status = sent.Status
	base.AckLevel = sent.AckLevel
	return base
}

func applyFaultOnce() {
	mode := os.Getenv("STAGECORE_PLUGIN_TEST_FAULT")
	marker := os.Getenv("STAGECORE_PLUGIN_TEST_MARKER")
	if mode == "" || marker == "" {
		return
	}
	if _, err := os.Stat(marker); err == nil {
		return
	}
	_ = os.WriteFile(marker, []byte("triggered"), 0o600)
	switch mode {
	case "crash-once":
		os.Exit(41)
	case "hang-once":
		select {}
	}
}

func write(out *bufio.Writer, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	fmt.Fprintln(out, string(data))
	_ = out.Flush()
}
