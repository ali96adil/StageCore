package lightingnode

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
)

func TestSimulatorRejectsExpiredCommand(t *testing.T) {
	now := time.Date(2026, 9, 18, 7, 0, 0, 0, time.UTC)
	sim := newTestSimulator(t, func() time.Time { return now })
	deadline := now.Add(-time.Millisecond)

	submission := sim.Submit(contracts.CommandEnvelope{
		CommandID:     "expired-1",
		CommandType:   CommandChannelsSet,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      now.Add(-time.Second),
		DeadlineAt:    &deadline,
		Payload:       mustJSON(t, ChannelsSetPayload{Channels: map[string]float64{"front_warm": 50}}),
	})

	if submission.Result.Status != contracts.CommandRejected {
		t.Fatalf("status=%s want=%s", submission.Result.Status, contracts.CommandRejected)
	}
	if submission.Result.Error == nil || submission.Result.Error.ErrorCode != "COMMAND_EXPIRED" {
		t.Fatalf("result=%+v", submission.Result)
	}
	if sim.ExecutionCount("expired-1") != 0 {
		t.Fatalf("expired command executed")
	}
	if got := sim.CurrentLevels()["front_warm"]; got != 0 {
		t.Fatalf("expired command changed output to %v", got)
	}
}

func TestSimulatorDeduplicatesCommandID(t *testing.T) {
	now := time.Date(2026, 9, 18, 7, 0, 0, 0, time.UTC)
	sim := newTestSimulator(t, func() time.Time { return now })
	command := contracts.CommandEnvelope{
		CommandID:     "set-1",
		CommandType:   CommandChannelsSet,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      now,
		Payload:       mustJSON(t, ChannelsSetPayload{Channels: map[string]float64{"front_warm": 70}}),
	}

	first := sim.Submit(command)
	second := sim.Submit(command)

	if first.Result.Status != contracts.CommandCompleted || second.Result.Status != contracts.CommandCompleted {
		t.Fatalf("first=%+v second=%+v", first.Result, second.Result)
	}
	if sim.ExecutionCount(command.CommandID) != 1 {
		t.Fatalf("execution count=%d want=1", sim.ExecutionCount(command.CommandID))
	}
	if got := sim.CurrentLevels()["front_warm"]; got != 70 {
		t.Fatalf("front_warm=%v want=70", got)
	}
}

func TestSimulatorReconnectDoesNotReplayCommand(t *testing.T) {
	now := time.Date(2026, 9, 18, 7, 0, 0, 0, time.UTC)
	sim := newTestSimulator(t, func() time.Time { return now })
	command := contracts.CommandEnvelope{
		CommandID:     "set-before-reconnect",
		CommandType:   CommandChannelsSet,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      now,
		Payload:       mustJSON(t, ChannelsSetPayload{Channels: map[string]float64{"front_cold": 60}}),
	}

	if got := sim.Submit(command).Result.Status; got != contracts.CommandCompleted {
		t.Fatalf("initial status=%s", got)
	}
	sim.Disconnect()
	sim.Reconnect()

	duplicate := sim.Submit(command)
	if duplicate.Result.Status != contracts.CommandCompleted {
		t.Fatalf("duplicate status=%s", duplicate.Result.Status)
	}
	if sim.ExecutionCount(command.CommandID) != 1 {
		t.Fatalf("reconnect replayed command: count=%d", sim.ExecutionCount(command.CommandID))
	}
	if got := sim.CurrentLevels()["front_cold"]; got != 60 {
		t.Fatalf("reconnect changed output to %v", got)
	}
}

