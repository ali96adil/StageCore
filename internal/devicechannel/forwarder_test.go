package devicechannel_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/devicechannel"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

type dispatchFunc func(context.Context, deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error)

func (f dispatchFunc) Dispatch(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
	return f(ctx, input)
}

type forwarderFixture struct {
	db         *sql.DB
	store      *store.Store
	repository *deviceexperience.Repository
	projectID  string
	snapshotID string
	sessionID  string
	now        time.Time
}

func newForwarderFixture(t *testing.T) forwarderFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	stageStore := store.New(h.DB, clock.Fixed{Time: now})
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Cue Stage Device", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshot, err := stageStore.CreateRuntimeSnapshot(ctx, revision.ID, "test", strings.Repeat("b", 64), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	session, err := stageStore.CreateSession(ctx, snapshot.ID, domain.SessionRehearsal, "Cue dispatch")
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
		DisplayName: "Stage Left Callboard", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"display.message.show"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	return forwarderFixture{db: h.DB, store: stageStore, repository: repository, projectID: project.ID, snapshotID: snapshot.ID, sessionID: session.ID, now: now}
}

func stageDeviceRequest(f forwarderFixture, executionID string) capability.Request {
	return capability.Request{
		ExecutionID:       executionID,
		ProjectID:         f.projectID,
		SessionID:         f.sessionID,
		RuntimeSnapshotID: f.snapshotID,
		Issuer:            "hub.cue_engine",
		CausationID:       "action.started:" + executionID,
		Capability:        "display.message.show",
		Target: &capability.Target{
			Ref:           "callboard-stage-left",
			LogicalType:   devicechannel.StageDeviceLogicalType,
			Configuration: json.RawMessage(`{"device_id":"display-01"}`),
		},
		Parameters:    json.RawMessage(`{"message":"Places"}`),
		Priority:      "P1",
		TimeoutMS:     250,
		CorrelationID: "corr-" + executionID,
	}
}

func TestForwarderDispatchesTypedCommandAndWaitsForDeviceResult(t *testing.T) {
	fixture := newForwarderFixture(t)
	dispatcher := dispatchFunc(func(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		command, _, err := fixture.repository.CreateCommand(ctx, input)
		if err != nil {
			return deviceexperience.DeviceCommand{}, err
		}
		go func(commandID string) {
			time.Sleep(15 * time.Millisecond)
			_, _ = fixture.repository.CompleteCommand(context.Background(), commandID, contracts.CommandCompleted, json.RawMessage(`{"ack":"DEVICE_ACK"}`))
		}(command.Envelope.CommandID)
		return command, nil
	})
	forwarder := devicechannel.NewForwarder(fixture.store, fixture.repository, dispatcher)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := forwarder.Execute(ctx, stageDeviceRequest(fixture, "action-1"))
	if result.Result != domain.ExecutionCompleted || result.AckLevel != contracts.AckDevice || result.ErrorCode != "" {
		t.Fatalf("result=%+v", result)
	}
	events, err := fixture.store.ListEvents(context.Background(), fixture.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].EventType != "stage_device.command.accepted" || events[1].EventType != "stage_device.command.completed" {
		t.Fatalf("events=%+v", events)
	}
	if events[0].CorrelationID != "corr-action-1" || events[0].CausationID != "action.started:action-1" {
		t.Fatalf("event correlation=%+v", events[0])
	}
}

func TestForwarderTimeoutBecomesTerminalAndIdempotent(t *testing.T) {
	fixture := newForwarderFixture(t)
	dispatcher := dispatchFunc(func(ctx context.Context, input deviceexperience.CreateCommandInput) (deviceexperience.DeviceCommand, error) {
		command, _, err := fixture.repository.CreateCommand(ctx, input)
		return command, err
	})
	forwarder := devicechannel.NewForwarder(fixture.store, fixture.repository, dispatcher)
	request := stageDeviceRequest(fixture, "action-timeout")
	request.TimeoutMS = 25
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	result := forwarder.Execute(ctx, request)
	cancel()
	if result.Result != domain.ExecutionTimedOut || result.ErrorCode != "STAGE_DEVICE_COMMAND_TIMED_OUT" {
		t.Fatalf("timeout result=%+v", result)
	}

	secondCtx, secondCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer secondCancel()
	second := forwarder.Execute(secondCtx, request)
	if second.Result != domain.ExecutionTimedOut {
		t.Fatalf("duplicate result=%+v", second)
	}

	var commands int
	if err := fixture.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM stage_device_commands`).Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if commands != 1 {
		t.Fatalf("commands=%d want=1", commands)
	}
	events, err := fixture.store.ListEvents(context.Background(), fixture.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].EventType != "stage_device.command.timed_out" {
		t.Fatalf("events=%+v", events)
	}
}
