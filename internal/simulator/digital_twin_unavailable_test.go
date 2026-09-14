package simulator

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestDigitalTwinCapabilityUnavailableFaultIsExplicit(t *testing.T) {
	twin := NewDigitalTwin()
	if err := twin.ConfigureFault("session-1", FaultScenario{
		Capability: "tablet.media.play",
		Behavior:   "UNAVAILABLE",
		Uses:       1,
	}); err != nil {
		t.Fatal(err)
	}

	result := twin.Execute(context.Background(), twinRequest("session-1", "tablet.3", "tablet.media.play"))
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SIM_CAPABILITY_UNAVAILABLE" {
		t.Fatalf("result=%#v", result)
	}
	if result.ResponseSummary != "simulated capability unavailable" {
		t.Fatalf("summary=%q", result.ResponseSummary)
	}
	if faults := twin.Snapshot("session-1").Faults; len(faults) != 0 {
		t.Fatalf("one-shot unavailable fault was not consumed: %#v", faults)
	}
}

func TestDigitalTwinCapabilityUnavailableAliasIsAccepted(t *testing.T) {
	twin := NewDigitalTwin()
	if err := twin.ConfigureFault("session-1", FaultScenario{
		TargetRef: "video.main",
		Behavior:  "CAPABILITY_UNAVAILABLE",
	}); err != nil {
		t.Fatal(err)
	}
	result := twin.Execute(context.Background(), twinRequest("session-1", "video.main", "media.play"))
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "SIM_CAPABILITY_UNAVAILABLE" {
		t.Fatalf("result=%#v", result)
	}
}
