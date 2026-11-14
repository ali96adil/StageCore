package devicechannel_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"golang.org/x/net/websocket"
)

func TestRuntimeRevokedDispatchIsTerminalAndNeverSent(t *testing.T) {
	f := newRuntimeFixture(t)
	device := f.connect(t)
	defer device.Close()
	ctx := capability.WithDispatchCheck(context.Background(), func(context.Context) error {
		return errors.New("grant revoked")
	})
	deadline := time.Now().Add(10 * time.Second)
	input := deviceexperience.CreateCommandInput{
		ProjectID: f.projectID, DeviceID: testDeviceID, CommandType: "TABLET_PLAY",
		Issuer: "operator:test", CorrelationID: "revoked", IdempotencyKey: "revoked",
		Payload: json.RawMessage(`{"media":"01.mp4"}`), DeadlineAt: &deadline,
	}
	command, err := f.runtime.Dispatch(ctx, input)
	if err != nil || command.Status != contracts.CommandFailed ||
		!strings.Contains(string(command.Result), capability.DispatchAuthorityRevokedCode) ||
		!strings.Contains(string(command.Result), `"retryable":false`) {
		t.Fatalf("revoked dispatch=%+v err=%v", command, err)
	}
	replay, err := f.runtime.Dispatch(context.Background(), input)
	if err != nil || replay.Envelope.CommandID != command.Envelope.CommandID || replay.Status != contracts.CommandFailed {
		t.Fatalf("revoked replay=%+v err=%v", replay, err)
	}
	_ = device.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var message map[string]any
	if err := websocket.JSON.Receive(device, &message); err == nil {
		t.Fatalf("revoked command reached socket: %v", message)
	}
}

func TestRuntimeRevocationAfterCommandAcceptancePreventsSocketWrite(t *testing.T) {
	f := newRuntimeFixture(t)
	device := f.connect(t)
	defer device.Close()

	boundary := make(chan struct{})
	release := make(chan struct{})
	authorized := true
	ctx := capability.WithDispatchCheck(context.Background(), func(context.Context) error {
		// Runtime.Dispatch has already committed the command before it asks the
		// connection to perform this final pre-write authorization check.
		close(boundary)
		<-release
		if !authorized {
			return capability.ErrDispatchAuthorityRevoked
		}
		return nil
	})
	deadline := time.Now().Add(10 * time.Second)
	input := deviceexperience.CreateCommandInput{
		ProjectID: f.projectID, DeviceID: testDeviceID, CommandType: "TABLET_PLAY",
		Issuer: "operator:test", CorrelationID: "revoke-after-acceptance",
		IdempotencyKey: "revoke-after-acceptance",
		Payload:        json.RawMessage(`{"media":"01.mp4"}`), DeadlineAt: &deadline,
	}
	type dispatchResult struct {
		command deviceexperience.DeviceCommand
		err     error
	}
	dispatched := make(chan dispatchResult, 1)
	go func() {
		command, err := f.runtime.Dispatch(ctx, input)
		dispatched <- dispatchResult{command: command, err: err}
	}()

	select {
	case <-boundary:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch never reached the socket write authorization boundary")
	}
	var commandID string
	var status contracts.CommandStatus
	if err := f.dbHandle.DB.QueryRowContext(context.Background(),
		`SELECT command_id, status FROM stage_device_commands WHERE idempotency_key = ?`, input.IdempotencyKey,
	).Scan(&commandID, &status); err != nil {
		t.Fatal(err)
	}
	if status != contracts.CommandAccepted {
		t.Fatalf("command status at final write boundary = %s, want ACCEPTED", status)
	}
	// Model revocation after persistence but before the physical WebSocket write.
	authorized = false
	close(release)
	var result dispatchResult
	select {
	case result = <-dispatched:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked dispatch did not finish")
	}
	if result.err != nil || result.command.Status != contracts.CommandFailed ||
		!strings.Contains(string(result.command.Result), capability.DispatchAuthorityRevokedCode) ||
		!strings.Contains(string(result.command.Result), `"retryable":false`) {
		t.Fatalf("revoked accepted command=%+v err=%v", result.command, result.err)
	}
	_ = device.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	var message map[string]any
	if err := websocket.JSON.Receive(device, &message); err == nil {
		t.Fatalf("revoked command reached the socket: %+v", message)
	}
	stored, err := f.repo.GetCommand(context.Background(), commandID)
	if err != nil || stored.Status != contracts.CommandFailed {
		t.Fatalf("revoked command was not durably terminalized: %+v err=%v", stored, err)
	}
}

