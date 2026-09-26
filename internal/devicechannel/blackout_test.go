package devicechannel_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"golang.org/x/net/websocket"
)

func connectLightingV2ForBlackout(t *testing.T, f *runtimeFixture) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	hello := map[string]any{
		"type": "device.hello", "schema_version": 1,
		"device_id": testDeviceID, "project_id": "",
		"device_kind": deviceexperience.DeviceGeneric,
		"profile_id": lightingnode.ProfileID, "display_name": "Simulated Lighting Node",
		"platform": "esp32", "client_version": "test-v2",
		"protocol_version": deviceexperience.ProtocolVersion2,
		"capabilities": lightingnode.CapabilityKeys(),
		"readiness": deviceexperience.ReadinessReady,
	}
	if err := websocket.JSON.Send(ws, hello); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var welcome map[string]any
	err = websocket.JSON.Receive(ws, &welcome)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || welcome["type"] != "assignment.state" ||
		welcome["state"] != "UNASSIGNED" || welcome["commands_enabled"] != false {
		t.Fatalf("invalid v2 blackout-only welcome=%+v err=%v", welcome, err)
	}
	return ws
}

func reserveBlackoutFixture(t *testing.T, f *runtimeFixture) deviceexperience.TransferReservation {
	t.Helper()
	generation, ok := f.runtime.CurrentV2Generation(testDeviceID)
	if !ok || generation < 1 {
		t.Fatalf("no authenticated v2 socket generation=%d ok=%v", generation, ok)
	}
	res, err := f.repo.ReserveTransferIntent(context.Background(),
		deviceexperience.TransferPreflightInput{
			DeviceID: testDeviceID, ExpectedEpoch: 1, TargetProjectID: f.projectID,
		}, generation, lightingnode.MaxChannels, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestV2SoftwareBlackoutAcknowledgesAllChannelsButNeverCommitsProject(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := connectLightingV2ForBlackout(t, f)
	res := reserveBlackoutFixture(t, f)
	type result struct {
		got devicechannel.SoftwareBlackoutConfirmation
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := f.runtime.RequestSoftwareBlackout(context.Background(), res)
		done <- result{got, err}
	}()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var request map[string]any
	err := websocket.JSON.Receive(ws, &request)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || request["type"] != "assignment.blackout" ||
		request["challenge"] != res.Challenge || request["transfer_id"] != res.TransferID ||
		request["connection_generation"] != float64(res.ConnectionGeneration) ||
		request["assignment_epoch"] != float64(res.ExpectedEpoch) ||
		request["expected_channels"] != float64(res.ExpectedChannels) {
		t.Fatalf("wrong request=%+v err=%v", request, err)
	}
	ack := map[string]any{
		"type": "assignment.blackout_ack", "schema_version": 2, "device_id": testDeviceID,
		"transfer_id": res.TransferID, "assignment_epoch": res.ExpectedEpoch,
		"connection_generation": res.ConnectionGeneration, "challenge": res.Challenge,
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}
	if err := websocket.JSON.Send(ws, ack); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-done:
		if completed.err != nil || completed.got.TransferID != res.TransferID ||
			len(completed.got.ChannelLevels) != lightingnode.MaxChannels {
			t.Fatalf("software ACK=%+v err=%v", completed.got, completed.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("valid simulated software ACK did not complete")
	}
	record, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
	if err != nil || record.State != "UNASSIGNED" || record.ProjectID != "" || record.Epoch != 1 {
		t.Fatalf("software blackout incorrectly granted project authority: %+v err=%v", record, err)
	}
}

func TestV2SoftwareBlackoutRejectsForgedOrIncompleteAcknowledgments(t *testing.T) {
	for _, tc := range []struct {
		name string
		mutate func(map[string]any)
	}{
		{"wrong nonce", func(a map[string]any) { a["challenge"] = strings.Repeat("0", 64) }},
		{"wrong epoch", func(a map[string]any) { a["assignment_epoch"] = 2 }},
		{"wrong generation", func(a map[string]any) { a["connection_generation"] = 999 }},
		{"wrong transfer", func(a map[string]any) { a["transfer_id"] = "other" }},
		{"not blackout", func(a map[string]any) { a["blackout"] = false }},
		{"partial channels", func(a map[string]any) { a["channel_levels"] = make([]int, lightingnode.MaxChannels-1) }},
		{"nonzero channel", func(a map[string]any) {
			levels := make([]int, lightingnode.MaxChannels)
			levels[3] = 1
			a["channel_levels"] = levels
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeFixture(t)
			ws := connectLightingV2ForBlackout(t, f)
			res := reserveBlackoutFixture(t, f)
			done := make(chan error, 1)
			go func() {
				_, err := f.runtime.RequestSoftwareBlackout(context.Background(), res)
				done <- err
			}()
			_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
			var request map[string]any
			if err := websocket.JSON.Receive(ws, &request); err != nil {
				t.Fatal(err)
			}
			_ = ws.SetReadDeadline(time.Time{})
			ack := map[string]any{
				"type": "assignment.blackout_ack", "schema_version": 2, "device_id": testDeviceID,
				"transfer_id": res.TransferID, "assignment_epoch": res.ExpectedEpoch,
				"connection_generation": res.ConnectionGeneration, "challenge": res.Challenge,
				"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
			}
			tc.mutate(ack)
			if err := websocket.JSON.Send(ws, ack); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, devicechannel.ErrBlackoutNotVerified) {
					t.Fatalf("invalid ACK passed: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("invalid ACK was not rejected")
			}
			record, err := f.repo.GetAssignmentRecord(context.Background(), testDeviceID)
			if err != nil || record.State != "UNASSIGNED" || record.ProjectID != "" {
				t.Fatalf("rejected ACK changed project authority: %+v err=%v", record, err)
			}
		})
	}
}

func TestV2SoftwareBlackoutRejectsStaleConnectionGeneration(t *testing.T) {
	f := newRuntimeFixture(t)
	first := connectLightingV2ForBlackout(t, f)
	res := reserveBlackoutFixture(t, f)
	second := connectLightingV2ForBlackout(t, f)
	defer second.Close()
	defer first.Close()
	next, ok := f.runtime.CurrentV2Generation(testDeviceID)
	if !ok || next <= res.ConnectionGeneration {
		t.Fatalf("replacement connection generation=%d old=%d ok=%v", next, res.ConnectionGeneration, ok)
	}
	if _, err := f.runtime.RequestSoftwareBlackout(context.Background(), res); !errors.Is(err, devicechannel.ErrBlackoutNotVerified) {
		t.Fatalf("stale transport was allowed to initiate blackout: %v", err)
	}
	_ = second.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var unexpected map[string]any
	if err := websocket.JSON.Receive(second, &unexpected); err == nil {
		t.Fatalf("stale transfer sent to replacement socket: %+v", unexpected)
	}
}

