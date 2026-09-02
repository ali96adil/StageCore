package preflight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companionchannel"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestPreflightExecutionEnvironmentInspectionTargetsBoundCompanion(t *testing.T) {
	fixture := newEnvironmentPreflightFixture(t, environmentPreflightManifest(false), true)
	var requests []companionchannel.InspectionRequest
	service := New(
		fixture.store,
		capability.NewRegistry(),
		healthyMonitor(fixture.root),
		WithConnectionCheck(func(id string) bool { return id == fixture.companion.ID }),
		WithEnvironmentInspection(func(_ context.Context, request companionchannel.InspectionRequest) companionchannel.InspectionResult {
			requests = append(requests, request)
			compatible := true
			return companionchannel.InspectionResult{
				InspectionID: request.InspectionID,
				AdapterKey:   request.Manifest.AdapterKey,
				Status:       companionchannel.InspectionCompleted,
				Observation: &executionenv.Observation{
					OS: "darwin", Architecture: "arm64",
					Application: executionenv.ApplicationObservation{
						Present: true, ObservedVersion: "8.1.0", VersionConstraintSatisfied: &compatible,
					},
				},
			}
		}),
	)

	report, err := service.Evaluate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Pass {
		t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
	}
	if len(report.Roles) != 1 || report.Roles[0].MachineRoleID != fixture.role.ID || !report.Roles[0].Required {
		t.Fatalf("roles=%+v", report.Roles)
	}
	if len(requests) != 1 {
		t.Fatalf("inspection requests=%d want=1", len(requests))
	}
	if requests[0].CompanionID != fixture.companion.ID {
		t.Fatalf("inspection companion=%s want=%s", requests[0].CompanionID, fixture.companion.ID)
	}
	if requests[0].Manifest.EnvironmentKey != fixture.environment.Manifest.EnvironmentKey || requests[0].Manifest.AdapterKey != fixture.environment.Manifest.AdapterKey {
		t.Fatalf("inspection manifest=%+v want=%+v", requests[0].Manifest, fixture.environment.Manifest)
	}
	if requests[0].TimeoutMS != environmentInspectionTimeoutMS {
		t.Fatalf("timeout=%d want=%d", requests[0].TimeoutMS, environmentInspectionTimeoutMS)
	}
	if !hasCheck(report, "environment.video-main.application.present", Pass) || !hasCheck(report, "environment.video-main.application.version", Pass) {
		t.Fatalf("environment PASS checks missing: %+v", report.Checks)
	}
	allowed, reason, err := service.ShowGate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
	if err != nil || !allowed || reason != "" {
		t.Fatalf("SHOW gate allowed=%v reason=%q err=%v", allowed, reason, err)
	}
}

func TestPreflightExecutionEnvironmentReferenceOnlyWarnsWithoutBlockingShow(t *testing.T) {
	fixture := newEnvironmentPreflightFixture(t, environmentPreflightManifest(true), true)
	compatible := true
	service := New(
		fixture.store,
		capability.NewRegistry(),
		healthyMonitor(fixture.root),
		WithConnectionCheck(func(string) bool { return true }),
		WithEnvironmentInspection(func(_ context.Context, request companionchannel.InspectionRequest) companionchannel.InspectionResult {
			return companionchannel.InspectionResult{
				InspectionID: request.InspectionID, AdapterKey: request.Manifest.AdapterKey, Status: companionchannel.InspectionCompleted,
				Observation: &executionenv.Observation{
					OS: "darwin", Architecture: "arm64",
					Application: executionenv.ApplicationObservation{Present: true, ObservedVersion: "8.1.0", VersionConstraintSatisfied: &compatible},
					Assets: []executionenv.AssetObservation{{Key: "workspace", Present: true, Inspectable: false}},
				},
			}
		}),
	)

	report, err := service.Evaluate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Warn || !hasCheck(report, "environment.video-main.asset.workspace", Warn) {
		t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
	}
	allowed, _, err := service.ShowGate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
	if err != nil || !allowed {
		t.Fatalf("reference-only warning should not block SHOW allowed=%v err=%v", allowed, err)
	}
}

func TestPreflightExecutionEnvironmentUnboundFailsClosedWithoutInspection(t *testing.T) {
	fixture := newEnvironmentPreflightFixture(t, environmentPreflightManifest(false), false)
	called := false
	service := New(
		fixture.store,
		capability.NewRegistry(),
		healthyMonitor(fixture.root),
		WithConnectionCheck(func(string) bool { return true }),
		WithEnvironmentInspection(func(_ context.Context, request companionchannel.InspectionRequest) companionchannel.InspectionResult {
			called = true
			return companionchannel.InspectionResult{InspectionID: request.InspectionID, AdapterKey: request.Manifest.AdapterKey, Status: companionchannel.InspectionCompleted}
		}),
	)

	report, err := service.Evaluate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("unbound environment must not trigger inspection")
	}
	if report.Status != Block || !hasCheck(report, "environment.video-main.binding", Block) {
		t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
	}
	allowed, _, err := service.ShowGate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
	if err != nil || allowed {
		t.Fatalf("unbound environment SHOW gate allowed=%v err=%v", allowed, err)
	}
}

