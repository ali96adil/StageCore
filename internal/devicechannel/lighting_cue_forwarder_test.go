package devicechannel_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestForwarderResolvesLightingAliasesFromPublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	now := time.Now().UTC()
	stageStore := store.New(h.DB, clock.Fixed{Time: now})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Cue", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := deviceexperience.NewRepository(
		h.DB,
		deviceexperience.WithClock(func() time.Time { return now }),
		deviceexperience.WithEventRecorder(stageStore),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting 01",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{lightingnode.CapabilityChannelsFade}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	binding := lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID,
		Configuration: lightingnode.Configuration{SchemaVersion: lightingnode.SchemaVersion1, Channels: []lightingnode.ChannelConfig{
			{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
			{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Cold", Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
		}},
		Aliases: map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
	}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, binding, "test"); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := stageStore.CreateSession(ctx, published.ID, domain.SessionRehearsal, "Lighting Cue")
	if err != nil {
		t.Fatal(err)
	}

	var captured deviceexperience.CreateCommandInput
	dispatcher := dispatchFunc(func(dispatchCtx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		captured = input
		command, _, err := repository.CreateCommand(dispatchCtx, input)
		if err != nil {
			return deviceexperience.DeviceCommand{}, err
		}
		result, _ := json.Marshal(contracts.CommandResult{
			CommandID: command.Envelope.CommandID,
			Status: contracts.CommandCompleted,
			Payload: json.RawMessage(`{"ack":"DEVICE_ACK"}`),
		})
		return repository.CompleteCommand(dispatchCtx, command.Envelope.CommandID, contracts.CommandCompleted, result)
	})
	forwarder := devicechannel.NewForwarder(stageStore, repository, dispatcher)

	result := forwarder.Execute(ctx, capability.Request{
		ExecutionID: "lighting-action-1", ProjectID: project.ID, SessionID: session.ID,
		RuntimeSnapshotID: published.ID, Issuer: "hub.cue_engine",
		Capability: lightingnode.CapabilityChannelsFade,
		Target: &capability.Target{
			Ref: "lighting.node.01", LogicalType: devicechannel.StageDeviceLogicalType,
			Configuration: json.RawMessage(`{"device_id":"lighting-01"}`),
		},
		Parameters: json.RawMessage(`{"fade_ms":1200,"aliases":{"front_warm":55,"front_cold":20}}`),
		Priority: "P1", TimeoutMS: 4000, CorrelationID: "corr-lighting",
	})
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckDevice {
		t.Fatalf("result=%+v", result)
	}
	if captured.CommandType != lightingnode.CommandChannelsFade || captured.DeviceID != "lighting-01" {
		t.Fatalf("captured command=%+v", captured)
	}
	var payload lightingnode.ChannelsFadePayload
	if err := json.Unmarshal(captured.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.FadeMS != 1200 || payload.Channels["warm_a"] != 55 || payload.Channels["cold_a"] != 20 {
		t.Fatalf("resolved device payload=%+v raw=%s", payload, captured.Payload)
	}
	if _, exists := payload.Channels["front_warm"]; exists {
		t.Fatalf("logical alias leaked to device payload: %s", captured.Payload)
	}
}

func TestForwarderRejectsLightingAliasOutsideTargetNode(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	now := time.Now().UTC()
	stageStore := store.New(h.DB, clock.Fixed{Time: now})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Cross Node Lighting Cue", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := deviceexperience.NewRepository(h.DB, deviceexperience.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lighting-a", "lighting-b"} {
		if _, err := repository.UpsertDevice(ctx, deviceexperience.Device{
			ID: id, ProjectID: project.ID, ProfileID: lightingnode.ProfileID, Kind: deviceexperience.DeviceGeneric,
			DisplayName: id, ProtocolVersion: deviceexperience.ProtocolVersion1,
			Capabilities: []string{lightingnode.CapabilityChannelsSet}, Enabled: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, binding := range []lightingnode.ProjectBinding{
		{
			DeviceID: "lighting-a", ProfileID: lightingnode.ProfileID,
			Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "A", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true}}},
			Aliases: map[string]string{"front_warm": "warm_a"},
		},
		{
			DeviceID: "lighting-b", ProfileID: lightingnode.ProfileID,
			Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{ChannelKey: "warm_b", ChannelNumber: 1, DisplayName: "B", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true}}},
			Aliases: map[string]string{"back_warm": "warm_b"},
		},
	} {
		if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, binding, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := stageStore.CreateSession(ctx, published.ID, domain.SessionRehearsal, "Cross Node")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	forwarder := devicechannel.NewForwarder(stageStore, repository, dispatchFunc(func(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		called = true
		return deviceexperience.DeviceCommand{}, nil
	}))
	result := forwarder.Execute(ctx, capability.Request{
		ExecutionID: "cross-node", ProjectID: project.ID, SessionID: session.ID, RuntimeSnapshotID: published.ID,
		Capability: lightingnode.CapabilityChannelsSet,
		Target: &capability.Target{LogicalType: devicechannel.StageDeviceLogicalType, Configuration: json.RawMessage(`{"device_id":"lighting-a"}`)},
		Parameters: json.RawMessage(`{"aliases":{"back_warm":50}}`), Priority: "P1",
	})
	if called {
		t.Fatal("dispatcher called for cross-device lighting alias")
	}
	if result.ErrorCode != "LIGHTING_ALIAS_RESOLUTION_FAILED" {
		t.Fatalf("result=%+v", result)
	}
}
