package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/ali96adil/StageCore/prototypes/spk-02-real-osc/internal/osc"
	"github.com/ali96adil/StageCore/prototypes/spk-02-real-osc/internal/runtime"
)

func main() {
	host := flag.String("host", "127.0.0.1", "OSC receiver host")
	port := flag.Int("port", 9000, "OSC receiver UDP port")
	address := flag.String("address", "/stagecore/demo", "OSC address")
	flag.Parse()

	endpoint := osc.Endpoint{ID: "VIDEO-MAIN", Host: *host, Port: *port}
	executor := runtime.Executor{Endpoints: map[string]osc.Endpoint{endpoint.ID: endpoint}}
	action := runtime.Action{
		ID:         "demo-action",
		Capability: "osc.send",
		TargetID:   "VIDEO-MAIN",
		Parameters: osc.Message{Address: *address, Arguments: []osc.Argument{{Type: "string", Value: "StageCore"}}},
	}
	if err := runtime.ValidateAction(action, executor.Endpoints); err != nil {
		log.Fatal(err)
	}
	result := executor.Execute(context.Background(), action)
	fmt.Printf("status=%s ack=%s bytes=%d error=%q\n", result.Status, result.AckLevel, result.BytesSent, result.Error)
	if result.Status != "COMPLETED" {
		log.Fatal("OSC demo failed")
	}
}
