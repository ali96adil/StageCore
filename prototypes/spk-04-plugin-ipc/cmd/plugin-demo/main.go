package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-04-plugin-ipc/internal/pluginhost"
	"github.com/ali96adil/StageCore/prototypes/spk-04-plugin-ipc/internal/protocol"
)

func main() {
	if len(os.Args) < 2 {
		panic("usage: plugin-demo <path-to-stagecore-osc-plugin>")
	}
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	recv, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}
	defer recv.Close()
	params, _ := json.Marshal(map[string]any{"address": "/stagecore/spk04", "arguments": []any{"hello", float64(7)}})
	req := protocol.ExecutionRequest{Type: "execution.request", SchemaVersion: 1, ExecutionID: "exec-demo", Capability: "osc.send", Target: protocol.ResolvedTarget{Host: "127.0.0.1", Port: recv.LocalAddr().(*net.UDPAddr).Port}, Parameters: params, Priority: "P1", TimeoutMS: 500, CorrelationID: "corr-demo"}
	h := pluginhost.New(os.Args[1], nil, nil, os.Stderr)
	defer h.Close()
	res, err := h.Execute(context.Background(), req)
	if err != nil {
		panic(err)
	}
	_ = recv.SetReadDeadline(time.Now().Add(time.Second))
	b := make([]byte, 256)
	n, _, err := recv.ReadFromUDP(b)
	if err != nil {
		panic(err)
	}
	fmt.Printf("plugin=%s status=%s ack=%s bytes=%d\n", h.Ready().PluginID, res.Status, res.AckLevel, n)
}
