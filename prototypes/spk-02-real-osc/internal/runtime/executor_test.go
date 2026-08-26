package runtime

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/prototypes/spk-02-real-osc/internal/osc"
)

func TestExecuteResolvesLogicalTargetAndSendsOnce(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	endpoint := osc.Endpoint{ID: "VIDEO-MAIN", Host: "127.0.0.1", Port: conn.LocalAddr().(*net.UDPAddr).Port}
	executor := Executor{Endpoints: map[string]osc.Endpoint{endpoint.ID: endpoint}}
	action := Action{ID: "action-1", Capability: "osc.send", TargetID: "VIDEO-MAIN", Parameters: osc.Message{Address: "/cue/go"}}

	if err := ValidateAction(action, executor.Endpoints); err != nil {
		t.Fatal(err)
	}
	result := executor.Execute(context.Background(), action)
	if result.Status != "COMPLETED" || result.AckLevel != "TRANSPORT_ONLY" {
		t.Fatalf("unexpected result: %#v", result)
	}

	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := osc.DecodeMessage(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	if msg.Address != "/cue/go" {
		t.Fatalf("address=%q", msg.Address)
	}

	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := conn.ReadFromUDP(buf); err == nil {
		t.Fatal("unexpected duplicate OSC packet")
	}
}

func TestExecuteMissingLogicalTargetFailsBeforeNetwork(t *testing.T) {
	executor := Executor{Endpoints: map[string]osc.Endpoint{}}
	result := executor.Execute(context.Background(), Action{ID: "action-1", Capability: "osc.send", TargetID: "VIDEO-MAIN", Parameters: osc.Message{Address: "/cue/go"}})
	if result.Status != "FAILED" || result.AckLevel != "NONE" || result.Error == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
