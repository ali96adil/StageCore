package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-04-plugin-ipc/internal/osc"
	"github.com/ali96adil/StageCore/prototypes/spk-04-plugin-ipc/internal/protocol"
)

type oscParams struct {
	Address   string `json:"address"`
	Arguments []any  `json:"arguments"`
}

func main() {
	out := bufio.NewWriter(os.Stdout)
	write(out, protocol.Ready{Type: "plugin.ready", SchemaVersion: protocol.SchemaVersion, PluginID: "stagecore.osc", PluginVersion: "0.1.0-spike", Capabilities: []string{"osc.send"}})

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req protocol.ExecutionRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		if applyTestFault() {
			return
		}
		started := time.Now()
		result := execute(req)
		result.DurationMS = time.Since(started).Milliseconds()
		write(out, result)
	}
}

func execute(req protocol.ExecutionRequest) protocol.ExecutionResult {
	base := protocol.ExecutionResult{Type: "execution.result", SchemaVersion: protocol.SchemaVersion, ExecutionID: req.ExecutionID}
	if req.Capability != "osc.send" {
		base.Status = "FAILED"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = "unsupported capability"
		return base
	}
	var p oscParams
	if err := json.Unmarshal(req.Parameters, &p); err != nil {
		base.Status = "FAILED"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = "invalid parameters"
		return base
	}
	packet, err := osc.Encode(p.Address, p.Arguments)
	if err != nil {
		base.Status = "FAILED"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = err.Error()
		return base
	}
	if req.Target.Host == "" || req.Target.Port <= 0 || req.Target.Port > 65535 {
		base.Status = "FAILED"
		base.ErrorCategory = "VALIDATION"
		base.ErrorMessage = "invalid resolved target"
		return base
	}
	conn, err := net.DialTimeout("udp", net.JoinHostPort(req.Target.Host, strconv.Itoa(req.Target.Port)), 250*time.Millisecond)
	if err != nil {
		base.Status = "FAILED"
		base.ErrorCategory = "NETWORK"
		base.ErrorMessage = err.Error()
		return base
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
	if _, err := conn.Write(packet); err != nil {
		base.Status = "FAILED"
		base.ErrorCategory = "NETWORK"
		base.ErrorMessage = err.Error()
		return base
	}
	base.Status = "COMPLETED"
	base.AckLevel = "TRANSPORT_ONLY"
	return base
}

func write(out *bufio.Writer, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintln(out, string(b))
	_ = out.Flush()
}

func applyTestFault() bool {
	mode := os.Getenv("STAGECORE_PLUGIN_TEST_FAULT")
	marker := os.Getenv("STAGECORE_PLUGIN_TEST_MARKER")
	if mode == "" || marker == "" {
		return false
	}
	if _, err := os.Stat(marker); err == nil {
		return false
	}
	_ = os.WriteFile(marker, []byte("triggered"), 0600)
	switch mode {
	case "crash-once":
		os.Exit(41)
	case "hang-once":
		select {}
	}
	return false
}