func TestPreflightExecutionEnvironmentUnsupportedAndOfflineBlock(t *testing.T) {
	t.Run("unsupported", func(t *testing.T) {
		fixture := newEnvironmentPreflightFixture(t, environmentPreflightManifest(false), true)
		service := New(
			fixture.store,
			capability.NewRegistry(),
			healthyMonitor(fixture.root),
			WithConnectionCheck(func(string) bool { return true }),
			WithEnvironmentInspection(func(_ context.Context, request companionchannel.InspectionRequest) companionchannel.InspectionResult {
				return companionchannel.InspectionResult{
					InspectionID: request.InspectionID, AdapterKey: request.Manifest.AdapterKey,
					Status: companionchannel.InspectionUnsupported, ErrorCode: "INSPECTION_ADAPTER_UNSUPPORTED",
					ResponseSummary: "no read-only inspection provider is registered for adapter_key",
				}
			}),
		)
		report, err := service.Evaluate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
		if err != nil {
			t.Fatal(err)
		}
		if report.Status != Block || !hasCheck(report, "environment.video-main.inspection", Block) {
			t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
		}
	})

	t.Run("offline", func(t *testing.T) {
		fixture := newEnvironmentPreflightFixture(t, environmentPreflightManifest(false), true)
		called := false
		service := New(
			fixture.store,
			capability.NewRegistry(),
			healthyMonitor(fixture.root),
			WithConnectionCheck(func(string) bool { return false }),
			WithEnvironmentInspection(func(_ context.Context, request companionchannel.InspectionRequest) companionchannel.InspectionResult {
				called = true
				return companionchannel.InspectionResult{InspectionID: request.InspectionID, AdapterKey: request.Manifest.AdapterKey, Status: companionchannel.InspectionCompleted}
			}),
		)
		report, err := service.Evaluate(context.Background(), fixture.project.ID, fixture.runtimeSnapshot.ID)
		if err != nil {
			t.Fatal(err)
		}
		if called {
			t.Fatal("offline Companion must not receive environment inspection")
		}
		if report.Status != Block || !hasCheck(report, "environment.video-main.connection", Block) {
			t.Fatalf("status=%s checks=%+v", report.Status, report.Checks)
		}
	})
}

type environmentPreflightFixture struct {
	root            string
	store           *store.Store
	project         domain.Project
	role            domain.MachineRole
	companion       domain.Companion
	environment     store.ExecutionEnvironmentManifest
	runtimeSnapshot domain.RuntimeSnapshot
}

func newEnvironmentPreflightFixture(t *testing.T, manifest executionenv.Manifest, bind bool) environmentPreflightFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	projectStore := store.New(handle.DB, clock.Real{})
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Environment Preflight", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := projectStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-MAIN", DisplayName: "Video Main", Required: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := projectStore.CreateExecutionEnvironmentManifest(ctx, revision.ID, manifest, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if bind {
		roleID := role.ID
		environment, err = projectStore.SetExecutionEnvironmentMachineRole(ctx, environment.ID, &roleID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshotManifest := snapshot.Manifest{
		SchemaVersion: snapshot.ManifestSchemaVersion,
		ProjectID: project.ID,
		RevisionID: revision.ID,
		RevisionNumber: revision.RevisionNumber,
		Cues: []snapshot.Cue{},
	}
	raw, err := json.Marshal(snapshotManifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	runtimeSnapshot, err := projectStore.CreateRuntimeSnapshot(ctx, revision.ID, "owner", hex.EncodeToString(digest[:]), raw)
	if err != nil {
		t.Fatal(err)
	}
	companionState, err := projectStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		CompanionID: "00000000-0000-7000-8000-000000000696",
		DisplayName: "VDMX Mac", Hostname: "vdmx-mac", Platform: "macOS", Architecture: "arm64", Version: "0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetCompanionTrustState(ctx, companionState.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	if _, err := projectStore.AssignMachineRole(ctx, role.ID, companionState.ID); err != nil {
		t.Fatal(err)
	}
	applied := runtimeSnapshot.ID
	if _, err := projectStore.UpdateCompanionReport(ctx, companionState.ID, store.CompanionReportParams{
		DisplayName: "VDMX Mac", Hostname: "vdmx-mac", Platform: "macOS", Architecture: "arm64", Version: "0.1",
		Readiness: domain.CompanionReadinessReady, AppliedRuntimeSnapshotID: &applied,
	}); err != nil {
		t.Fatal(err)
	}
	companionState, err = projectStore.GetCompanion(ctx, companionState.ID)
	if err != nil {
		t.Fatal(err)
	}
	return environmentPreflightFixture{
		root: root, store: projectStore, project: project, role: role,
		companion: companionState, environment: environment, runtimeSnapshot: runtimeSnapshot,
	}
}

func environmentPreflightManifest(referenceOnly bool) executionenv.Manifest {
	manifest := executionenv.Manifest{
		SchemaVersion: executionenv.ManifestSchemaVersion,
		EnvironmentKey: "video-main",
		Name: "Main video workstation",
		AdapterKey: "stagecore.adapter.vdmx",
		Application: executionenv.ApplicationRequirement{
			Key: "vdmx", Name: "VDMX", Vendor: "VIDVOX", VersionConstraint: "8.x-tested",
			Hosts: []executionenv.HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
	}
	if referenceOnly {
		manifest.Assets = []executionenv.AssetRequirement{{
			Key: "workspace", Kind: executionenv.AssetProjectFile, Name: "VDMX workspace",
			CapturePolicy: executionenv.CaptureReferenceOnly, Locator: "/Users/show/Stage.vdmx5",
		}}
	}
	return manifest
}

func hasCheck(report Report, key string, status Status) bool {
	for _, check := range report.Checks {
		if check.Key == key && check.Status == status {
			return true
		}
	}
	return false
}
