package deviceexperience_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCommandEventsUseCanonicalFlightRecorder(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 8, 17, 0, 0, 0, time.UTC)
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	stageStore := store.New(h.DB, clock.Fixed{Time: now})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Flight Recorder", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshot, err := stageStore.CreateRuntimeSnapshot(ctx, revision.ID, "test", strings.Repeat("a", 64), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	session, err := stageStore.CreateSession(ctx, snapshot.ID, domain.SessionRehearsal, "Phase 4 events")
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
		ID: "display-01", ProjectID: project.ID, Kind: deviceexperience.DeviceStageDisplay,
		DisplayName: "Callboard", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"display.message.show"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	deadline := now.Add(time.Second)
	command, duplicate, err := repository.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: project.ID, SessionID: session.ID, DeviceID: "display-01", CommandType: "DISPLAY_MESSAGE",
		Issuer: "hub.cue_engine", CorrelationID: "corr-phase4", CausationID: "action-execution-1",
		RuntimeSnapshotID: snapshot.ID, Priority: "P1", IdempotencyKey: "cue-action-1",
		Payload: json.RawMessage(`{"message":"Places"}`), DeadlineAt: &deadline,
	})
	if err != nil || duplicate {
		t.Fatalf("create command duplicate=%v err=%v", duplicate, err)
	}
	if _, err := repository.CompleteCommand(ctx, command.Envelope.CommandID, contracts.CommandCompleted, json.RawMessage(`{"ack":"DEVICE_ACK"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CompleteCommand(ctx, command.Envelope.CommandID, contracts.CommandFailed, json.RawMessage(`{"late":true}`)); err != nil {
		t.Fatal(err)
	}

	events, err := stageStore.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%d want=2: %+v", len(events), events)
	}
	if events[0].EventType != "stage_device.command.accepted" || events[1].EventType != "stage_device.command.completed" {
		t.Fatalf("event types=%s,%s", events[0].EventType, events[1].EventType)
	}
	for _, event := range events {
		if event.Source != "hub.stage_device_runtime" || event.ProjectID != project.ID || event.RuntimeSnapshotID != snapshot.ID {
			t.Fatalf("event context=%+v", event)
		}
		if event.CorrelationID != "corr-phase4" || event.CausationID != "action-execution-1" || event.Priority != "P1" {
			t.Fatalf("event correlation=%+v", event)
		}
	}
}
