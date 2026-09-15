package routing_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/dispatchauthority"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/routing"
)

func TestRoutingOperationalOutputHonorsPhysicalDispatchAuthority(t *testing.T) {
	f := newOSCOutputRouteFixture(t, 53001, json.RawMessage(`{"operator":"equals","value":1}`), true, nil)
	var authorityCalls atomic.Int32
	var physicalCalls atomic.Int32
	physical := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		physicalCalls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckDevice}
	})
	gate := dispatchauthority.New(physical, dispatchauthority.SourceFunc(func(context.Context) (dispatchauthority.Snapshot, error) {
		authorityCalls.Add(1)
		return dispatchauthority.Snapshot{Mode: dispatchauthority.ModeStandby, HolderID: "hub-leader", Epoch: 11}, nil
	}))

	result := routing.NewSimulationSafe(f.store, gate).InjectTest(
		context.Background(),
		f.session.ID,
		injectCommand(t, f, json.RawMessage(`1`)),
	)
	if result.Status != contracts.CommandFailed {
		t.Fatalf("result=%#v", result)
	}
	if authorityCalls.Load() != 1 {
		t.Fatalf("authority calls=%d, want 1", authorityCalls.Load())
	}
	if physicalCalls.Load() != 0 {
		t.Fatalf("standby route reached physical executor %d time(s), want 0", physicalCalls.Load())
	}

	events, err := f.store.ListEvents(context.Background(), f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var fenced bool
	for _, event := range events {
		if event.EventType != "route.action.failed" {
			continue
		}
		var payload struct {
			ErrorCode string `json:"error_code"`
			Result    string `json:"result"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.ErrorCode == "PHYSICAL_DISPATCH_NOT_LEADER" && payload.Result == string(domain.ExecutionFailed) {
			fenced = true
		}
	}
	if !fenced {
		t.Fatalf("missing fenced RouteAction evidence; events=%v", routeEventTypes(events))
	}
}

func TestRoutingSimulationBypassesPhysicalDispatchAuthority(t *testing.T) {
	f := newOSCOutputRouteFixture(t, 53002, json.RawMessage(`{"operator":"equals","value":1}`), true, nil)
	if _, err := f.h.DB.Exec(`UPDATE sessions SET session_type = 'SIMULATION' WHERE session_id = ?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	var authorityCalls atomic.Int32
	var physicalCalls atomic.Int32
	physical := capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		physicalCalls.Add(1)
		return capability.Result{Result: domain.ExecutionCompleted}
	})
	gate := dispatchauthority.New(physical, dispatchauthority.SourceFunc(func(context.Context) (dispatchauthority.Snapshot, error) {
		authorityCalls.Add(1)
		return dispatchauthority.Snapshot{Mode: dispatchauthority.ModeStandby, HolderID: "hub-leader", Epoch: 11}, nil
	}))

	result := routing.NewSimulationSafe(f.store, gate).InjectTest(
		context.Background(),
		f.session.ID,
		injectCommand(t, f, json.RawMessage(`1`)),
	)
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("result=%#v", result)
	}
	if authorityCalls.Load() != 0 {
		t.Fatalf("simulation consulted physical authority %d time(s), want 0", authorityCalls.Load())
	}
	if physicalCalls.Load() != 0 {
		t.Fatalf("simulation reached physical executor %d time(s), want 0", physicalCalls.Load())
	}
}
