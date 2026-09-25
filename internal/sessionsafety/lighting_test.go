package sessionsafety

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/lightingnode"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type fakeLightingRuntime struct {
	inputs   []deviceexperience.CreateCommandInput
	commands map[string]deviceexperience.DeviceCommand
	status   contracts.CommandStatus
}

func (f *fakeLightingRuntime) Dispatch(_ context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
	if f.commands == nil {
		f.commands = make(map[string]deviceexperience.DeviceCommand)
	}
	f.inputs = append(f.inputs, input)
	id := fmt.Sprintf("blackout-%d", len(f.inputs))
	status := f.status
	if status == "" {
		status = contracts.CommandCompleted
	}
	command := deviceexperience.DeviceCommand{
		Envelope: contracts.CommandEnvelope{CommandID: id},
		DeviceID: input.DeviceID,
		Status: status,
	}
	f.commands[id] = command
	return command, nil
}

func (f *fakeLightingRuntime) GetCommand(_ context.Context, commandID string) (deviceexperience.DeviceCommand, error) {
	command, ok := f.commands[commandID]
	if !ok {
		return deviceexperience.DeviceCommand{}, fmt.Errorf("command not found")
	}
	return command, nil
}

func TestBlackoutLightingUsesSessionSnapshotAndP0ImmediateBlackout(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	stageStore := store.New(handle.DB, clock.Real{})

	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Session Safety", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	manifest := snapshot.Manifest{
		SchemaVersion: snapshot.ManifestSchemaVersion,
		ProjectID: project.ID,
		RevisionID: revision.ID,
		RevisionNumber: revision.RevisionNumber,
		Cues: []snapshot.Cue{},
		LightingNodes: []lightingnode.ProjectBinding{{
			DeviceID: "lighting-01",
			ProfileID: lightingnode.ProfileID,
			Configuration: lightingnode.Configuration{
				SchemaVersion: lightingnode.SchemaVersion1,
				Channels: []lightingnode.ChannelConfig{{
					ChannelKey: "front_cold_a", ChannelNumber: 1, DisplayName: "Front Cold A",
					Kind: lightingnode.ChannelColdWhite, MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
				}},
			},
			Aliases: map[string]string{"front_cold_a": "front_cold_a"},
		}},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, err := stageStore.CreateRuntimeSnapshot(ctx, revision.ID, "test", strings.Repeat("a", 64), manifestJSON)
	if err != nil {
		t.Fatal(err)
	}
	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Rehearsal")
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeLightingRuntime{}
	stopCommand := contracts.CommandEnvelope{CommandID: "stop-1", CorrelationID: "stop-1"}
	if err := BlackoutLighting(ctx, stageStore, fake, fake, session, stopCommand); err != nil {
		t.Fatal(err)
	}
	if len(fake.inputs) != 1 {
		t.Fatalf("blackout dispatches=%d want 1", len(fake.inputs))
	}
	input := fake.inputs[0]
	if input.ProjectID != project.ID || input.SessionID != session.ID || input.DeviceID != "lighting-01" {
		t.Fatalf("blackout authority=%+v", input)
	}
	if input.CommandType != lightingnode.CommandBlackout || input.Priority != "P0" || input.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("blackout command=%+v", input)
	}
	if input.CausationID != stopCommand.CommandID || input.CorrelationID != stopCommand.CorrelationID {
		t.Fatalf("blackout trace=%+v", input)
	}
	var payload lightingnode.BlackoutPayload
	if err := json.Unmarshal(input.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.FadeMS != 0 {
		t.Fatalf("blackout fade_ms=%d want 0", payload.FadeMS)
	}
}
