package devicechannel_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"golang.org/x/net/websocket"
)

func connectLightingNode(t *testing.T, f *runtimeFixture) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	hello := map[string]any{
		"type":             "device.hello",
		"schema_version":   1,
		"device_id":        testDeviceID,
		"project_id":       f.projectID,
		"profile_id":       lightingnode.ProfileID,
		"device_kind":      deviceexperience.DeviceGeneric,
		"display_name":     "ESP32 DMX Lighting 01",
		"platform":         "esp32",
		"architecture":     "xtensa",
		"client_version":   "0.1.0-test",
		"protocol_version": deviceexperience.ProtocolVersion1,
		"capabilities":     lightingnode.CapabilityKeys(),
		"readiness":        deviceexperience.ReadinessReady,
		"observed_state": json.RawMessage(`{
			"schema_version":1,
			"current_levels":{"warm_a":0},
			"dmx_healthy":true,
			"authority":"STAGECORE"
		}`),
		"network_state": json.RawMessage(`{"transport":"TLS_WEBSOCKET"}`),
	}
	if err := websocket.JSON.Send(ws, hello); err != nil {
		t.Fatal(err)
	}
	var ready struct {
		Type     string `json:"type"`
		DeviceID string `json:"device_id"`
	}
	if err := websocket.JSON.Receive(ws, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Type != "runtime.ready" || ready.DeviceID != testDeviceID {
		t.Fatalf("ready=%+v", ready)
	}
	return ws
}

func TestLightingFadeAllowsIntermediateAcceptedThenTerminalCompleted(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	ws := connectLightingNode(t, f)
	defer ws.Close()

	deadline := time.Now().Add(3 * time.Second)
	command, err := f.runtime.Dispatch(ctx, deviceexperience.CreateCommandInput{
		ProjectID: f.projectID, DeviceID: testDeviceID,
		CommandType: lightingnode.CommandChannelsFade, Issuer: "operator:test",
		CorrelationID: "lighting-fade-correlation",
		IdempotencyKey: "lighting-fade-1",
		Payload: json.RawMessage(`{"fade_ms":500,"channels":{"warm_a":75}}`),
		DeadlineAt: &deadline,
	})
	if err != nil {
		t.Fatal(err)
	}

	var execute struct {
		Type     string `json:"type"`
		DeviceID string `json:"device_id"`
		Command  contracts.CommandEnvelope `json:"command"`
	}
	if err := websocket.JSON.Receive(ws, &execute); err != nil {
		t.Fatal(err)
	}
	if execute.Type != "command.execute" ||
		execute.DeviceID != testDeviceID ||
		execute.Command.CommandID != command.Envelope.CommandID ||
		execute.Command.CommandType != lightingnode.CommandChannelsFade {
		t.Fatalf("execute=%+v", execute)
	}

	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "command.result", "schema_version": 1, "device_id": testDeviceID,
		"command_id": command.Envelope.CommandID, "status": contracts.CommandAccepted,
	}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Millisecond)
	accepted, err := f.repo.GetCommand(ctx, command.Envelope.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != contracts.CommandAccepted || accepted.CompletedAt != nil {
		t.Fatalf("intermediate ACK terminalized command: %+v", accepted)
	}
	if !f.runtime.IsConnected(testDeviceID) {
		t.Fatal("intermediate ACCEPTED closed the Stage Device connection")
	}

	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "command.result", "schema_version": 1, "device_id": testDeviceID,
		"command_id": command.Envelope.CommandID, "status": contracts.CommandCompleted,
		"payload": map[string]any{"levels": map[string]float64{"warm_a": 75}},
	}); err != nil {
		t.Fatal(err)
	}

	var completed deviceexperience.DeviceCommand
	waitUntil := time.Now().Add(time.Second)
	for {
		completed, err = f.repo.GetCommand(ctx, command.Envelope.CommandID)
		if err != nil {
			t.Fatal(err)
		}
		if completed.Status == contracts.CommandCompleted {
			break
		}
		if time.Now().After(waitUntil) {
			t.Fatalf("command never completed: %+v", completed)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if completed.CompletedAt == nil {
		t.Fatalf("completed command missing completed_at: %+v", completed)
	}
	var result contracts.CommandResult
	if err := json.Unmarshal(completed.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != contracts.CommandCompleted || !strings.Contains(string(result.Payload), "warm_a") {
		t.Fatalf("terminal lighting result=%+v raw=%s", result, completed.Result)
	}
	if !f.runtime.IsConnected(testDeviceID) {
		t.Fatal("terminal lighting result unexpectedly closed the Stage Device connection")
	}
}