func TestReconnectTerminatesInflightCommandWithoutReplay(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	first := f.connect(t)

	deadline := time.Now().Add(10 * time.Second)
	input := deviceexperience.CreateCommandInput{
		ProjectID:      f.projectID,
		DeviceID:       testDeviceID,
		CommandType:    "TABLET_PLAY",
		Issuer:         "operator:test",
		CorrelationID:  "corr-interrupted",
		IdempotencyKey: "cue-interrupted/tablet-01/play",
		Payload:        json.RawMessage(`{"media":"01.mp4"}`),
		DeadlineAt:     &deadline,
	}
	command, err := f.runtime.Dispatch(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var execute struct {
		Type    string `json:"type"`
		Command struct {
			CommandID string `json:"command_id"`
		} `json:"command"`
	}
	if err := websocket.JSON.Receive(first, &execute); err != nil {
		t.Fatal(err)
	}
	if execute.Type != "command.execute" || execute.Command.CommandID != command.Envelope.CommandID {
		t.Fatalf("execute=%+v command=%+v", execute, command)
	}

	// A replacement connection is the authoritative reconnect event. Registering
	// it closes the previous server-side runtime connection deterministically,
	// which must terminalize commands dispatched through that old generation.
	second := f.connect(t)
	defer second.Close()
	interrupted := waitForDeviceCommandStatus(t, f, command.Envelope.CommandID, contracts.CommandFailed)
	if !strings.Contains(string(interrupted.Result), "DEVICE_EXECUTION_INTERRUPTED") {
		t.Fatalf("interrupted result=%s", interrupted.Result)
	}
	if !strings.Contains(string(interrupted.Result), `"retryable":false`) {
		t.Fatalf("ambiguous interrupted command must not authorize retry: %s", interrupted.Result)
	}

	_ = second.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var replay map[string]any
	if err := websocket.JSON.Receive(second, &replay); err == nil {
		t.Fatalf("interrupted command replayed after reconnect: %+v", replay)
	}
	_ = second.SetReadDeadline(time.Time{})

	duplicate, err := f.runtime.Dispatch(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Envelope.CommandID != command.Envelope.CommandID || duplicate.Status != contracts.CommandFailed {
		t.Fatalf("idempotent interrupted duplicate=%+v original=%+v", duplicate, command)
	}
	_ = second.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	replay = nil
	if err := websocket.JSON.Receive(second, &replay); err == nil {
		t.Fatalf("terminal idempotent command was resent: %+v", replay)
	}
}

func TestReplacementConnectionCannotCompletePreviousGenerationCommand(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	first := f.connect(t)

	deadline := time.Now().Add(10 * time.Second)
	command, err := f.runtime.Dispatch(ctx, deviceexperience.CreateCommandInput{
		ProjectID:      f.projectID,
		DeviceID:       testDeviceID,
		CommandType:    "TABLET_PLAY",
		Issuer:         "operator:test",
		CorrelationID:  "corr-generation",
		IdempotencyKey: "cue-generation/tablet-01/play",
		Payload:        json.RawMessage(`{"media":"02.mp4"}`),
		DeadlineAt:     &deadline,
	})
	if err != nil {
		t.Fatal(err)
	}
	var execute map[string]any
	if err := websocket.JSON.Receive(first, &execute); err != nil {
		t.Fatal(err)
	}

	second := f.connect(t)
	defer second.Close()
	interrupted := waitForDeviceCommandStatus(t, f, command.Envelope.CommandID, contracts.CommandFailed)
	if !strings.Contains(string(interrupted.Result), "DEVICE_EXECUTION_INTERRUPTED") {
		t.Fatalf("replacement did not terminalize old command: %s", interrupted.Result)
	}

	if err := websocket.JSON.Send(second, map[string]any{
		"type":           "command.result",
		"schema_version": 1,
		"device_id":      testDeviceID,
		"command_id":     command.Envelope.CommandID,
		"status":         contracts.CommandCompleted,
		"payload":        map[string]any{"ack": "DEVICE_ACK"},
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	after, err := f.repo.GetCommand(ctx, command.Envelope.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != contracts.CommandFailed || !strings.Contains(string(after.Result), "DEVICE_EXECUTION_INTERRUPTED") {
		t.Fatalf("replacement connection changed old command truth: %+v result=%s", after, after.Result)
	}
	if !f.runtime.IsConnected(testDeviceID) {
		t.Fatal("ignoring a stale result disconnected the current Stage Device")
	}
}

func waitForDeviceCommandStatus(t *testing.T, f *runtimeFixture, commandID string, want contracts.CommandStatus) deviceexperience.DeviceCommand {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		command, err := f.repo.GetCommand(context.Background(), commandID)
		if err == nil && command.Status == want {
			return command
		}
		time.Sleep(20 * time.Millisecond)
	}
	command, err := f.repo.GetCommand(context.Background(), commandID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("command %s status=%s want=%s result=%s", commandID, command.Status, want, command.Result)
	return deviceexperience.DeviceCommand{}
}
