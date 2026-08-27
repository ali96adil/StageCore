package companionchannel_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestSimulatedChannelDuplicateAfterReconnectDoesNotReplay(t *testing.T) {
	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       "companion-a",
		AppliedSnapshotID: "snapshot-a",
		Capabilities:      []string{"osc.send"},
		Connected:         true,
	}); err != nil {
		t.Fatal(err)
	}

	request := companionchannel.ExecutionRequest{
		ExecutionID:       "exec-1",
		CorrelationID:     "corr-1",
		CompanionID:       "companion-a",
		MachineRoleID:     "role-video-main",
		RuntimeSnapshotID: "snapshot-a",
		Capability:        "osc.send",
	}
	first := channel.Execute(context.Background(), request)
	if first.Result != domain.ExecutionCompleted || first.AckLevel != contracts.AckAccepted {
		t.Fatalf("first=%#v", first)
	}
	if got := channel.ExecutionCount("companion-a"); got != 1 {
		t.Fatalf("execution count=%d want 1", got)
	}

	if err := channel.SetConnected("companion-a", false); err != nil {
		t.Fatal(err)
	}
	if err := channel.SetConnected("companion-a", true); err != nil {
		t.Fatal(err)
	}
	duplicate := channel.Execute(context.Background(), request)
	if duplicate != first {
		t.Fatalf("duplicate=%#v want cached %#v", duplicate, first)
	}
	if got := channel.ExecutionCount("companion-a"); got != 1 {
		t.Fatalf("duplicate replayed execution; count=%d want 1", got)
	}
}

func TestSimulatedChannelStaleSnapshotIsCachedAndRequiresNewExecutionAfterSync(t *testing.T) {
	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       "companion-a",
		AppliedSnapshotID: "snapshot-old",
		Capabilities:      []string{"osc.send"},
		Connected:         true,
	}); err != nil {
		t.Fatal(err)
	}

	request := companionchannel.ExecutionRequest{
		ExecutionID:       "exec-stale",
		CompanionID:       "companion-a",
		MachineRoleID:     "role-video-main",
		RuntimeSnapshotID: "snapshot-new",
		Capability:        "osc.send",
	}
	stale := channel.Execute(context.Background(), request)
	if stale.Result != domain.ExecutionFailed || stale.ErrorCode != "SNAPSHOT_MISMATCH" || stale.AckLevel != contracts.AckNone {
		t.Fatalf("stale=%#v", stale)
	}
	if got := channel.ExecutionCount("companion-a"); got != 0 {
		t.Fatalf("stale request executed; count=%d", got)
	}

	if err := channel.SetAppliedSnapshot("companion-a", "snapshot-new"); err != nil {
		t.Fatal(err)
	}
	cached := channel.Execute(context.Background(), request)
	if cached != stale {
		t.Fatalf("same execution id after sync was not cached: got=%#v want=%#v", cached, stale)
	}
	if got := channel.ExecutionCount("companion-a"); got != 0 {
		t.Fatalf("same execution id replayed after sync; count=%d", got)
	}

	request.ExecutionID = "exec-after-sync"
	completed := channel.Execute(context.Background(), request)
	if completed.Result != domain.ExecutionCompleted {
		t.Fatalf("completed=%#v", completed)
	}
	if got := channel.ExecutionCount("companion-a"); got != 1 {
		t.Fatalf("execution count=%d want 1", got)
	}
}

func TestSimulatedChannelOfflineAttemptDoesNotReplayOnReconnect(t *testing.T) {
	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       "companion-a",
		AppliedSnapshotID: "snapshot-a",
		Capabilities:      []string{"osc.send"},
		Connected:         false,
	}); err != nil {
		t.Fatal(err)
	}

	request := companionchannel.ExecutionRequest{
		ExecutionID:       "exec-offline",
		CompanionID:       "companion-a",
		MachineRoleID:     "role-video-main",
		RuntimeSnapshotID: "snapshot-a",
		Capability:        "osc.send",
	}
	offline := channel.Execute(context.Background(), request)
	if offline.Result != domain.ExecutionFailed || offline.ErrorCode != "COMPANION_OFFLINE" {
		t.Fatalf("offline=%#v", offline)
	}
	if got := channel.ExecutionCount("companion-a"); got != 0 {
		t.Fatalf("offline request executed; count=%d", got)
	}

	if err := channel.SetConnected("companion-a", true); err != nil {
		t.Fatal(err)
	}
	cached := channel.Execute(context.Background(), request)
	if cached != offline {
		t.Fatalf("offline execution id replayed on reconnect: got=%#v want=%#v", cached, offline)
	}
	if got := channel.ExecutionCount("companion-a"); got != 0 {
		t.Fatalf("reconnect replayed offline execution; count=%d", got)
	}

	request.ExecutionID = "exec-new"
	completed := channel.Execute(context.Background(), request)
	if completed.Result != domain.ExecutionCompleted {
		t.Fatalf("new execution=%#v", completed)
	}
	if got := channel.ExecutionCount("companion-a"); got != 1 {
		t.Fatalf("execution count=%d want 1", got)
	}
}

func TestSimulatedChannelRejectsMissingCapabilityBeforeExecution(t *testing.T) {
	channel := companionchannel.NewSimulated()
	if err := channel.RegisterAgent(companionchannel.AgentConfig{
		CompanionID:       "companion-a",
		AppliedSnapshotID: "snapshot-a",
		Capabilities:      []string{"osc.send"},
		Connected:         true,
	}); err != nil {
		t.Fatal(err)
	}

	result := channel.Execute(context.Background(), companionchannel.ExecutionRequest{
		ExecutionID:       "exec-cap",
		CompanionID:       "companion-a",
		MachineRoleID:     "role-video-main",
		RuntimeSnapshotID: "snapshot-a",
		Capability:        "midi.send",
	})
	if result.Result != domain.ExecutionFailed || result.ErrorCode != "CAPABILITY_UNAVAILABLE" {
		t.Fatalf("result=%#v", result)
	}
	if got := channel.ExecutionCount("companion-a"); got != 0 {
		t.Fatalf("missing capability executed; count=%d", got)
	}
}
