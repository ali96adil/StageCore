package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deviceexperience"
)

func TestDisplayCountdownCanonicalizesRelativeDurationToAbsoluteTarget(t *testing.T) {
	ctx := context.Background()
	repo, _, projectID := newRepository(t)
	if _, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "display-countdown", ProjectID: projectID, Kind: deviceexperience.DeviceStageDisplay,
		DisplayName: "Countdown", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"display.countdown.show"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	command, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "display-countdown", CommandType: "DISPLAY_COUNTDOWN", Issuer: "operator:test",
		Payload: json.RawMessage(`{"duration_seconds":60,"message":"Places"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(command.Envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["duration_seconds"]; exists {
		t.Fatalf("relative duration leaked into canonical payload: %s", command.Envelope.Payload)
	}
	rawTarget, _ := payload["target_at"].(string)
	target, err := time.Parse(time.RFC3339Nano, rawTarget)
	if err != nil {
		t.Fatalf("target_at=%q err=%v payload=%s", rawTarget, err, command.Envelope.Payload)
	}
	if !target.Equal(phase4Time.Add(time.Minute)) {
		t.Fatalf("target=%s want=%s", target, phase4Time.Add(time.Minute))
	}
}

func TestDisplayCountdownRejectsPastAbsoluteTarget(t *testing.T) {
	ctx := context.Background()
	repo, _, projectID := newRepository(t)
	if _, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "display-past", ProjectID: projectID, Kind: deviceexperience.DeviceStageDisplay,
		DisplayName: "Past", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"display.countdown.show"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"target_at": phase4Time.Add(-time.Second).Format(time.RFC3339Nano)})
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "display-past", CommandType: "DISPLAY_COUNTDOWN", Issuer: "operator:test", Payload: payload,
	}); !errors.Is(err, deviceexperience.ErrCommandExpired) {
		t.Fatalf("past countdown err=%v want ErrCommandExpired", err)
	}
}

func TestDisplayMessageAndVideoSourcePayloadValidation(t *testing.T) {
	ctx := context.Background()
	repo, _, projectID := newRepository(t)
	if _, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "multi-device", ProjectID: projectID, Kind: deviceexperience.DeviceGeneric,
		DisplayName: "Multi", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"display.message.show", "video.source.open"}, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "multi-device", CommandType: "DISPLAY_MESSAGE", Issuer: "operator:test", Payload: json.RawMessage(`{"message":"   "}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("empty display message err=%v want ErrInvalidState", err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "multi-device", CommandType: "VIDEO_SOURCE_OPEN", Issuer: "operator:test", Payload: json.RawMessage(`{"endpoint_ref":"capture-1"}`),
	}); !errors.Is(err, deviceexperience.ErrInvalidState) {
		t.Fatalf("video command without source_id err=%v want ErrInvalidState", err)
	}
	command, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "multi-device", CommandType: "VIDEO_SOURCE_OPEN", Issuer: "operator:test", Payload: json.RawMessage(`{"source_id":" source-1 "}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(command.Envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["source_id"] != "source-1" {
		t.Fatalf("canonical source payload=%s", command.Envelope.Payload)
	}
}
