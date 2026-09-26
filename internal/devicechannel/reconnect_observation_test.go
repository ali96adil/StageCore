package devicechannel_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"golang.org/x/net/websocket"
)

func waitForV2ProbeCache(t *testing.T, f *runtimeFixture) devicechannelSoftwareLevelView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if report, found := f.runtime.LatestV2SoftwareLevels(testDeviceID); found {
			return devicechannelSoftwareLevelView{
				Generation: report.ConnectionGeneration, Levels: report.ChannelLevels,
				CommandsEnabled: report.CommandsEnabled,
				PhysicalVerified: report.PhysicalOutputVerified,
				Unsafe: report.UnsafeWhileUnactivated,
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("negotiated automatic reconnect probe did not produce diagnostic")
	return devicechannelSoftwareLevelView{}
}

type devicechannelSoftwareLevelView struct {
	Generation int64
	Levels []uint8
	CommandsEnabled bool
	PhysicalVerified bool
	Unsafe bool
}

func TestNegotiatedV2AutoProbeOnEveryReconnectNeverGrantsOutput(t *testing.T) {
	t.Setenv("STAGECORE_EXPERIMENTAL_V2_AUTO_PROBE", "1")
	f := newRuntimeFixture(t)
	first := connectV2ReadOnlyProbe(t, f, true)
	oldRequest := readProbeRequest(t, first)
	oldLevels := make([]int, lightingnode.MaxChannels)
	oldLevels[1] = 140 // unexpected logical output on the unassigned node
	sendProbeReport(t, first, oldRequest, oldLevels)
	oldResult := waitForV2ProbeCache(t, f)
	if !oldResult.Unsafe || oldResult.Levels[1] != 140 ||
		oldResult.CommandsEnabled || oldResult.PhysicalVerified {
		t.Fatalf("first diagnostic granted output or lost unsafe state: %+v", oldResult)
	}

	// This reconnect is still the *same* logical Cue/Hub assignment. A new
	// authenticated socket must invalidate the previous observation.
	second := connectV2ReadOnlyProbe(t, f, true)
	defer second.Close()
	if _, exists := f.runtime.LatestV2SoftwareLevels(testDeviceID); exists {
		t.Fatal("cached pre-reconnect levels were treated as fresh")
	}
	newRequest := readProbeRequest(t, second)
	if newRequest["challenge"] == oldRequest["challenge"] ||
		newRequest["connection_generation"] == oldRequest["connection_generation"] {
		t.Fatalf("reconnect reused old scope: old=%+v new=%+v", oldRequest, newRequest)
	}
	if err := websocket.JSON.Send(first, map[string]any{
		"type": "lighting.state_report", "schema_version": 2,
		"device_id": testDeviceID, "assignment_epoch": 1,
		"connection_generation": oldRequest["connection_generation"],
		"challenge": oldRequest["challenge"],
		"levels_known": true, "blackout": true,
		"channel_levels": oldLevels,
	}); err == nil {
		if _, exists := f.runtime.LatestV2SoftwareLevels(testDeviceID); exists {
			t.Fatal("old socket report published after reconnect")
		}
	}
	sendProbeReport(t, second, newRequest, make([]int, lightingnode.MaxChannels))
	newResult := waitForV2ProbeCache(t, f)
	if newResult.Generation <= oldResult.Generation || newResult.Unsafe ||
		newResult.CommandsEnabled || newResult.PhysicalVerified ||
		newResult.Levels[1] != 0 {
		t.Fatalf("new zero report was not treated as a fresh diagnostic: %+v", newResult)
	}
	// Returned state is a copy; changing a UI caller's slice cannot mutate
	// the Hub's cached report.
	newResult.Levels[1] = 255
	actual, ok := f.runtime.LatestV2SoftwareLevels(testDeviceID)
	if !ok || actual.ChannelLevels[1] != 0 {
		t.Fatal("diagnostic caller mutated shared cached device state")
	}
	device, err := f.repo.GetDevice(context.Background(), testDeviceID)
	if err != nil || device.ProjectID != "" || device.Runtime == nil ||
		device.Runtime.Readiness != deviceexperience.ReadinessBlocker {
		t.Fatalf("read-only reconnect probe enabled v2 output: %+v err=%v", device, err)
	}
}

func TestLegacyReconnectDoesNotBecomeOfflineFromOldSocket(t *testing.T) {
	f := newRuntimeFixture(t)
	first := f.connect(t)
	// A replacement registers while the old connection's unregister and
	// diagnostic paths can still be running on another goroutine.
	second := f.connect(t)
	defer second.Close()
	_ = first.Close()
	if err := websocket.JSON.Send(second, map[string]any{
		"type": "device.observation", "schema_version": 1,
		"device_id": testDeviceID,
		"readiness": deviceexperience.ReadinessReady,
		"observed_state": json.RawMessage(`{"fresh_marker":"replacement"}`),
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		device, err := f.repo.GetDevice(context.Background(), testDeviceID)
		if err != nil {
			t.Fatal(err)
		}
		if device.Runtime != nil &&
			strings.Contains(string(device.Runtime.ObservedState), "replacement") {
			if device.Runtime.Connection != deviceexperience.ConnectionOnline {
				t.Fatalf("old socket overwrote replacement status: %+v", device.Runtime)
			}
			// A delayed unregister must not turn the *new* socket OFFLINE.
			time.Sleep(100 * time.Millisecond)
			latest, err := f.repo.GetDevice(context.Background(), testDeviceID)
			if err != nil || latest.Runtime == nil ||
				latest.Runtime.Connection != deviceexperience.ConnectionOnline {
				t.Fatalf("replacement was overwritten OFFLINE: %+v err=%v", latest.Runtime, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("replacement observation not persisted")
}

func TestBlockedV2AutoProbeStartsOnlyAfterPersistedEpochReceipt(t *testing.T) {
	t.Setenv("STAGECORE_EXPERIMENTAL_V2_AUTO_PROBE", "1")
	f := newRuntimeFixture(t)
	ctx := context.Background()
	// Hub-approved registration owns the allowlisted capability; an old
	// firmware socket without that capability must not receive a probe.
	caps := append(lightingnode.CapabilityKeys(), "lighting.state_probe/1")
	if _, err := f.repo.RegisterUnassignedV2(ctx, deviceexperience.Device{
		ID: testDeviceID, Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Blocked Probe Node", ProfileID: lightingnode.ProfileID,
		ProtocolVersion: deviceexperience.ProtocolVersion2,
		Enabled: true, Capabilities: caps,
	}); err != nil {
		t.Fatal(err)
	}
	// Exercise the canonical reserved-transfer path. Direct sidecar creation
	// is not enough for the epoch-ACK DB trigger: the persisted committed
	// transfer audit and reservation must exist before the Hub accepts ACK.
	prepareBlockedEpoch(t, f)
	url := "ws" + strings.TrimPrefix(f.server.URL, "http")
	ws, err := websocket.Dial(url, "", f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "device.hello", "schema_version": 1,
		"device_id": testDeviceID, "project_id": "",
		"device_kind": deviceexperience.DeviceGeneric, "display_name": "Blocked Probe Node",
		"profile_id": lightingnode.ProfileID, "protocol_version": deviceexperience.ProtocolVersion2,
		"capabilities": caps, "readiness": deviceexperience.ReadinessReady,
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var welcome map[string]any
	err = websocket.JSON.Receive(ws, &welcome)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || welcome["type"] != "assignment.state" ||
		welcome["state"] != "BLOCKED" || welcome["commands_enabled"] != false ||
		welcome["epoch_ack_required"] != true {
		t.Fatalf("expected blocked welcome: %+v err=%v", welcome, err)
	}
	// No state probe may preempt the durable epoch ACK; the firmware
	// must persist the zero-state epoch before receiving probe traffic.
	if err := websocket.JSON.Send(ws, map[string]any{
		"type": "assignment.epoch_ack", "schema_version": 2,
		"device_id": testDeviceID, "project_id": f.projectID,
		"assignment_epoch": welcome["assignment_epoch"],
		"connection_generation": welcome["connection_generation"],
		"blackout": true, "channel_levels": make([]int, lightingnode.MaxChannels),
	}); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	var receipt map[string]any
	err = websocket.JSON.Receive(ws, &receipt)
	_ = ws.SetReadDeadline(time.Time{})
	if err != nil || receipt["type"] != "assignment.epoch_ack_receipt" ||
		receipt["state"] != "BLOCKED" || receipt["commands_enabled"] != false {
		t.Fatalf("probe arrived before receipt or epoch denied: %+v err=%v", receipt, err)
	}
	request := readProbeRequest(t, ws)
	if request["assignment_epoch"] != welcome["assignment_epoch"] ||
		request["connection_generation"] != welcome["connection_generation"] {
		t.Fatalf("probe not scoped to persisted BLOCKED epoch: %+v", request)
	}
	sendProbeReport(t, ws, request, make([]int, lightingnode.MaxChannels))
	report := waitForV2ProbeCache(t, f)
	if report.CommandsEnabled || report.PhysicalVerified || report.Unsafe {
		t.Fatalf("blocked diagnostic changed software authority: %+v", report)
	}
	assigned, err := f.repo.GetAssignmentRecord(ctx, testDeviceID)
	if err != nil || assigned.State != "BLOCKED" || assigned.ProjectID != f.projectID ||
		assigned.Epoch != 2 || assigned.RuntimeSnapshotID != "" {
		t.Fatalf("read-only report modified project/epoch/snapshot: %+v err=%v", assigned, err)
	}
}

func TestAutoProbeOptInCannotSendUnsupportedFrameToLegacyV2(t *testing.T) {
	t.Setenv("STAGECORE_EXPERIMENTAL_V2_AUTO_PROBE", "1")
	f := newRuntimeFixture(t)
	ws := connectV2ReadOnlyProbe(t, f, false)
	defer ws.Close()
	_ = ws.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var unexpected map[string]any
	if err := websocket.JSON.Receive(ws, &unexpected); err == nil {
		t.Fatalf("unadvertised v2 node received an experimental probe: %+v", unexpected)
	}
	if _, exists := f.runtime.LatestV2SoftwareLevels(testDeviceID); exists {
		t.Fatal("unsupported firmware produced a fresh diagnostic")
	}
}
