package simulator

import (
	"context"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestDigitalTwinKeepsSessionStateIsolated(t *testing.T) {
	twin := NewDigitalTwin()
	if err := twin.SetTargetOnline("session-a", "tablet.3", false); err != nil {
		t.Fatal(err)
	}

	a := twin.Execute(context.Background(), twinRequest("session-a", "tablet.3", "tablet.media.play"))
	b := twin.Execute(context.Background(), twinRequest("session-b", "tablet.3", "tablet.media.play"))
	if a.Result != domain.ExecutionFailed || a.ErrorCode != "SIM_TARGET_OFFLINE" {
		t.Fatalf("session-a result=%#v", a)
	}
	if b.Result != domain.ExecutionCompleted {
		t.Fatalf("session-b result=%#v", b)
	}
	if got := twin.Snapshot("session-a").Targets[0].Online; got {
		t.Fatal("session-a target unexpectedly online")
	}
	if got := twin.Snapshot("session-b").Targets[0].Online; !got {
		t.Fatal("session-b target unexpectedly offline")
	}
}

func TestDigitalTwinOneShotFailureFallsBackToCompletion(t *testing.T) {
	twin := NewDigitalTwin()
	if err := twin.ConfigureFault("session-1", FaultScenario{
		TargetRef: "lighting.front",
		Behavior:  "FAIL",
		ErrorCode: "VIRTUAL_DIMMER_FAULT",
		Uses:      1,
	}); err != nil {
		t.Fatal(err)
	}

	first := twin.Execute(context.Background(), twinRequest("session-1", "lighting.front", "osc.send"))
	second := twin.Execute(context.Background(), twinRequest("session-1", "lighting.front", "osc.send"))
	if first.Result != domain.ExecutionFailed || first.ErrorCode != "VIRTUAL_DIMMER_FAULT" {
		t.Fatalf("first=%#v", first)
	}
	if second.Result != domain.ExecutionCompleted {
		t.Fatalf("second=%#v", second)
	}
	if faults := twin.Snapshot("session-1").Faults; len(faults) != 0 {
		t.Fatalf("one-shot fault still present: %#v", faults)
	}
}

func TestDigitalTwinDisconnectPersistsUntilReconnect(t *testing.T) {
	twin := NewDigitalTwin()
	if err := twin.ConfigureFault("session-1", FaultScenario{TargetRef: "tablet.4", Behavior: "DISCONNECT", Uses: 1}); err != nil {
		t.Fatal(err)
	}

	disconnected := twin.Execute(context.Background(), twinRequest("session-1", "tablet.4", "tablet.media.play"))
	stillOffline := twin.Execute(context.Background(), twinRequest("session-1", "tablet.4", "tablet.media.stop"))
	if disconnected.ErrorCode != "SIM_TARGET_OFFLINE" || stillOffline.ErrorCode != "SIM_TARGET_OFFLINE" {
		t.Fatalf("disconnect=%#v later=%#v", disconnected, stillOffline)
	}

	if err := twin.ConfigureFault("session-1", FaultScenario{TargetRef: "tablet.4", Capability: "tablet.device.reconnect", Behavior: "RECONNECT", Uses: 1}); err != nil {
		t.Fatal(err)
	}
	reconnected := twin.Execute(context.Background(), twinRequest("session-1", "tablet.4", "tablet.device.reconnect"))
	after := twin.Execute(context.Background(), twinRequest("session-1", "tablet.4", "tablet.media.play"))
	if reconnected.Result != domain.ExecutionCompleted || after.Result != domain.ExecutionCompleted {
		t.Fatalf("reconnect=%#v after=%#v", reconnected, after)
	}
	if !twin.Snapshot("session-1").Targets[0].Online {
		t.Fatal("target remained offline after reconnect")
	}
}

func TestDigitalTwinFaultSelectorDoesNotLeakAcrossCapabilities(t *testing.T) {
	twin := NewDigitalTwin()
	if err := twin.ConfigureFault("session-1", FaultScenario{
		TargetRef:  "video.main",
		Capability: "media.stop",
		Behavior:   "REJECT",
	}); err != nil {
		t.Fatal(err)
	}

	play := twin.Execute(context.Background(), twinRequest("session-1", "video.main", "media.play"))
	stop := twin.Execute(context.Background(), twinRequest("session-1", "video.main", "media.stop"))
	if play.Result != domain.ExecutionCompleted {
		t.Fatalf("play=%#v", play)
	}
	if stop.Result != domain.ExecutionFailed || stop.ErrorCode != "SIM_TARGET_REJECTED" {
		t.Fatalf("stop=%#v", stop)
	}
}

func TestDigitalTwinSnapshotIsDeterministicAndRecordsOutcome(t *testing.T) {
	twin := NewDigitalTwin()
	_ = twin.Execute(context.Background(), twinRequest("session-1", "z.target", "osc.send"))
	_ = twin.Execute(context.Background(), twinRequest("session-1", "a.target", "http.request"))
	_ = twin.Execute(context.Background(), twinRequest("session-1", "a.target", "http.request"))
	if err := twin.ConfigureFault("session-1", FaultScenario{TargetRef: "z.target", Behavior: "FAIL"}); err != nil {
		t.Fatal(err)
	}
	if err := twin.ConfigureFault("session-1", FaultScenario{TargetRef: "a.target", Capability: "http.request", Behavior: "DELAY", DelayMS: 1}); err != nil {
		t.Fatal(err)
	}

	snapshot := twin.Snapshot("session-1")
	if len(snapshot.Targets) != 2 || snapshot.Targets[0].TargetRef != "a.target" || snapshot.Targets[1].TargetRef != "z.target" {
		t.Fatalf("targets=%#v", snapshot.Targets)
	}
	if snapshot.Targets[0].ExecutionCount != 2 || snapshot.Targets[0].LastResult != domain.ExecutionCompleted {
		t.Fatalf("a.target state=%#v", snapshot.Targets[0])
	}
	if len(snapshot.Faults) != 2 || snapshot.Faults[0].TargetRef != "a.target" || snapshot.Faults[1].TargetRef != "z.target" {
		t.Fatalf("faults=%#v", snapshot.Faults)
	}
}

func TestDigitalTwinRejectsInvalidFaults(t *testing.T) {
	twin := NewDigitalTwin()
	cases := []FaultScenario{
		{Behavior: "FAIL"},
		{TargetRef: "x", Behavior: "NOT_REAL"},
		{TargetRef: "x", Behavior: "FAIL", Uses: -1},
		{TargetRef: "x", Behavior: "DELAY", DelayMS: -1},
	}
	for _, scenario := range cases {
		if err := twin.ConfigureFault("session-1", scenario); err == nil {
			t.Fatalf("scenario unexpectedly accepted: %#v", scenario)
		}
	}
	if err := twin.ConfigureFault("", FaultScenario{TargetRef: "x", Behavior: "FAIL"}); err == nil {
		t.Fatal("empty session ID unexpectedly accepted")
	}
}

func TestDigitalTwinTimeoutUsesCallerDeadline(t *testing.T) {
	twin := NewDigitalTwin()
	if err := twin.ConfigureFault("session-1", FaultScenario{TargetRef: "video.main", Behavior: "TIMEOUT", Uses: 1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result := twin.Execute(ctx, twinRequest("session-1", "video.main", "media.play"))
	if result.Result != domain.ExecutionTimedOut || result.ErrorCode != "TIMEOUT" {
		t.Fatalf("result=%#v", result)
	}
}

func twinRequest(sessionID, targetRef, capabilityKey string) capability.Request {
	return capability.Request{
		SessionID:  sessionID,
		Capability: capabilityKey,
		Target: &capability.Target{
			Ref:         targetRef,
			LogicalType: "virtual",
		},
	}
}
