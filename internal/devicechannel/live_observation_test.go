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

func connectV2ReadOnlyProbe(t *testing.T, f *runtimeFixture, optIn bool) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	capabilities := append([]string(nil), lightingnode.CapabilityKeys()...)
	if optIn {
		capabilities = append(capabilities, devicechannel.V2LightingStateProbeCapability)
	}
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.hello", "schema_version": 1, "device_id": testDeviceID,
		"project_id": "", "device_kind": deviceexperience.DeviceGeneric,
		"profile_id": lightingnode.ProfileID, "display_name": "Read-only Probe Node",
		"platform": "esp32", "client_version": "probe-test",
		"protocol_version": deviceexperience.ProtocolVersion2,
		"capabilities": capabilities, "readiness": deviceexperience.ReadinessReady,
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var welcome map[string]any
	err = websocket.JSON.Receive(ws, &welcome)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || welcome["type"] != "assignment.state" ||
		welcome["state"] != "UNASSIGNED" || welcome["commands_enabled"] != false {
		t.Fatalf("v2 read-only welcome=%+v err=%v", welcome, err)
	}
	return ws
}

type probeResult struct {
	report devicechannel.V2SoftwareLevels
	err error
}

func startReadOnlyProbe(f *runtimeFixture, ctx context.Context) <-chan probeResult {
	done := make(chan probeResult, 1)
	go func() {
		got, err := f.runtime.ProbeV2SoftwareLevels(ctx, testDeviceID)
		done <- probeResult{got, err}
	}()
	return done
}

func readProbeRequest(t *testing.T, ws *websocket.Conn) map[string]any {
	t.Helper()
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var request map[string]any
	err := websocket.JSON.Receive(ws, &request)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || request["type"] != "lighting.state_probe" ||
		request["schema_version"] != float64(2) ||
		request["device_id"] != testDeviceID || request["assignment_epoch"] != float64(1) ||
		request["commands_enabled"] != false || len(request["challenge"].(string)) != 64 ||
		request["expected_channels"] != float64(lightingnode.MaxChannels) {
		t.Fatalf("invalid read-only probe request: %+v err=%v", request, err)
	}
	return request
}

