package preflight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestPreflightReadyMismatchOfflineAndMediaTruth(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	projectStore := store.New(handle.DB, clock.Real{})
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Preflight", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := projectStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "VIDEO-MAIN", DisplayName: "Video Main", RequiredCapabilities: []string{"osc.send"},
		RequiredConfigHash: "cfg-1", Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	roleConfig, _ := json.Marshal(map[string]string{"machine_role_id": role.ID})
	manifest := snapshot.Manifest{
		SchemaVersion: snapshot.ManifestSchemaVersion, ProjectID: project.ID, RevisionID: revision.ID, RevisionNumber: revision.RevisionNumber,
		Targets: []snapshot.Target{{TargetRef: role.RoleKey, LogicalType: companion.MachineRoleLogicalType, Configuration: roleConfig}},
		Cues: []snapshot.Cue{{
			ID: "00000000-0000-7000-8000-000000000601", DisplayLabel: "1", Name: "Video", OrderIndex: 1,
			CueType: "STANDARD", Criticality: "NORMAL", Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
			Actions: []snapshot.Action{{
				ID: "00000000-0000-7000-8000-000000000602", OrderIndex: 0, ExecutionMode: "SEQUENTIAL",
				TargetRef: role.RoleKey, CapabilityKey: "osc.send", Parameters: json.RawMessage(`{}`),
				TimeoutPolicy: json.RawMessage(`{}`), ErrorPolicy: json.RawMessage(`{}`), PriorityClass: "P1", Enabled: true,
			}},
		}},
		RequiredMedia: []snapshot.RequiredMedia{{
			MachineRoleID: role.ID, RoleKey: role.RoleKey,
			MediaAssetID: "00000000-0000-7000-8000-000000000603",
			ContentVersionID: "00000000-0000-7000-8000-000000000604",
			ChecksumAlgorithm: "SHA256", ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SizeBytes: 4096, Required: true,
		}},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	runtimeSnapshot, err := projectStore.CreateRuntimeSnapshot(ctx, revision.ID, "owner", hex.EncodeToString(digest[:]), raw)
	if err != nil {
		t.Fatal(err)
	}

	companionState, err := projectStore.RegisterCompanion(ctx, store.RegisterCompanionParams{
		CompanionID: "00000000-0000-7000-8000-000000000605", DisplayName: "Mac A",
		Hostname: "mac-a", Platform: "macOS", Architecture: "arm64", Version: "0.1", Capabilities: []string{"osc.send"},
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
		DisplayName: "Mac A", Hostname: "mac-a", Platform: "macOS", Architecture: "arm64", Version: "0.1",
		Capabilities: []string{"osc.send"}, Readiness: domain.CompanionReadinessReady,
		AppliedRuntimeSnapshotID: &applied, ConfigHash: "cfg-1",
	}); err != nil {
		t.Fatal(err)
	}

	registry := capability.NewRegistry()
	if err := registry.RegisterTargetType(companion.MachineRoleLogicalType, capability.ExecutorFunc(func(context.Context, capability.Request) capability.Result {
		return capability.Result{Result: domain.ExecutionCompleted}
	})); err != nil {
		t.Fatal(err)
	}
	connected := true
	service := New(projectStore, registry, healthyMonitor(root), WithConnectionCheck(func(string) bool { return connected }))
	report, err := service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != Pass || len(report.Roles) != 1 || report.Roles[0].RoleState != domain.RoleReady || len(report.Media) != 1 || report.Media[0].Status != Pass {
		t.Fatalf("ready Preflight=%+v roles=%+v media=%+v", report.Status, report.Roles, report.Media)
	}
	allowed, _, err := service.ShowGate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil || !allowed {
		t.Fatalf("ready SHOW gate allowed=%v err=%v", allowed, err)
	}

	connected = false
	offline, err := service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if offline.Status != Block || offline.Roles[0].RoleState != domain.RoleOffline || offline.Media[0].Status != Block {
		t.Fatalf("offline Preflight=%+v roles=%+v media=%+v", offline.Status, offline.Roles, offline.Media)
	}

	connected = true
	wrong := "00000000-0000-7000-8000-000000000699"
	if _, err := projectStore.UpdateCompanionReport(ctx, companionState.ID, store.CompanionReportParams{
		DisplayName: "Mac A", Hostname: "mac-a", Platform: "macOS", Architecture: "arm64", Version: "0.1",
		Capabilities: []string{"osc.send"}, Readiness: domain.CompanionReadinessMismatch,
		AppliedRuntimeSnapshotID: &wrong, ConfigHash: "cfg-1",
	}); err != nil {
		t.Fatal(err)
	}
	mismatch, err := service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mismatch.Status != Block || mismatch.Roles[0].RoleState != domain.RoleMismatch || mismatch.Media[0].Status != Block {
		t.Fatalf("mismatch Preflight=%+v roles=%+v media=%+v", mismatch.Status, mismatch.Roles, mismatch.Media)
	}
}

func TestPreflightStorageWarningCriticalAndMissingSnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	projectStore := store.New(handle.DB, clock.Real{})
	project, revision, err := projectStore.CreateProject(ctx, store.CreateProjectParams{Name: "Storage Preflight", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()

	missing := New(projectStore, registry, healthyMonitor(root))
	missingReport, err := missing.Evaluate(ctx, project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if missingReport.Status != Block {
		t.Fatalf("missing Snapshot status=%s, want BLOCK", missingReport.Status)
	}

	if err := projectStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	manifest := snapshot.Manifest{SchemaVersion: snapshot.ManifestSchemaVersion, ProjectID: project.ID, RevisionID: revision.ID, RevisionNumber: revision.RevisionNumber, Cues: []snapshot.Cue{}}
	raw, _ := json.Marshal(manifest)
	digest := sha256.Sum256(raw)
	runtimeSnapshot, err := projectStore.CreateRuntimeSnapshot(ctx, revision.ID, "owner", hex.EncodeToString(digest[:]), raw)
	if err != nil {
		t.Fatal(err)
	}

	warningMonitor := monitorWithFree(root, 10_000, 1_000, 500, 15)
	warningReport, err := New(projectStore, registry, warningMonitor).Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if warningReport.Status != Warn || warningReport.Storage.Status != Warn {
		t.Fatalf("warning Preflight=%s storage=%+v", warningReport.Status, warningReport.Storage)
	}
	allowed, _, err := New(projectStore, registry, warningMonitor).ShowGate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil || !allowed {
		t.Fatalf("storage warning should not block SHOW: allowed=%v err=%v", allowed, err)
	}

	criticalMonitor := monitorWithFree(root, 10_000, 400, 500, 15)
	criticalReport, err := New(projectStore, registry, criticalMonitor).Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if criticalReport.Status != Block || criticalReport.Storage.Status != Block {
		t.Fatalf("critical Preflight=%s storage=%+v", criticalReport.Status, criticalReport.Storage)
	}
}

func healthyMonitor(root string) *storagehealth.Monitor {
	return monitorWithFree(root, 10_000, 5_000, 500, 15)
}

func monitorWithFree(root string, total, free, reserve uint64, warning float64) *storagehealth.Monitor {
	policy := storagehealth.NewPolicyWithProbe(int64(reserve), warning, func(string) (storagehealth.Filesystem, error) {
		return storagehealth.Filesystem{TotalBytes: total, FreeBytes: free}, nil
	})
	return storagehealth.NewMonitor(policy, root, root)
}
