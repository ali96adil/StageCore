package capability_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

func TestRegistryTargetTypeTakesPrecedenceWithoutChangingCapabilityIdentity(t *testing.T) {
	registry := capability.NewRegistry()
	if err := registry.Register("osc.send", capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckTransportOnly, ResponseSummary: "local"}
	})); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterTargetType("machine_role", capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckAccepted, ResponseSummary: "companion"}
	})); err != nil {
		t.Fatal(err)
	}

	remote := registry.Execute(context.Background(), capability.Request{
		Capability: "osc.send",
		Target:     &capability.Target{LogicalType: "MACHINE_ROLE"},
	})
	if remote.ResponseSummary != "companion" || remote.AckLevel != contracts.AckAccepted {
		t.Fatalf("remote=%#v", remote)
	}

	local := registry.Execute(context.Background(), capability.Request{
		Capability: "osc.send",
		Target:     &capability.Target{LogicalType: "projector"},
	})
	if local.ResponseSummary != "local" || local.AckLevel != contracts.AckTransportOnly {
		t.Fatalf("local=%#v", local)
	}
}
