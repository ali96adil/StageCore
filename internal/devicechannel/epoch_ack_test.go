package devicechannel_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"golang.org/x/net/websocket"
)

// This test uses a simulated, authenticated v2 software-zero ACK. It cannot
// assert physical LED or DMX decoder darkness.
func prepareBlockedEpoch(t *testing.T, f *runtimeFixture) {
	t.Helper()
	ws := connectLightingV2ForBlackout(t, f)
	done := make(chan error, 1)
	go func() {
		_, err := f.runtime.ExecuteReservedSoftwareTransfer(context.Background(),
			deviceexperience.TransferPreflightInput{
				DeviceID: testDeviceID, TargetProjectID: f.projectID, ExpectedEpoch: 1,
			}, "owner")
		done <- err
	}()
	_ = ws.SetReadDeadline(time.Now().Add(4 * time.Second))
	var msg struct {
		Type string `json:"type"`
		TransferID string `json:"transfer_id"`
		Challenge string `json:"challenge"`
		AssignmentEpoch int64 `json:"assignment_epoch"`
		ConnectionGeneration int64 `json:"connection_generation"`
	}
	err := websocket.JSON.Receive(ws, &msg)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || msg.Type != "assignment.blackout" {
		t.Fatalf("simulated transfer request=%+v err=%v", msg, err)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "assignment.blackout_ack", "schema_version": 2,
		"device_id": testDeviceID, "transfer_id": msg.TransferID,
		"challenge": msg.Challenge, "assignment_epoch": msg.AssignmentEpoch,
		"connection_generation": msg.ConnectionGeneration,
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("prepare BLOCKED epoch: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("software transfer did not complete")
	}
}

func connectBlockedEpoch(t *testing.T, f *runtimeFixture) (*websocket.Conn, int64) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.hello", "schema_version": 1, "device_id": testDeviceID,
		"project_id": "", "device_kind": deviceexperience.DeviceGeneric,
		"profile_id": lightingnode.ProfileID, "display_name": "Simulated Lighting Node",
		"platform": "esp32", "client_version": "test-v2",
		"protocol_version": deviceexperience.ProtocolVersion2,
		"capabilities": lightingnode.CapabilityKeys(),
		"readiness": deviceexperience.ReadinessBlocker,
	}); err != nil {
		_ = ws.Close()
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var assignment struct {
		Type string `json:"type"`
		State string `json:"state"`
		ProjectID string `json:"project_id"`
		AssignmentEpoch int64 `json:"assignment_epoch"`
		ConnectionGeneration int64 `json:"connection_generation"`
		BlackoutRequired bool `json:"blackout_required"`
		CommandsEnabled bool `json:"commands_enabled"`
		EpochAckRequired bool `json:"epoch_ack_required"`
	}
	err = websocket.JSON.Receive(ws, &assignment)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || assignment.Type != "assignment.state" ||
		assignment.State != "BLOCKED" || assignment.ProjectID != f.projectID ||
		assignment.AssignmentEpoch != 2 || assignment.ConnectionGeneration < 1 ||
		!assignment.BlackoutRequired || assignment.CommandsEnabled || !assignment.EpochAckRequired {
		_ = ws.Close()
		t.Fatalf("reconnect did not remain BLOCKED: %+v err=%v", assignment, err)
	}
	return ws, assignment.ConnectionGeneration
}

func TestBlockedEpochACKPersistsOnlyAfterCurrentSocketReportsAllZero(t *testing.T) {
	f := newRuntimeFixture(t)
	prepareBlockedEpoch(t, f)
	if _, err := f.repo.GetBlockedEpochAck(context.Background(), testDeviceID, 2); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Hub guessed an epoch ACK before new socket report: %v", err)
	}

	for _, tc := range []struct {
		name string
		mutate func(map[string]any)
	}{
		{"stale epoch", func(a map[string]any) { a["assignment_epoch"] = 1 }},
		{"stale generation", func(a map[string]any) { a["connection_generation"] = 0 }},
		{"wrong project", func(a map[string]any) { a["project_id"] = "other-project" }},
		{"incomplete logical output", func(a map[string]any) { a["channel_levels"] = make([]int, lightingnode.MaxChannels-1) }},
		{"nonzero logical output", func(a map[string]any) {
			l := make([]int, lightingnode.MaxChannels)
			l[4] = 2
			a["channel_levels"] = l
		}},
		{"no blackout", func(a map[string]any) { a["blackout"] = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, generation := connectBlockedEpoch(t, f)
			defer ws.Close()
			ack := map[string]any{
				"type": "assignment.epoch_ack", "schema_version": 2,
				"device_id": testDeviceID, "project_id": f.projectID,
				"assignment_epoch": 2, "connection_generation": generation,
				"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
			}
			tc.mutate(ack)
			if err := websocket.JSON.Send(ws, ack); err != nil {
				t.Fatal(err)
			}
			_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
			var receipt map[string]any
			if err := websocket.JSON.Receive(ws, &receipt); err == nil {
				t.Fatalf("invalid epoch ACK received persisted receipt: %+v", receipt)
			}
			_ = ws.SetReadDeadline(time.Time{})
			if _, err := f.repo.GetBlockedEpochAck(context.Background(), testDeviceID, 2); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("invalid epoch ACK persisted: %v", err)
			}
		})
	}

	ws, generation := connectBlockedEpoch(t, f)
	defer ws.Close()
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "assignment.epoch_ack", "schema_version": 2,
		"device_id": testDeviceID, "project_id": f.projectID,
		"assignment_epoch": 2, "connection_generation": generation,
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var receipt struct {
		Type string `json:"type"`
		State string `json:"state"`
		AssignmentEpoch int64 `json:"assignment_epoch"`
		ConnectionGeneration int64 `json:"connection_generation"`
		CommandsEnabled bool `json:"commands_enabled"`
		Persisted bool `json:"persisted"`
	}
	err := websocket.JSON.Receive(ws, &receipt)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || receipt.Type != "assignment.epoch_ack_receipt" ||
		receipt.State != "BLOCKED" || receipt.AssignmentEpoch != 2 ||
		receipt.ConnectionGeneration != generation || !receipt.Persisted ||
		receipt.CommandsEnabled {
		t.Fatalf("valid software-zero receipt=%+v err=%v", receipt, err)
	}
	record, err := f.repo.GetBlockedEpochAck(context.Background(), testDeviceID, 2)
	if err != nil || record.ProjectID != f.projectID || record.ConnectionGeneration != generation ||
		record.ChannelCount != lightingnode.MaxChannels {
		t.Fatalf("authenticated epoch ACK not persisted: %+v err=%v", record, err)
	}
	assignment, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
	if err != nil || assignment.State != "BLOCKED" || assignment.Epoch != 2 ||
		assignment.RuntimeSnapshotID != "" {
		t.Fatalf("epoch ACK wrongly activated Project/snapshot: %+v err=%v", assignment, err)
	}
	if _, _, err := f.repo.CreateCommand(context.Background(), deviceexperience.CreateCommandInput{
		DeviceID: testDeviceID, ProjectID: f.projectID,
		CommandType: lightingnode.CommandBlackout, Issuer: "operator",
	}); err == nil {
		t.Fatal("epoch ACK enabled legacy command dispatch")
	}
}

