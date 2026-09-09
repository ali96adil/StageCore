package deviceexperience_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/store"
)

var phase4Time = time.Date(2026, 9, 8, 15, 30, 0, 0, time.UTC)

func newRepository(t *testing.T) (*deviceexperience.Repository, *db.Handle, string) {
	t.Helper()
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	stageStore := store.New(h.DB, clock.Fixed{Time: phase4Time})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Phase 4 Test", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := deviceexperience.NewRepository(h.DB, deviceexperience.WithClock(func() time.Time { return phase4Time }))
	if err != nil {
		t.Fatal(err)
	}
	return repo, h, project.ID
}

func TestDeviceIdentityRuntimeAndProtocol(t *testing.T) {
	ctx := context.Background()
	repo, _, projectID := newRepository(t)
	device, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "tablet-01", ProjectID: projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Tablet 01", Platform: "android", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"tablet.media.play", "tablet.media.prepare", "tablet.media.play"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(device.Capabilities) != 2 || device.Capabilities[0] != "tablet.media.play" || device.Capabilities[1] != "tablet.media.prepare" {
		t.Fatalf("capabilities=%v", device.Capabilities)
	}
	state, err := repo.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: "tablet-01", Connection: deviceexperience.ConnectionOnline, Readiness: deviceexperience.ReadinessReady,
		ObservedState: json.RawMessage(`{"media":"01.mp4","state":"PREPARED"}`),
		NetworkState:  json.RawMessage(`{"transport":"TLS"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Connection != deviceexperience.ConnectionOnline || !state.LastSeenAt.Equal(phase4Time) {
		t.Fatalf("runtime state=%+v", state)
	}
	loaded, err := repo.GetDevice(ctx, "tablet-01")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Runtime == nil || loaded.Runtime.Readiness != deviceexperience.ReadinessReady {
		t.Fatalf("loaded runtime=%+v", loaded.Runtime)
	}
	_, err = repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "bad", ProjectID: projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Bad", ProtocolVersion: "stagecore.device/2", Enabled: true,
	})
	if !errors.Is(err, deviceexperience.ErrInvalidDevice) {
		t.Fatalf("unsupported protocol err=%v", err)
	}
}

func TestCommandCapabilityExpiryIdempotencyAndCompletion(t *testing.T) {
	ctx := context.Background()
	repo, _, projectID := newRepository(t)
	_, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "tablet-01", ProjectID: projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Tablet 01", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"tablet.media.play"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	past := phase4Time.Add(-time.Second)
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "tablet-01", CommandType: "TABLET_PLAY", Issuer: "operator:test", DeadlineAt: &past,
	}); !errors.Is(err, deviceexperience.ErrCommandExpired) {
		t.Fatalf("expired command err=%v", err)
	}
	if _, _, err := repo.CreateCommand(ctx, deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "tablet-01", CommandType: "TABLET_PAUSE", Issuer: "operator:test",
	}); !errors.Is(err, deviceexperience.ErrCapabilityMissing) {
		t.Fatalf("missing capability err=%v", err)
	}
	deadline := phase4Time.Add(5 * time.Second)
	input := deviceexperience.CreateCommandInput{
		ProjectID: projectID, DeviceID: "tablet-01", CommandType: "TABLET_PLAY", Issuer: "operator:test",
		RuntimeSnapshotID: "snapshot-1", CorrelationID: "corr-1", IdempotencyKey: "go-42",
		Payload: json.RawMessage(`{"media_key":"01"}`), DeadlineAt: &deadline,
	}
	first, reused, err := repo.CreateCommand(ctx, input)
	if err != nil || reused {
		t.Fatalf("first err=%v reused=%v", err, reused)
	}
	second, reused, err := repo.CreateCommand(ctx, input)
	if err != nil || !reused || second.Envelope.CommandID != first.Envelope.CommandID {
		t.Fatalf("idempotency second=%+v reused=%v err=%v", second, reused, err)
	}
	if second.Envelope.RuntimeSnapshotID != "snapshot-1" {
		t.Fatalf("snapshot=%q", second.Envelope.RuntimeSnapshotID)
	}
	completed, err := repo.CompleteCommand(ctx, first.Envelope.CommandID, contracts.CommandCompleted, json.RawMessage(`{"ack":"DEVICE_ACK"}`))
	if err != nil || completed.Status != contracts.CommandCompleted {
		t.Fatalf("complete=%+v err=%v", completed, err)
	}
	repeated, err := repo.CompleteCommand(ctx, first.Envelope.CommandID, contracts.CommandFailed, json.RawMessage(`{"ignored":true}`))
	if err != nil || repeated.Status != contracts.CommandCompleted {
		t.Fatalf("terminal replay changed result: %+v err=%v", repeated, err)
	}
}

func TestDisplaySourceAndCockpitState(t *testing.T) {
	ctx := context.Background()
	repo, h, projectID := newRepository(t)
	_, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "display-01", ProjectID: projectID, Kind: deviceexperience.DeviceStageDisplay,
		DisplayName: "Callboard", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"display.message.show", "display.countdown.show", "video.source.open"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	expires := phase4Time.Add(time.Minute)
	display, err := repo.SetDisplayState(ctx, deviceexperience.DisplayState{
		DeviceID: "display-01", Mode: deviceexperience.DisplayCountdown,
		Payload: json.RawMessage(`{"target_at":"2026-09-08T15:31:00Z"}`), ExpiresAt: &expires,
	})
	if err != nil || display.Mode != deviceexperience.DisplayCountdown {
		t.Fatalf("display=%+v err=%v", display, err)
	}
	_, err = repo.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: projectID, Name: "USB Capture 1", Class: deviceexperience.SourceUSBCapture,
		ExecutionDeviceID: "display-01", Capabilities: []string{"video.source.open"},
		Config: json.RawMessage(`{"port":"capture-1"}`), Required: true, DesiredEnabled: true,
		Readiness: deviceexperience.ReadinessReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := repo.ListLiveSources(ctx, projectID)
	if err != nil || len(sources) != 1 || !sources[0].Required {
		t.Fatalf("sources=%+v err=%v", sources, err)
	}

	_, err = repo.RecordNetworkObservation(ctx, deviceexperience.NetworkObservation{
		TargetKind: "STAGE_DEVICE", TargetID: "display-01", Reachability: deviceexperience.Reachable,
		TransportState: "CONNECTED", ObservedAt: phase4Time,
	})
	if err != nil {
		t.Fatal(err)
	}
	latency := 125.0
	_, err = repo.RecordNetworkObservation(ctx, deviceexperience.NetworkObservation{
		TargetKind: "ENDPOINT", TargetID: "camera-net", Reachability: deviceexperience.Reachable,
		TransportState: "CONNECTED", LatencyMS: &latency, ObservedAt: phase4Time,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.RecordNetworkObservation(ctx, deviceexperience.NetworkObservation{
		TargetKind: "COMPANION", TargetID: "old", Reachability: deviceexperience.Reachable,
		TransportState: "CONNECTED", ObservedAt: phase4Time.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	cockpit, err := repo.Cockpit(ctx, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]deviceexperience.CockpitTarget{}
	for _, target := range cockpit {
		got[target.TargetKind+":"+target.TargetID] = target
	}
	if target := got["STAGE_DEVICE:display-01"]; target.Readiness != deviceexperience.ReadinessReady || target.Observation.LatencyMS != nil {
		t.Fatalf("unmeasured latency should remain nil and ready: %+v", target)
	}
	if target := got["ENDPOINT:camera-net"]; target.Readiness != deviceexperience.ReadinessWarning || target.ReasonCode != "LATENCY_ELEVATED" {
		t.Fatalf("latency classification=%+v", target)
	}
	if target := got["COMPANION:old"]; !target.Stale || target.ReasonCode != "STALE_OBSERVATION" {
		t.Fatalf("stale classification=%+v", target)
	}

	for i := 0; i < 3; i++ {
		_, err := repo.RecordNetworkObservation(ctx, deviceexperience.NetworkObservation{
			TargetKind: "ENDPOINT", TargetID: "prune", Reachability: deviceexperience.Reachable,
			TransportState: "CONNECTED", ObservedAt: phase4Time.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.PruneNetworkObservations(ctx, 1); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := h.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM network_observations WHERE target_kind='ENDPOINT' AND target_id='prune'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("pruned count=%d", count)
	}
}
