package cueengine_test

import (
	"context"
	"encoding/json"
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
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type cueDeviceDispatchFunc func(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error)

func (f cueDeviceDispatchFunc) Dispatch(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
	return f(ctx, input)
}

func TestCueGoDispatchesStageDeviceActionThroughCanonicalRuntime(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	stageStore := store.New(h.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Cue Callboard", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "CALLBOARD_LEFT", LogicalType: devicechannel.StageDeviceLogicalType,
		TargetRef: "display-01", GroupName: "actors", ProjectConfig: json.RawMessage(`{"device_id":"display-01"}`),
	}); err != nil {
		t.Fatal(err)
	}
	action := domain.Action{
		OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "CALLBOARD_LEFT",
		CapabilityKey: "display.message.show", Parameters: json.RawMessage(`{"message":"Places"}`),
		TimeoutPolicy: json.RawMessage(`{"timeout_ms":500}`), ErrorPolicy: json.RawMessage(`{"on_error":"FAIL_CUE"}`),
		PriorityClass: domain.PriorityP1, Enabled: true,
	}
	cue, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "CB1", Name: "Places", OrderIndex: 0, Enabled: true,
	}, []domain.Action{action})
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
	if target := manifest.ResolveTarget("CALLBOARD_LEFT"); target == nil || target.LogicalType != devicechannel.StageDeviceLogicalType {
		t.Fatalf("stage device target missing from snapshot: %+v", target)
	}
	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Cue callboard test")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := deviceexperience.NewRepository(h.DB, deviceexperience.WithEventRecorder(stageStore))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpsertDevice(ctx, deviceexperience.Device{
		ID: "display-01", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay,
		DisplayName: "Stage Left", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"display.message.show"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	dispatcher := cueDeviceDispatchFunc(func(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		command, _, err := repository.CreateCommand(ctx, input)
		if err != nil {
			return deviceexperience.DeviceCommand{}, err
		}
		go func(commandID string) {
			time.Sleep(15 * time.Millisecond)
			_, _ = repository.CompleteCommand(context.Background(), commandID, contracts.CommandCompleted, json.RawMessage(`{"ack":"DEVICE_ACK"}`))
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
		Issuer: "test.operator", CorrelationID: "corr-stage-device-cue", Priority: "P1", Payload: payload,
	})
	if result.Status != contracts.CommandCompleted {
		t.Fatalf("cue result=%+v", result)
	}
	cueExecutions, err := stageStore.ListCueExecutions(ctx, session.ID)
	if err != nil || len(cueExecutions) != 1 || cueExecutions[0].CueID != cue.ID || cueExecutions[0].Result != domain.ExecutionCompleted {
		t.Fatalf("cue executions=%+v err=%v", cueExecutions, err)
	}
	actionExecutions, err := stageStore.ListActionExecutions(ctx, cueExecutions[0].ID)
	if err != nil || len(actionExecutions) != 1 || actionExecutions[0].Result != domain.ExecutionCompleted {
		t.Fatalf("action executions=%+v err=%v", actionExecutions, err)
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
		t.Fatalf("event count=%d want=%d events=%v", len(events), len(want), eventTypes(events))
	}
	for i := range want {
		if events[i].EventType != want[i] {
			t.Fatalf("events=%v want=%v", eventTypes(events), want)
		}
	}
	for _, event := range events {
		if event.CorrelationID != "corr-stage-device-cue" {
			t.Fatalf("event correlation lost: %+v", event)
		}
	}
	var completedPayload struct {
		AckLevel contracts.AckLevel `json:"ack_level"`
	}
	if err := json.Unmarshal(events[5].Payload, &completedPayload); err != nil {
		t.Fatal(err)
	}
	if completedPayload.AckLevel != contracts.AckDevice {
		t.Fatalf("action ack=%s", completedPayload.AckLevel)
	}
}
