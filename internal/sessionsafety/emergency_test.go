package sessionsafety

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/stagelaser"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

type fakeEmergencyRuntime struct {
	devices  []deviceexperience.Device
	inputs   []deviceexperience.CreateCommandInput
	commands map[string]deviceexperience.DeviceCommand
}

func (f *fakeEmergencyRuntime) ListDevices(context.Context, string) ([]deviceexperience.Device, error) {
	return append([]deviceexperience.Device(nil), f.devices...), nil
}

func (f *fakeEmergencyRuntime) Dispatch(_ context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
	if f.commands == nil {
		f.commands = map[string]deviceexperience.DeviceCommand{}
	}
	f.inputs = append(f.inputs, input)
	id := "emergency-device-" + string(rune('0'+len(f.inputs)))
	command := deviceexperience.DeviceCommand{
		Envelope: contracts.CommandEnvelope{CommandID: id},
		DeviceID: input.DeviceID,
		Status: contracts.CommandCompleted,
	}
	f.commands[id] = command
	return command, nil
}

func (f *fakeEmergencyRuntime) GetCommand(_ context.Context, id string) (deviceexperience.DeviceCommand, error) {
	return f.commands[id], nil
}

type fakeEmergencyCapabilityExecutor struct {
	requests []capability.Request
}

func (f *fakeEmergencyCapabilityExecutor) Execute(_ context.Context, request capability.Request) capability.Result {
	f.requests = append(f.requests, request)
	return capability.Result{
		Result: domain.ExecutionCompleted,
		AckLevel: contracts.AckDevice,
		ResponseSummary: "blackout applied",
	}
}

func TestManagedOutputEmergencyBlackoutCoversTabletAndNativeVisualWithoutAudio(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	stageStore := store.New(handle.DB, clock.Real{})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Emergency domains", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-NATIVE", DisplayName: "Native Visual",
		RequiredCapabilities: []string{visualengine.CapabilityBlackout}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	roleConfig, _ := json.Marshal(map[string]string{"machine_role_id": role.ID})
	manifest := snapshot.Manifest{
		SchemaVersion: snapshot.ManifestSchemaVersion,
		ProjectID: project.ID,
		RevisionID: revision.ID,
		RevisionNumber: revision.RevisionNumber,
		Cues: []snapshot.Cue{},
		Targets: []snapshot.Target{{
			AliasID: "visual-alias",
			TargetRef: role.RoleKey,
			LogicalType: companion.MachineRoleLogicalType,
			Configuration: roleConfig,
		}},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, err := stageStore.CreateRuntimeSnapshot(ctx, revision.ID, "owner", strings.Repeat("a", 64), raw)
	if err != nil {
		t.Fatal(err)
	}
	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Emergency")
	if err != nil {
		t.Fatal(err)
	}

	devices := &fakeEmergencyRuntime{
		devices: []deviceexperience.Device{
			{
				ID: "tablet-1",
				ProjectID: project.ID,
				Kind: deviceexperience.DeviceTabletPlayer,
				ProtocolVersion: deviceexperience.ProtocolVersion1,
				Capabilities: []string{
					deviceexperience.CapabilityTabletBlackout,
					deviceexperience.CapabilityTabletBlackoutClear,
				},
				Enabled: true,
			},
			{
				ID: "stagelaser-1",
				ProfileID: stagelaser.ProfileID,
				Kind: deviceexperience.DeviceGeneric,
				ProtocolVersion: deviceexperience.ProtocolVersion2,
				Capabilities: []string{stagelaser.CapabilitySafeOff},
				Enabled: true,
				Assignment: &deviceexperience.AssignmentRecord{
					DeviceID: "stagelaser-1",
					ProjectID: project.ID,
					Epoch: 2,
					State: "ACTIVE",
					RuntimeSnapshotID: runtimeSnapshot.ID,
					RequiredForShow: true,
				},
			},
		},
	}
	visual := &fakeEmergencyCapabilityExecutor{}
	command := contracts.CommandEnvelope{CommandID: "emergency-1", CorrelationID: "emergency-1"}

	applied, err := SetManagedOutputBlackout(ctx, stageStore, devices, devices, visual, session, command, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Lighting.Status != "NOT_CONFIGURED" ||
		applied.StageLaser.Status != "COMPLETED" || applied.StageLaser.Attempted != 1 || applied.StageLaser.Completed != 1 ||
		applied.Tablets.Status != "COMPLETED" || applied.Tablets.Attempted != 1 || applied.Tablets.Completed != 1 ||
		applied.NativeVisual.Status != "COMPLETED" || applied.NativeVisual.Attempted != 1 || applied.NativeVisual.Completed != 1 ||
		applied.Audio.Status != "UNCHANGED_BY_DESIGN" ||
		applied.ExternalAdapters.Status != "UNCHANGED_BY_DESIGN" {
		t.Fatalf("applied report=%+v", applied)
	}
	if len(devices.inputs) != 2 ||
		devices.inputs[0].CommandType != stagelaser.CommandSafeOff ||
		devices.inputs[0].Priority != "P0" ||
		devices.inputs[0].RuntimeSnapshotID != runtimeSnapshot.ID ||
		devices.inputs[1].CommandType != deviceexperience.CommandTabletBlackout ||
		devices.inputs[1].Priority != "P0" ||
		devices.inputs[1].RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("managed blackout inputs=%+v", devices.inputs)
	}
	if len(visual.requests) != 1 || visual.requests[0].Capability != visualengine.CapabilityBlackout ||
		visual.requests[0].Priority != "P0" {
		t.Fatalf("visual blackout requests=%+v", visual.requests)
	}
	var visualPayload struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(visual.requests[0].Parameters, &visualPayload); err != nil || !visualPayload.Enabled {
		t.Fatalf("visual blackout payload=%s err=%v", visual.requests[0].Parameters, err)
	}

	cleared, err := SetManagedOutputBlackout(ctx, stageStore, devices, devices, visual, session, command, false)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.StageLaser.Status != "MANUAL_RECOVERY_REQUIRED" ||
		cleared.Tablets.Status != "COMPLETED" || cleared.NativeVisual.Status != "COMPLETED" ||
		cleared.Audio.Status != "UNCHANGED_BY_DESIGN" {
		t.Fatalf("cleared report=%+v", cleared)
	}
	if len(devices.inputs) != 3 || devices.inputs[2].CommandType != deviceexperience.CommandTabletBlackoutClear {
		t.Fatalf("managed clear inputs=%+v", devices.inputs)
	}
	for _, input := range devices.inputs[2:] {
		if input.CommandType == stagelaser.CommandArm || input.CommandType == stagelaser.CommandSetOn {
			t.Fatalf("managed clear must never re-arm or turn StageLaser on: %+v", devices.inputs)
		}
	}
	if len(visual.requests) != 2 {
		t.Fatalf("visual clear requests=%+v", visual.requests)
	}
	if err := json.Unmarshal(visual.requests[1].Parameters, &visualPayload); err != nil || visualPayload.Enabled {
		t.Fatalf("visual clear payload=%s err=%v", visual.requests[1].Parameters, err)
	}
}