func sendProbeReport(t *testing.T, ws *websocket.Conn, request map[string]any, levels []int) {
	t.Helper()
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "lighting.state_report", "schema_version": 2,
		"device_id": testDeviceID,
		"assignment_epoch": request["assignment_epoch"],
		"connection_generation": request["connection_generation"],
		"challenge": request["challenge"], "levels_known": true,
		"blackout": true, "channel_levels": levels,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestV2ReadOnlyProbeDoesNotSendToUnadvertisedFirmware(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := connectV2ReadOnlyProbe(t, f, false)
	_, err := f.runtime.ProbeV2SoftwareLevels(context.Background(), testDeviceID)
	if !errors.Is(err, devicechannel.ErrV2LightingProbeUnavailable) {
		t.Fatalf("unadvertised probe unexpectedly enabled: %v", err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var unexpected map[string]any
	if err := websocket.JSON.Receive(ws, &unexpected); err == nil {
		t.Fatalf("legacy firmware received unknown frame: %+v", unexpected)
	}
}

func TestV2ReadOnlyProbeFreshReportRemainsBlockedAndSoftwareOnly(t *testing.T) {
	f := newRuntimeFixture(t)
	ws := connectV2ReadOnlyProbe(t, f, true)
	done := startReadOnlyProbe(f, context.Background())
	request := readProbeRequest(t, ws)
	levels := make([]int, lightingnode.MaxChannels)
	levels[1] = 140 // report unsafe state without claiming it is acceptable
	sendProbeReport(t, ws, request, levels)
	select {
	case result := <-done:
		if result.err != nil || result.report.CommandsEnabled ||
			result.report.PhysicalOutputVerified || !result.report.UnsafeWhileUnactivated ||
			result.report.AssignmentState != "UNASSIGNED" ||
			result.report.ProjectID != "" ||
			result.report.ConnectionGeneration != int64(request["connection_generation"].(float64)) ||
			len(result.report.ChannelLevels) != lightingnode.MaxChannels ||
			result.report.ChannelLevels[1] != 140 {
			t.Fatalf("read-only software report gained authority: %+v err=%v", result.report, result.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("authenticated software report did not complete")
	}
	device, err := f.repo.GetDevice(context.Background(), testDeviceID)
	if err != nil || device.ProjectID != "" || device.Runtime == nil ||
		device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
		t.Fatalf("read-only probe granted READY or project: %+v err=%v", device, err)
	}
	if _, _, err := f.repo.CreateCommand(context.Background(), deviceexperience.CreateCommandInput{
		DeviceID: testDeviceID, ProjectID: f.projectID,
		CommandType: lightingnode.CommandChannelsSet, Issuer: "owner",
	}); err == nil {
		t.Fatal("read-only probe enabled a legacy lighting command")
	}
}

func TestV2ReadOnlyProbeRejectsStaleOrMalformedReport(t *testing.T) {
	for _, tc := range []struct {
		name string
		mutate func(map[string]any)
	}{
		{"bad challenge", func(m map[string]any) { m["challenge"] = strings.Repeat("0", 64) }},
		{"old socket generation", func(m map[string]any) { m["connection_generation"] = 0 }},
		{"wrong epoch", func(m map[string]any) { m["assignment_epoch"] = 9 }},
		{"partial levels", func(m map[string]any) { m["channel_levels"] = make([]int, lightingnode.MaxChannels-1) }},
		{"negative level", func(m map[string]any) { levels := make([]int, lightingnode.MaxChannels); levels[0] = -1; m["channel_levels"] = levels }},
		{"out-of-range level", func(m map[string]any) { levels := make([]int, lightingnode.MaxChannels); levels[0] = 256; m["channel_levels"] = levels }},
		{"unknown values", func(m map[string]any) { m["levels_known"] = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeFixture(t)
			ws := connectV2ReadOnlyProbe(t, f, true)
			done := startReadOnlyProbe(f, context.Background())
			request := readProbeRequest(t, ws)
			report := map[string]any{
				"type": "lighting.state_report", "schema_version": 2,
				"device_id": testDeviceID, "assignment_epoch": request["assignment_epoch"],
				"connection_generation": request["connection_generation"],
				"challenge": request["challenge"], "levels_known": true,
				"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
			}
			tc.mutate(report)
			_ = websocket.JSON.Send(ws, report)
			select {
			case result := <-done:
				if !errors.Is(result.err, devicechannel.ErrV2LightingProbeUnavailable) ||
					result.report.CommandsEnabled || result.report.PhysicalOutputVerified {
					t.Fatalf("bad report accepted: %+v err=%v", result.report, result.err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("invalid report did not fail closed")
			}
		})
	}
}

func TestV2ReadOnlyProbeTimeoutFencesSocketAndReconnect(t *testing.T) {
	f := newRuntimeFixture(t)
	first := connectV2ReadOnlyProbe(t, f, true)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	done := startReadOnlyProbe(f, ctx)
	request := readProbeRequest(t, first)
	select {
	case result := <-done:
		if !errors.Is(result.err, devicechannel.ErrV2LightingProbeUnavailable) {
			t.Fatalf("expired probe incorrectly accepted: %+v err=%v", result.report, result.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("expired probe stalled")
	}
	oldGeneration := int64(request["connection_generation"].(float64))
	second := connectV2ReadOnlyProbe(t, f, true)
	newGeneration, ok := f.runtime.CurrentV2Generation(testDeviceID)
	if !ok || newGeneration <= oldGeneration {
		t.Fatalf("reconnect reused old generation: old=%d new=%d ok=%t", oldGeneration, newGeneration, ok)
	}
	// A report sent on the old socket cannot establish a current reading.
	_ = websocket.JSON.Send(first, map[string]any{
		"type": "lighting.state_report", "schema_version": 2,
		"device_id": testDeviceID, "assignment_epoch": 1,
		"connection_generation": oldGeneration, "challenge": request["challenge"],
		"levels_known": true, "blackout": true,
		"channel_levels": make([]int, lightingnode.MaxChannels),
	})
	next := startReadOnlyProbe(f, context.Background())
	newRequest := readProbeRequest(t, second)
	if newRequest["challenge"] == request["challenge"] {
		t.Fatal("new socket reused old challenge")
	}
	sendProbeReport(t, second, newRequest, make([]int, lightingnode.MaxChannels))
	select {
	case result := <-next:
		if result.err != nil || result.report.ConnectionGeneration != newGeneration ||
			result.report.CommandsEnabled || result.report.PhysicalOutputVerified {
			t.Fatalf("new report affected by stale socket: %+v err=%v", result.report, result.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("new report not delivered")
	}
}
