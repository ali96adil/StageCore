package runtimecontrol

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/snapshot"
)

func TestOutputResourceIdentityCanonicalization(t *testing.T) {
	firstOSC := snapshot.Target{AliasID: "osc-a", TargetRef: "OSC-A", Configuration: json.RawMessage(`{"osc":{"host":"StageCore.LOCAL.","port":9000}}`)}
	secondOSC := snapshot.Target{AliasID: "osc-b", TargetRef: "OSC-B", Configuration: json.RawMessage(`{"osc":{"host":"stagecore.local","port":9000}}`)}
	first, firstKnown := targetOutputResources(firstOSC)
	second, secondKnown := targetOutputResources(secondOSC)
	if !firstKnown || !secondKnown || !outputResourcesConflict(first, second) {
		t.Fatalf("same OSC endpoint was not canonicalized: first=%v (%t) second=%v (%t)", first, firstKnown, second, secondKnown)
	}

	firstTablet := snapshot.Target{AliasID: "tablet-a", TargetRef: "TABLET-A", Configuration: json.RawMessage(`{"tablet_group_id":"  Actors  "}`)}
	secondTablet := snapshot.Target{AliasID: "tablet-b", TargetRef: "TABLET-B", Configuration: json.RawMessage(`{"tablet_group_id":"actors"}`)}
	first, firstKnown = targetOutputResources(firstTablet)
	second, secondKnown = targetOutputResources(secondTablet)
	if !firstKnown || !secondKnown || !outputResourcesConflict(first, second) {
		t.Fatalf("targets in one declared tablet group were not fenced: first=%v (%t) second=%v (%t)", first, firstKnown, second, secondKnown)
	}
	firstDevice, _ := targetOutputResources(snapshot.Target{AliasID: "stage-a", TargetRef: "STAGE-A", Configuration: json.RawMessage(`{"device_id":"dmx-a","group_name":"actors"}`)})
	secondDevice, _ := targetOutputResources(snapshot.Target{AliasID: "stage-b", TargetRef: "STAGE-B", Configuration: json.RawMessage(`{"device_id":"dmx-b","group_name":"actors"}`)})
	if outputResourcesConflict(firstDevice, secondDevice) {
		t.Fatalf("administrative group name incorrectly merged distinct physical devices: first=%v second=%v", firstDevice, secondDevice)
	}

	unknown, known := targetOutputResources(snapshot.Target{AliasID: "unknown", TargetRef: "UNRESOLVED", Configuration: json.RawMessage(`{}`)})
	wildcard := false
	for _, resource := range unknown {
		wildcard = wildcard || resource == wildcardOutputResource
	}
	if known || !wildcard || !outputResourcesConflict(unknown, first) {
		t.Fatalf("unknown physical identity must fail closed: resources=%v known=%t", unknown, known)
	}
}

func BenchmarkCueOutputAdmissionLinked24Actions(b *testing.B) {
	const cueCount = 6
	const actionsPerCue = 4
	manifest := snapshot.Manifest{Targets: make([]snapshot.Target, cueCount)}
	group := make([]snapshot.Cue, cueCount)
	for cueIndex := range cueCount {
		targetRef := fmt.Sprintf("LIGHT-%d", cueIndex)
		manifest.Targets[cueIndex] = snapshot.Target{
			AliasID: fmt.Sprintf("alias-%d", cueIndex), TargetRef: targetRef,
			Configuration: json.RawMessage(fmt.Sprintf(
				`{"device_id":"dmx-%d","osc":{"host":"node-%d.local","port":9000}}`, cueIndex, cueIndex,
			)),
		}
		group[cueIndex] = snapshot.Cue{ID: fmt.Sprintf("cue-%d", cueIndex), Actions: make([]snapshot.Action, actionsPerCue)}
		for actionIndex := range actionsPerCue {
			group[cueIndex].Actions[actionIndex] = snapshot.Action{
				TargetRef: targetRef, CapabilityKey: "lighting.channels.fade", Enabled: true,
				Parameters: json.RawMessage(`{"channels":{"warm_a":35}}`),
			}
		}
	}
	service := &Service{
		active:   map[string]map[string]activeRun{"session": {"active": {requestID: "active"}}},
		stopping: map[string]bool{}, emergencyApplying: map[string]bool{},
	}
	latencies := make([]time.Duration, b.N)
	b.ReportAllocs()
	b.ReportMetric(cueCount*actionsPerCue, "actions/group")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		started := time.Now()
		result := service.admitCueOutputResources("session", "active", cueOutputResources(manifest, group), group)
		latencies[i] = time.Since(started)
		if result != nil {
			b.Fatalf("simulated resource admission rejected: %+v", result)
		}
	}
	b.StopTimer()
	reportRuntimeControlLatencyDistribution(b, latencies)
}
