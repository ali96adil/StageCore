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
