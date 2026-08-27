package companionchannel_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestSimulatedChannelReportsTimeoutExplicitlyAndCachesIt(t *testing.T) {
	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       "companion-timeout",
		AppliedSnapshotID: "snapshot-a",
		Capabilities:      []string{"sim.test"},
		Connected:         true,
		Behavior:          companionchannel.SimulationTimeout,
	}); err != nil {
		t.Fatal(err)
	}
	request := companionchannel.ExecutionRequest{
		ExecutionID:       "exec-timeout",
		CompanionID:       "companion-timeout",
		MachineRoleID:     "role-video-main",
		RuntimeSnapshotID: "snapshot-a",
		Capability:        "sim.test",
	}
	first := channel.Execute(context.Background(), request)
	if first.Result != domain.ExecutionTimedOut || first.ErrorCode != "COMPANION_EXECUTION_TIMEOUT" || first.AckLevel != contracts.AckNone {
		t.Fatalf("timeout=%#v", first)
	}
	second := channel.Execute(context.Background(), request)
	if second != first {
		t.Fatalf("timeout result was not cached: second=%#v first=%#v", second, first)
	}
	if got := channel.ExecutionCount("companion-timeout"); got != 1 {
		t.Fatalf("timeout execution replayed: count=%d", got)
	}
}

func TestSimulatedChannelReportsInterruptedWithoutFalseSuccess(t *testing.T) {
	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       "companion-interrupted",
		AppliedSnapshotID: "snapshot-a",
		Capabilities:      []string{"sim.test"},
		Connected:         true,
		Behavior:          companionchannel.SimulationInterrupted,
	}); err != nil {
		t.Fatal(err)
	}
	result := channel.Execute(context.Background(), companionchannel.ExecutionRequest{
		ExecutionID:       "exec-interrupted",
		CompanionID:       "companion-interrupted",
		MachineRoleID:     "role-video-main",
		RuntimeSnapshotID: "snapshot-a",
		Capability:        "sim.test",
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "COMPANION_EXECUTION_INTERRUPTED" || result.AckLevel != contracts.AckNone {
		t.Fatalf("interrupted=%#v", result)
	}
	if got := channel.ExecutionCount("companion-interrupted"); got != 1 {
		t.Fatalf("interrupted execution count=%d want 1", got)
	}
}
