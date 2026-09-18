package cueengine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCueGoResolvesLightingAliasesThroughPublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	stageStore := store.New(h.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Lighting Cue E2E", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	repository, err := deviceexperience.NewRepository(h.DB, deviceexperience.WithEventRecorder(stageStore))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpsertDevice(ctx, deviceexperience.Device{
		ID: "lighting-01", ProjectID: project.ID, ProfileID: lightingnode.ProfileID,
		Kind: deviceexperience.DeviceGeneric, DisplayName: "Front Lighting",
		ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{lightingnode.CapabilityChannelsFade}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.SetLightingNodeBinding(ctx, revision.ID, lightingnode.ProjectBinding{
		DeviceID: "lighting-01", ProfileID: lightingnode.ProfileID,
		Configuration: lightingnode.Configuration{SchemaVersion: lightingnode.SchemaVersion1, Channels: []lightingnode.ChannelConfig{
			{ChannelKey: "warm_a", ChannelNumber: 1, DisplayName: "Front Warm", Kind: lightingnode.ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true},
			{ChannelKey: "cold_a", ChannelNumber: 2, DisplayName: "Front Cold", Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true},
		}},
		Aliases: map[string]string{"front_warm": "warm_a", "front_cold": "cold_a"},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "lighting.front.fade", LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef: "lighting-01", ProjectConfig: json.RawMessage(`{"device_id":"lighting-01","capability_key":"lighting.channels.fade"}`),
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "LX1", Name: "Front Fade", OrderIndex: 0,
		CueType: "LIGHTING_SCENE", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
	}, []domain.Action{{
		OrderIndex: 0, ExecutionMode: "PARALLEL_BARRIER", TargetRef: "lighting.front.fade",
		CapabilityKey: lightingnode.CapabilityChannelsFade,
		Parameters: json.RawMessage(`{"fade_ms":1200,"aliases":{"front_warm":55,"front_cold":20}}`),
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":4200}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, manifest, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	resolvedWarm := manifest.ResolveLightingChannel("front_warm")
	if resolvedWarm == nil || resolvedWarm.DeviceID != "lighting-01" || resolvedWarm.ChannelKey != "warm_a" {
		t.Fatalf("snapshot lighting mapping=%+v", resolvedWarm)
	}
	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Lighting E2E")
	if err != nil {
		t.Fatal(err)
	}

	var captured deviceexperience.CreateCommandInput
	dispatcher := cueDeviceDispatchFunc(func(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		captured = input
		command, _, err := repository.CreateCommand(ctx, input)
		if err != nil {
			return deviceexperience.DeviceCommand{}, err
		}
		go func(commandID string) {
			time.Sleep(15 * time.Millisecond)
			result, _ := json.Marshal(contracts.CommandResult{
				CommandID: commandID, Status: contracts.CommandCompleted,
				Payload: json.RawMessage(`{"levels":{"warm_a":55,"cold_a":20}}`),
			})
			_, _ = repository.CompleteCommand(context.Background(), commandID, contracts.CommandCompleted, result)
		}(command.Envelope.CommandID)
		return command, nil
	})
	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(
		devicechannel.StageDeviceLogicalType,
		devicechannel.NewForwarder(stageStore, repository, dispatcher),
	); err != nil {
		t.Fatal(err)
	}

	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(cueengine.CueGoPayload{})
	result := cueengine.NewWithExecutor(stageStore, registry).ExecuteCueGo(ctx, session.ID, contracts.CommandEnvelope{
		CommandID: commandID, CommandType: cueengine.CueGoCommandType, SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: time.Now().UTC(), ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer: "test.operator", CorrelationID: "corr-lighting-cue", Priority: "P1", Payload: payload,
	})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("cue result=%+v", result)
	}
	if captured.CommandType != lightingnode.CommandChannelsFade || captured.DeviceID != "lighting-01" {
		t.Fatalf("captured command=%+v", captured)
	}
	if strings.Contains(string(captured.Payload), "front_warm") || strings.Contains(string(captured.Payload), "front_cold") {
		t.Fatalf("logical aliases leaked to physical command: %s", captured.Payload)
	}
	var resolved lightingnode.ChannelsFadePayload
	if err := json.Unmarshal(captured.Payload, &resolved); err != nil {
		t.Fatal(err)
	}
	if resolved.FadeMS != 1200 || resolved.Channels["warm_a"] != 55 || resolved.Channels["cold_a"] != 20 {
		t.Fatalf("resolved payload=%+v raw=%s", resolved, captured.Payload)
	}

	cueExecutions, err := stageStore.ListCueExecutions(ctx, session.ID)
	if err != nil || len(cueExecutions) != 1 || cueExecutions[0].CueID != cue.ID || cueExecutions[0].Result != domain.ExecutionCompleted {
		t.Fatalf("cue executions=%+v err=%v", cueExecutions, err)
	}
	events, err := stageStore.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cue.timing_observed",
		"cue.started",
		"action.started",
		"stage_device.command.accepted",
		"stage_device.command.completed",
		"action.completed",
		"cue.completed",
	}
	if len(events) != len(want) {
		t.Fatalf("events=%v want=%v", eventTypes(events), want)
	}
	for i := range want {
		if events[i].EventType != want[i] {
			t.Fatalf("events=%v want=%v", eventTypes(events), want)
		}
		if events[i].CorrelationID != "corr-lighting-cue" {
			t.Fatalf("event correlation lost: %+v", events[i])
		}
	}
}