func TestV2EpochAckCannotBeMistakenForCurrentAfterHubRestart(t *testing.T) {
	f := newRuntimeFixture(t)
	prepareBlockedEpoch(t, f)

	first, generationBefore := connectBlockedEpoch(t, f)
	if err := websocket.JSON.Send(first, map[string]any{
		"type": "assignment.epoch_ack", "schema_version": 2,
		"device_id": testDeviceID, "project_id": f.projectID,
		"assignment_epoch": 2, "connection_generation": generationBefore,
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}); err != nil {
		t.Fatal(err)
	}
	_ = first.SetReadDeadline(time.Now().Add(3 * time.Second))
	var receipt map[string]any
	err := websocket.JSON.Receive(first, &receipt)
	_ = first.SetReadDeadline(time.Time{})
	if err != nil || receipt["type"] != "assignment.epoch_ack_receipt" {
		t.Fatalf("original software-zero receipt=%+v err=%v", receipt, err)
	}
	stored, err := f.repo.GetBlockedEpochAck(context.Background(), testDeviceID, 2)
	if err != nil || stored.ConnectionGeneration != generationBefore {
		t.Fatalf("missing original persisted ACK=%+v err=%v", stored, err)
	}
	_ = first.Close()
	f.runtime.Close()
	f.server.Close()

	// Reconstruct Hub Runtime over the SAME durable database. Previously
	// nextGeneration reset to zero and could match the persisted old ACK,
	// falsely marking it as reported on the newly authenticated socket.
	f.runtime = devicechannel.New(f.repo, f.auth)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.runtime.ServeWebSocket(w, r, f.session, f.token)
	}))
	next, generationAfter := connectBlockedEpoch(t, f)
	defer next.Close()
	if generationAfter <= generationBefore {
		t.Fatalf("Hub reboot reused an old persisted epoch ACK generation: before=%d after=%d",
			generationBefore, generationAfter)
	}
	historical, err := f.repo.GetBlockedEpochAck(context.Background(), testDeviceID, 2)
	if err != nil || historical.ConnectionGeneration != generationBefore ||
		historical.ConnectionGeneration == generationAfter {
		t.Fatalf("stale report appeared current after restart: %+v err=%v", historical, err)
	}
	if err := websocket.JSON.Send(next, map[string]any{
		"type": "assignment.epoch_ack", "schema_version": 2,
		"device_id": testDeviceID, "project_id": f.projectID,
		"assignment_epoch": 2, "connection_generation": generationAfter,
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}); err != nil {
		t.Fatal(err)
	}
	_ = next.SetReadDeadline(time.Now().Add(3 * time.Second))
	err = websocket.JSON.Receive(next, &receipt)
	_ = next.SetReadDeadline(time.Time{})
	if err != nil || receipt["type"] != "assignment.epoch_ack_receipt" {
		t.Fatalf("fresh post-restart software-zero receipt=%+v err=%v", receipt, err)
	}
	fresh, err := f.repo.GetBlockedEpochAck(context.Background(), testDeviceID, 2)
	if err != nil || fresh.ConnectionGeneration != generationAfter {
		t.Fatalf("current report not persisted after restart: %+v err=%v", fresh, err)
	}
	assignment, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
	if err != nil || assignment.State != "BLOCKED" || assignment.Epoch != 2 ||
		assignment.RuntimeSnapshotID != "" {
		t.Fatalf("reboot/epoch ACK activated outputs: %+v err=%v", assignment, err)
	}
}
