package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestLightingCommandMappingsPayloadsAndSnapshotAuthority(t *testing.T) {
	ctx := context.Background()
	repo, handle, projectID := newRepository(t)
	stageStore := store.New(handle.DB, clock.Fixed{Time: phase4Time})

	if _, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: projectID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting 01",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: lightingnode.CapabilityKeys(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	mappings := map[string]string{
		lightingnode.CommandChannelsSet:  lightingnode.CapabilityChannelsSet,
		lightingnode.CommandChannelsFade: lightingnode.CapabilityChannelsFade,
		lightingnode.CommandBlackout:     lightingnode.CapabilityBlackout,
		lightingnode.CommandStateRead:    lightingnode.CapabilityStateRead,
		lightingnode.CommandIdentify:     lightingnode.CapabilityIdentify,
		lightingnode.CommandConfigRead:   lightingnode.CapabilityConfigRead,
		lightingnode.CommandConfigApply:  lightingnode.CapabilityConfigApply,
	}
	for commandType, capability := range mappings {
		if got := deviceexperience.RequiredCapability(commandType); got != capability {
			t.Fatalf("RequiredCapability(%s)=%q want=%q", commandType, got, capability)
		}
		if got := deviceexperience.CommandTypeForCapability(capability); got != commandType {
			t.Fatalf("CommandTypeForCapability(%s)=%q want=%q", capability, got, commandType)
		}
	}
	for _, capability := range []string{
		lightingnode.CapabilityChannelsSet,
		lightingnode.CapabilityChannelsFade,
		lightingnode.CapabilityBlackout,
	} {
		if deviceexperience.CommandTypeForCueCapability(capability) == "" {
			t.Fatalf("cue-safe capability %s is unavailable", capability)
		}
	}
	for _, capability := range []string{
		lightingnode.CapabilityStateRead,
		lightingnode.CapabilityIdentify,
		lightingnode.CapabilityConfigRead,
		lightingnode.CapabilityConfigApply,
	} {
		if got := deviceexperience.CommandTypeForCueCapability(capability); got != "" {
			t.Fatalf("commissioning capability %s leaked into Cue execution as %s", capability, got)
		}
	}

	setCommand, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "lighting-01",
		CommandType: lightingnode.CommandChannelsSet, Issuer: "operator:test",
		Payload: json.RawMessage(`{"channels":{" warm_a ":55}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var setPayload lightingnode.ChannelsSetPayload
	if err := json.Unmarshal(setCommand.Envelope.Payload, &setPayload); err != nil {
		t.Fatal(err)
	}
	if setPayload.Channels["warm_a"] != 55 {
		t.Fatalf("canonical lighting set payload=%s", setCommand.Envelope.Payload)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "lighting-01",
		CommandType: lightingnode.CommandChannelsFade, Issuer: "operator:test",
		Payload: json.RawMessage(`{"fade_ms":0,"channels":{"warm_a":50}}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("invalid fade err=%v want ErrInvalidState", err)
	}

	if _, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "generic-lighting", ProjectID: projectID, Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Generic", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{lightingnode.CapabilityChannelsSet}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "generic-lighting",
		CommandType: lightingnode.CommandChannelsSet, Issuer: "operator:test",
		Payload: json.RawMessage(`{"channels":{"warm_a":50}}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidDevice) {
		t.Fatalf("generic lighting profile err=%v want ErrInvalidDevice", err)
	}

	config := lightingnode.Configuration{
		SchemaVersion: lightingnode.SchemaVersion1,
		Channels: []lightingnode.ChannelConfig{
			{
				ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm",
				Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true,
			},
			{
				ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Cold",
				Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
			},
		},
	}
	configPayload, err := json.Marshal(lightingnode.ConfigApplyPayload{Configuration: config})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "lighting-01",
		CommandType: lightingnode.CommandConfigApply, Issuer: "operator:test",
		Payload: configPayload,
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("config apply without snapshot err=%v want ErrInvalidState", err)
	}

	project, err := stageStore.GetProject(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.SetLightingNodeBinding(ctx, project.CurrentRevisionID, lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID,
		Configuration: config,
		Aliases: map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, project.CurrentRevisionID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(stageStore).Create(ctx, project.CurrentRevisionID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "lighting-01",
		CommandType: lightingnode.CommandConfigApply, Issuer: "operator:test",
		RuntimeSnapshotID: published.ID, Payload: configPayload,
	}); err != nil {
		t.Fatalf("authoritative config apply: %v", err)
	}

	mismatch := config
	mismatch.Channels = append([]lightingnode.ChannelConfig(nil), config.Channels...)
	mismatch.Channels[0].MaximumLevel = 70
	mismatchPayload, _ := json.Marshal(lightingnode.ConfigApplyPayload{Configuration: mismatch})
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "lighting-01",
		CommandType: lightingnode.CommandConfigApply, Issuer: "operator:test",
		RuntimeSnapshotID: published.ID, Payload: mismatchPayload,
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("mismatched config apply err=%v want ErrInvalidState", err)
	}

	show, err := stageStore.CreateSession(ctx, published.ID, domain.SessionShow, "lighting safety")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "lighting-01",
		CommandType: lightingnode.CommandIdentify, Issuer: "operator:test",
		Payload: json.RawMessage(`{"channel_key":"warm_a","level":25,"duration_ms":500}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("identify during SHOW err=%v want ErrInvalidState", err)
	}
	if err := stageStore.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
}
