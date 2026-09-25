package devicechannel_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"golang.org/x/net/websocket"
)

func TestQualificationExchangeUsesLiveConnectionWithoutProductionPersistence(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := f.connect(t)
	defer ws.Close()

	now := time.Now().UTC()
	deadline := now.Add(10 * time.Second)
	envelope := contracts.CommandEnvelope{
		CommandID:     "qualification-runtime-duplicate",
		CommandType:   "LIGHTING_CHANNELS_FADE",
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      now,
		DeadlineAt:    &deadline,
		ProjectID:     f.projectID,
		Issuer:        "qualification:physical-runner",
		Priority:      "P2",
		Payload:       json.RawMessage(`{"fade_ms":1000,"channels":{"warm":50}}`),
	}

	deviceDone := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			var execute struct {
				Type     string                    `json:"type"`
				DeviceID string                    `json:"device_id"`
				Command  contracts.CommandEnvelope `json:"command"`
			}
			if err := websocket.JSON.Receive(ws, &execute); err != nil {
				deviceDone <- err
				return
			}
			if execute.Type != "command.execute" || execute.Command.CommandID != envelope.CommandID {
				deviceDone <- context.Canceled
				return
			}
			if i == 0 {
				if err := websocket.JSON.Send(ws, map[string]any{
					"type": "command.result", "schema_version": 1, "device_id": testDeviceID,
					"command_id": envelope.CommandID, "status": contracts.CommandAccepted,
				}); err != nil {
					deviceDone <- err
					return
				}
			}
			if err := websocket.JSON.Send(ws, map[string]any{
				"type": "command.result", "schema_version": 1, "device_id": testDeviceID,
				"command_id": envelope.CommandID, "status": contracts.CommandCompleted,
				"payload": map[string]any{"levels": map[string]float64{"warm": 50}},
			}); err != nil {
				deviceDone <- err
				return
			}
		}
		deviceDone <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	first, err := f.runtime.QualificationExchange(ctx, testDeviceID, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Statuses) != 2 || first.Statuses[0] != contracts.CommandAccepted || first.Statuses[1] != contracts.CommandCompleted {
		t.Fatalf("first qualification lifecycle=%v", first.Statuses)
	}
	second, err := f.runtime.QualificationExchange(ctx, testDeviceID, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Statuses) != 1 || second.Statuses[0] != contracts.CommandCompleted {
		t.Fatalf("duplicate qualification lifecycle=%v", second.Statuses)
	}
	if err := <-deviceDone; err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.GetCommand(context.Background(), envelope.CommandID); err == nil {
		t.Fatal("qualification envelope leaked into production command persistence")
	}
}
