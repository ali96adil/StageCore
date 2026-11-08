package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/google/uuid"
)

func TestExternalRehearsalStopUsesPublishedPolicyAndDispatchesOnce(t *testing.T) {
	h := newRuntimeHarness(t)
	ctx := context.Background()
	p, rev, err := h.store.CreateProject(ctx, store.CreateProjectParams{Name: "External stop fixture", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	config := json.RawMessage(`{"machine_role_id":"fixture","end_rehearsal":{"capability":"osc.send","parameters":{"address":"/fixture/stop"}}}`)
	if _, err := h.store.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: p.ID, LogicalName: "VIDEO",
		LogicalType: "MACHINE_ROLE", ProjectConfig: config}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetRevisionStatus(ctx, rev.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(h.store).Create(ctx, rev.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	dispatches := 0
	blackout := false
	h.service.stopSafety = func(context.Context, domain.Session, contracts.CommandEnvelope) error {
		if !blackout {
			return errors.New("lighting safety not complete")
		}
		return nil
	}
	h.service.executor.inner = capability.ExecutorFunc(func(_ context.Context, req capability.Request) capability.Result {
		dispatches++
		if !blackout || req.RuntimeSnapshotID != published.ID || req.Target.Ref != "VIDEO" || req.Capability != "osc.send" {
			t.Errorf("unexpected dispatch: %+v", req)
		}
		return capability.Result{Result: domain.ExecutionCompleted, AckLevel: contracts.AckTransportOnly}
	})
	session, err := h.store.CreateSession(ctx, published.ID, domain.SessionRehearsal, "confirmed external stop")
	if err != nil {
		t.Fatal(err)
	}
	stop := StopRequest{SessionID: session.ID, Issuer: "owner", RequestID: uuid.NewString(), RehearsalExternalStopSessionID: "stale"}
	if result := h.service.StopSession(ctx, stop); result.Status != contracts.CommandRejected || dispatches != 0 {
		t.Fatal("stale confirmation dispatched")
	}
	stop.RequestID, stop.RehearsalExternalStopSessionID = uuid.NewString(), session.ID
	if result := h.service.StopSession(ctx, stop); result.Status != contracts.CommandFailed || dispatches != 0 {
		t.Fatal("failed safety dispatched")
	}
	stop.RequestID, blackout = uuid.NewString(), true
	if result := h.service.StopSession(ctx, stop); result.Status != contracts.CommandCompleted || dispatches != 1 {
		t.Fatalf("explicit stop did not complete: %+v, calls=%d", result, dispatches)
	}
	manifest, err := snapshot.Decode(published.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := rehearsalOutputs(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.runRehearsalOutputs(ctx, session, contracts.CommandEnvelope{CommandID: uuid.NewString(), Issuer: "owner"}, plan); err != nil || dispatches != 1 {
		t.Fatalf("persisted output was replayed: %v, calls=%d", err, dispatches)
	}
}
