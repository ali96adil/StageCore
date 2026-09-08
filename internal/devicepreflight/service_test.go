package devicepreflight_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deviceexperience"
	"github.com/ali96adil/StageCore/internal/devicepreflight"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/store"
)

type baseReport struct{ report preflight.Report }

func (b baseReport) Evaluate(context.Context, string, string) (preflight.Report, error) { return b.report, nil }

func TestRequiredLiveSourceBlocksWhenRenderNodeOffline(t *testing.T) {
	ctx := context.Background()
	repo, projectID := newFixture(t)
	render, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "render-01", ProjectID: projectID, Kind: deviceexperience.DeviceRenderNode,
		DisplayName: "Render 01", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"video.source.open"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: render.ID, Connection: deviceexperience.ConnectionOffline, Readiness: deviceexperience.ReadinessWarning,
	}); err != nil {
		t.Fatal(err)
	}
	source, err := repo.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: projectID, Name: "USB Capture", Class: deviceexperience.SourceUSBCapture,
		ExecutionDeviceID: render.ID, EndpointRef: "capture-1", Capabilities: []string{"video.source.open"},
		Config: json.RawMessage(`{"port":"capture-1"}`), Required: true, DesiredEnabled: true,
		Readiness: deviceexperience.ReadinessBlocker,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := baseReport{report: preflight.Report{Status: preflight.Pass, ProjectID: projectID, RuntimeSnapshotID: "snapshot-1", Checks: []preflight.Check{}}}
	service := devicepreflight.New(base, repo)
	report, err := service.Evaluate(ctx, projectID, "snapshot-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Block {
		t.Fatalf("status=%s want BLOCK checks=%+v", report.Status, report.Checks)
	}
	if !hasCheck(report.Checks, "live_video."+source.ID+".execution_device", preflight.Block) || !hasCheck(report.Checks, "live_video."+source.ID, preflight.Block) {
		t.Fatalf("required live source blockers missing: %+v", report.Checks)
	}
	allowed, reason, err := service.ShowGate(ctx, projectID, "snapshot-1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed || reason == "" {
		t.Fatalf("SHOW gate allowed=%v reason=%q", allowed, reason)
	}
}

func TestRequiredLiveSourceBlocksWhenRenderNodeCapabilityMissing(t *testing.T) {
	ctx := context.Background()
	repo, projectID := newFixture(t)
	render, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "render-capability", ProjectID: projectID, Kind: deviceexperience.DeviceRenderNode,
		DisplayName: "Render Capability", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"video.source.open"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ObserveDevice(ctx, deviceexperience.RuntimeObservation{
		DeviceID: render.ID, Connection: deviceexperience.ConnectionOnline, Readiness: deviceexperience.ReadinessReady,
	}); err != nil {
		t.Fatal(err)
	}
	source, err := repo.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: projectID, Name: "Routed Capture", Class: deviceexperience.SourceUSBCapture,
		ExecutionDeviceID: render.ID, EndpointRef: "capture-route",
		Capabilities: []string{"video.source.open", "video.source.route"},
		Config: json.RawMessage(`{"port":"capture-route"}`), Required: true, DesiredEnabled: true,
		Readiness: deviceexperience.ReadinessReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := baseReport{report: preflight.Report{Status: preflight.Pass, ProjectID: projectID, RuntimeSnapshotID: "snapshot-1", Checks: []preflight.Check{}}}
	report, err := devicepreflight.New(base, repo).Evaluate(ctx, projectID, "snapshot-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Block || !hasCheck(report.Checks, "live_video."+source.ID+".capabilities", preflight.Block) {
		t.Fatalf("missing capability did not block required source: %+v", report)
	}
	for _, check := range report.Checks {
		if check.Key == "live_video."+source.ID+".capabilities" && check.Detail != "video.source.route" {
			t.Fatalf("missing capability detail=%q", check.Detail)
		}
	}
}

func TestOptionalOfflineDeviceAndElevatedNetworkAreWarnings(t *testing.T) {
	ctx := context.Background()
	repo, projectID := newFixture(t)
	device, err := repo.UpsertDevice(ctx, deviceexperience.Device{
		ID: "tablet-01", ProjectID: projectID, Kind: deviceexperience.DeviceTabletPlayer,
		DisplayName: "Tablet 01", ProtocolVersion: deviceexperience.ProtocolVersion1,
		Capabilities: []string{"tablet.media.play"}, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	latency := 150.0
	if _, err := repo.RecordNetworkObservation(ctx, deviceexperience.NetworkObservation{
		TargetKind: "STAGE_DEVICE", TargetID: device.ID, Reachability: deviceexperience.Reachable,
		TransportState: "WEBSOCKET_CONNECTED", LatencyMS: &latency,
	}); err != nil {
		t.Fatal(err)
	}
	base := baseReport{report: preflight.Report{Status: preflight.Pass, ProjectID: projectID, RuntimeSnapshotID: "snapshot-1", Checks: []preflight.Check{}}}
	service := devicepreflight.New(base, repo)
	report, err := service.Evaluate(ctx, projectID, "snapshot-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Warn {
		t.Fatalf("status=%s want WARN checks=%+v", report.Status, report.Checks)
	}
	if !hasCheck(report.Checks, "device."+device.ID, preflight.Warn) || !hasCheck(report.Checks, "network.STAGE_DEVICE."+device.ID, preflight.Warn) {
		t.Fatalf("warning checks missing: %+v", report.Checks)
	}
}

func TestNoPhase4RequirementsPreservesBasePass(t *testing.T) {
	ctx := context.Background()
	repo, projectID := newFixture(t)
	base := baseReport{report: preflight.Report{Status: preflight.Pass, ProjectID: projectID, RuntimeSnapshotID: "snapshot-1", Checks: []preflight.Check{}}}
	report, err := devicepreflight.New(base, repo).Evaluate(ctx, projectID, "snapshot-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Pass || len(report.Checks) != 0 {
		t.Fatalf("report=%+v", report)
	}
}

func newFixture(t *testing.T) (*deviceexperience.Repository, string) {
	t.Helper()
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	stageStore := store.New(h.DB, clock.Real{})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Phase 4 Preflight", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := deviceexperience.NewRepository(h.DB, deviceexperience.WithClock(func() time.Time { return time.Now().UTC() }))
	if err != nil {
		t.Fatal(err)
	}
	return repo, project.ID
}

func hasCheck(checks []preflight.Check, key string, status preflight.Status) bool {
	for _, check := range checks {
		if check.Key == key && check.Status == status {
			return true
		}
	}
	return false
}
