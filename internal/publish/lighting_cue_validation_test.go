package publish

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestValidateRejectsInvalidLightingCueAlias(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Publish Alias"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting 01",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{lightingnode.CapabilityChannelsFade}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID,
		Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
			ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm",
			Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
		}}},
		Aliases: map[string]string{"front_warm": "warm_a"},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "lighting.front.fade", LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef: "lighting-01", ProjectConfig: json.RawMessage(`{"device_id":"lighting-01","capability_key":"lighting.channels.fade"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, Name: "Bad lighting alias", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "PARALLEL_BARRIER", TargetRef: "lighting.front.fade",
		CapabilityKey: lightingnode.CapabilityChannelsFade,
		Parameters: json.RawMessage(`{"fade_ms":1000,"aliases":{"typo_alias":50}}`),
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":4000}`), ErrorPolicy: json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}

	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(devicechannel.StageDeviceLogicalType, capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{}
	})); err != nil {
		t.Fatal(err)
	}
	report, err := New(s, registry).Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasFinding(report, "LIGHTING_CUE_INVALID") {
		t.Fatalf("invalid lighting alias must block publish: %#v", report)
	}
}

func TestValidateRejectsLightingCommissioningCapabilityInCue(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Publish Commissioning"})
	if err != nil {
		t.Fatal(err)
	}
	devices, err := deviceexperience.NewRepository(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devices.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Lighting 01",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{lightingnode.CapabilityConfigApply}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID,
		Configuration: lightingnode.Configuration{SchemaVersion: 1, Channels: []lightingnode.ChannelConfig{{
			ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Warm",
			Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
		}}},
		Aliases: map[string]string{"front_warm": "warm_a"},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "lighting.front.config", LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef: "lighting-01", ProjectConfig: json.RawMessage(`{"device_id":"lighting-01","capability_key":"lighting.config.apply"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, Name: "Forbidden config Cue", OrderIndex: 0, Enabled: true,
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "lighting.front.config",
		CapabilityKey: lightingnode.CapabilityConfigApply, Parameters: json.RawMessage(`{"configuration":{"schema_version":1,"channels":[{"channel_key":"warm_a","channel_number":1,"display_name":"Warm","kind":"WARM_WHITE","minimum_level":0,"maximum_level":100,"inverted":false,"enabled":true}]}}`),
		TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}

	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(devicechannel.StageDeviceLogicalType, capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{}
	})); err != nil {
		t.Fatal(err)
	}
	report, err := New(s, registry).Validate(ctx, project.ID, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !hasFinding(report, "LIGHTING_CUE_CAPABILITY_FORBIDDEN") {
		t.Fatalf("commissioning lighting capability must block publish: %#v", report)
	}
}
