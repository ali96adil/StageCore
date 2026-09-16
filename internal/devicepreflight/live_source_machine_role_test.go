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
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestRequiredLiveSourceMachineRoleUsesCompanionReadinessAndCapabilities(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	stageStore := store.New(h.DB, clock.Fixed{Time: now})
	project, _, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "F-026 Live Preflight", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "visual-render", DisplayName: "Visual Render",
		RequiredCapabilities: []string{deviceexperience.CapabilityVideoSourceOpen}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	companionRuntime, err := stageStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		DisplayName: "Render Mac", Platform: "macos", Architecture: "arm64",
		Capabilities: []string{deviceexperience.CapabilityVideoSourceOpen, deviceexperience.CapabilityVideoSourceRoute},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetCompanionTrustState(ctx, companionRuntime.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	companionRuntime, err = stageStore.UpdateCompanionReport(ctx, companionRuntime.ID, store.CompanionReportParams{
		DisplayName: "Render Mac", Platform: "macos", Architecture: "arm64", Version: "test",
		Capabilities: []string{deviceexperience.CapabilityVideoSourceOpen, deviceexperience.CapabilityVideoSourceRoute},
		Readiness: domain.CompanionReadinessReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.AssignMachineRole(ctx, role.ID, companionRuntime.ID); err != nil {
		t.Fatal(err)
	}
	repo, err := deviceexperience.NewRepository(h.DB, deviceexperience.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	source, err := repo.UpsertLiveSource(ctx, deviceexperience.LiveSource{
		ProjectID: project.ID, Name: "Main Camera", Class: deviceexperience.SourceLocalCamera,
		ExecutionMachineRoleID: role.ID,
		Capabilities: []string{deviceexperience.CapabilityVideoSourceOpen, deviceexperience.CapabilityVideoSourceRoute},
		Config: json.RawMessage(`{"device_ref":"camera-main"}`), Required: true, DesiredEnabled: true,
		Readiness: deviceexperience.ReadinessReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := baseReport{report: preflight.Report{Status: preflight.Pass, ProjectID: project.ID, RuntimeSnapshotID: "snapshot-1"}}
	service := devicepreflight.New(
		base,
		repo,
		devicepreflight.WithCompanionAuthority(stageStore),
		devicepreflight.WithClock(func() time.Time { return now }),
		devicepreflight.WithCompanionHeartbeatTimeout(5*time.Second),
	)

	report, err := service.Evaluate(ctx, project.ID, "snapshot-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Pass {
		t.Fatalf("ready Companion should pass live-source preflight: %+v", report)
	}

	_, err = stageStore.UpdateCompanionReport(ctx, companionRuntime.ID, store.CompanionReportParams{
		DisplayName: "Render Mac", Platform: "macos", Architecture: "arm64", Version: "test",
		Capabilities: []string{deviceexperience.CapabilityVideoSourceOpen},
		Readiness: domain.CompanionReadinessReady,
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err = service.Evaluate(ctx, project.ID, "snapshot-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Block || !hasCheck(report.Checks, "live_video."+source.ID+".capabilities", preflight.Block) {
		t.Fatalf("missing Companion route capability should block required source: %+v", report)
	}
}