func TestSimulatorNewerFadeSupersedesOlderFadeDeterministically(t *testing.T) {
	current := time.Date(2026, 9, 18, 7, 0, 0, 0, time.UTC)
	sim := newTestSimulator(t, func() time.Time { return current })

	first := sim.Submit(contracts.CommandEnvelope{
		CommandID:     "fade-1",
		CommandType:   CommandChannelsFade,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      current,
		Payload:       mustJSON(t, ChannelsFadePayload{FadeMS: 1000, Channels: map[string]float64{"front_warm": 80}}),
	})
	if first.Result.Status != contracts.CommandAccepted {
		t.Fatalf("first fade status=%s", first.Result.Status)
	}

	current = current.Add(500 * time.Millisecond)
	if terminal := sim.Advance(current); terminal != nil {
		t.Fatalf("fade completed too early: %+v", terminal)
	}
	if got := sim.CurrentLevels()["front_warm"]; got != 40 {
		t.Fatalf("half fade level=%v want=40", got)
	}

	second := sim.Submit(contracts.CommandEnvelope{
		CommandID:     "fade-2",
		CommandType:   CommandChannelsFade,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      current,
		Payload:       mustJSON(t, ChannelsFadePayload{FadeMS: 500, Channels: map[string]float64{"front_warm": 20}}),
	})
	if second.Result.Status != contracts.CommandAccepted {
		t.Fatalf("second fade status=%s", second.Result.Status)
	}
	if second.Superseded == nil || second.Superseded.CommandID != "fade-1" || second.Superseded.Status != contracts.CommandCancelled {
		t.Fatalf("superseded=%+v", second.Superseded)
	}

	duplicateOld := sim.Submit(contracts.CommandEnvelope{CommandID: "fade-1", CommandType: CommandChannelsFade})
	if duplicateOld.Result.Status != contracts.CommandCancelled {
		t.Fatalf("old duplicate status=%s want=%s", duplicateOld.Result.Status, contracts.CommandCancelled)
	}
	if sim.ExecutionCount("fade-1") != 1 || sim.ExecutionCount("fade-2") != 1 {
		t.Fatalf("execution counts old=%d new=%d", sim.ExecutionCount("fade-1"), sim.ExecutionCount("fade-2"))
	}

	current = current.Add(500 * time.Millisecond)
	terminal := sim.Advance(current)
	if terminal == nil || terminal.Status != contracts.CommandCompleted || terminal.CommandID != "fade-2" {
		t.Fatalf("terminal=%+v", terminal)
	}
	if got := sim.CurrentLevels()["front_warm"]; got != 20 {
		t.Fatalf("final level=%v want=20", got)
	}
}

func TestSimulatorBlackoutIsImmediateAndInvalidChannelFailsClosed(t *testing.T) {
	now := time.Date(2026, 9, 18, 7, 0, 0, 0, time.UTC)
	sim := newTestSimulator(t, func() time.Time { return now })

	set := sim.Submit(contracts.CommandEnvelope{
		CommandID:     "set-before-blackout",
		CommandType:   CommandChannelsSet,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      now,
		Payload:       mustJSON(t, ChannelsSetPayload{Channels: map[string]float64{"front_warm": 80, "front_cold": 75}}),
	})
	if set.Result.Status != contracts.CommandCompleted {
		t.Fatalf("set=%+v", set.Result)
	}

	invalid := sim.Submit(contracts.CommandEnvelope{
		CommandID:     "invalid-channel",
		CommandType:   CommandChannelsSet,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      now,
		Payload:       mustJSON(t, ChannelsSetPayload{Channels: map[string]float64{"missing": 100}}),
	})
	if invalid.Result.Status != contracts.CommandRejected || invalid.Result.Error == nil {
		t.Fatalf("invalid=%+v", invalid.Result)
	}
	if got := sim.CurrentLevels()["front_warm"]; got != 80 {
		t.Fatalf("invalid command mutated existing output: %v", got)
	}

	blackout := sim.Submit(contracts.CommandEnvelope{
		CommandID:     "blackout-1",
		CommandType:   CommandBlackout,
		SchemaVersion: contracts.SchemaVersion1,
		IssuedAt:      now,
		Priority:      "P0",
		Payload:       mustJSON(t, BlackoutPayload{}),
	})
	if blackout.Result.Status != contracts.CommandCompleted {
		t.Fatalf("blackout=%+v", blackout.Result)
	}
	for key, level := range sim.CurrentLevels() {
		if level != 0 {
			t.Fatalf("channel %s remains at %v after blackout", key, level)
		}
	}
}

func TestLevelToDMXUsesNormalizedRangeLimitsAndInversion(t *testing.T) {
	channel := ChannelConfig{
		ChannelKey: "front_warm", ChannelNumber: 1, DisplayName: "Front Warm",
		Kind: ChannelWarmWhite, MinimumLevel: 0, MaximumLevel: 80, Enabled: true,
	}
	value, err := LevelToDMX(channel, 100)
	if err != nil {
		t.Fatal(err)
	}
	if value != 204 {
		t.Fatalf("DMX value=%d want=204", value)
	}

	channel.Inverted = true
	value, err = LevelToDMX(channel, 0)
	if err != nil {
		t.Fatal(err)
	}
	if value != 255 {
		t.Fatalf("inverted DMX value=%d want=255", value)
	}
}

func newTestSimulator(t *testing.T, now func() time.Time) *Simulator {
	t.Helper()
	sim, err := NewSimulator(Configuration{
		SchemaVersion: SchemaVersion1,
		Channels: []ChannelConfig{
			{
				ChannelKey: "front_warm", ChannelNumber: 1, DisplayName: "Front Warm",
				Kind: ChannelWarmWhite, PhysicalZone: "front", MinimumLevel: 0, MaximumLevel: 80, Enabled: true,
			},
			{
				ChannelKey: "front_cold", ChannelNumber: 2, DisplayName: "Front Cold",
				Kind: ChannelColdWhite, PhysicalZone: "front", MinimumLevel: 0, MaximumLevel: 100, Enabled: true,
			},
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return sim
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
