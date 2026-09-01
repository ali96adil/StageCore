package extension

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/store"
)

func TestUpdateAndRollbackPreserveInstallationIdentityAndResetAuthority(t *testing.T) {
	h := newDependencyTestHarness(t)
	v1 := h.register(t, "example.maintenance", "1.0.0", nil)
	v2 := h.register(t, "example.maintenance", "2.0.0", nil)
	installed, err := h.installer.InstallPlanned(h.ctx, v1.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.handle.DB.ExecContext(h.ctx, `
		INSERT INTO extension_permission_reviews (installation_id, permission, decision, reviewed_by, reviewed_at_us)
		VALUES (?, 'network.udp.send', 'APPROVED', 'owner', 1)`, installed.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.installer.library.store.SetExtensionRuntimeDesiredState(h.ctx, installed.InstallationID, store.ExtensionRuntimeDesiredDisabled, "owner"); err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, v2)
	defer supervisor.Close()

	plan, err := h.installer.PlanUpdate(h.ctx, installed.InstallationID, v2.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != InstallPlanReady || plan.Direction != UpdateDirectionUpdate || len(plan.Steps) != 0 {
		t.Fatalf("update plan=%+v", plan)
	}
	updated, err := supervisor.UpdateInstallation(h.ctx, installed.InstallationID, v2.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Direction != UpdateDirectionUpdate || updated.FromVersion != "1.0.0" || updated.ToVersion != "2.0.0" || updated.InstallationID != installed.InstallationID {
		t.Fatalf("update result=%+v", updated)
	}
	current, err := h.installer.Get(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if current.PackageID != v2.PackageID || current.Version != "2.0.0" || current.InstallationID != installed.InstallationID {
		t.Fatalf("updated installation=%+v", current)
	}
	for _, table := range []string{"extension_permission_reviews", "extension_runtime_lifecycle"} {
		var count int
		if err := h.handle.DB.QueryRowContext(h.ctx, "SELECT COUNT(*) FROM "+table+" WHERE installation_id = ?", installed.InstallationID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows after update", table, count)
		}
	}

	rollbackPlan, err := h.installer.PlanUpdate(h.ctx, installed.InstallationID, v1.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if rollbackPlan.Status != InstallPlanReady || rollbackPlan.Direction != UpdateDirectionRollback {
		t.Fatalf("rollback plan=%+v", rollbackPlan)
	}
	rolledBack, err := supervisor.UpdateInstallation(h.ctx, installed.InstallationID, v1.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Direction != UpdateDirectionRollback || rolledBack.FromVersion != "2.0.0" || rolledBack.ToVersion != "1.0.0" {
		t.Fatalf("rollback result=%+v", rolledBack)
	}
	current, err = h.installer.Get(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if current.PackageID != v1.PackageID || current.Version != "1.0.0" {
		t.Fatalf("rolled-back installation=%+v", current)
	}
}

func TestUpdatePlanBlocksInstalledDependentVersionConflict(t *testing.T) {
	h := newDependencyTestHarness(t)
	depV1 := h.register(t, "dep.core", "1.0.0", nil)
	depV2 := h.register(t, "dep.core", "2.0.0", nil)
	app := h.register(t, "app.root", "1.0.0", []Dependency{{ExtensionID: "dep.core", MaxVersion: "1.5.0"}})
	depInstall, err := h.installer.InstallPlanned(h.ctx, depV1.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.installer.InstallPlanned(h.ctx, app.PackageID, "owner"); err != nil {
		t.Fatal(err)
	}
	plan, err := h.installer.PlanUpdate(h.ctx, depInstall.InstallationID, depV2.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != InstallPlanBlocked || len(plan.Blockers) != 1 || plan.Blockers[0].Code != "INSTALLED_DEPENDENT_VERSION_CONFLICT" || plan.Blockers[0].RequiredBy != "app.root" {
		t.Fatalf("dependent conflict plan=%+v", plan)
	}
}

func TestUpdatePlanRequiresMissingDependenciesBeforeReplacement(t *testing.T) {
	h := newDependencyTestHarness(t)
	v1 := h.register(t, "example.maintenance", "1.0.0", nil)
	dependency := h.register(t, "dep.new", "1.0.0", nil)
	v2 := h.register(t, "example.maintenance", "2.0.0", []Dependency{{ExtensionID: "dep.new", MinVersion: "1.0.0"}})
	installed, err := h.installer.InstallPlanned(h.ctx, v1.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := h.installer.PlanUpdate(h.ctx, installed.InstallationID, v2.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != InstallPlanRequiresDependencies || len(plan.Steps) != 1 || plan.Steps[0].PackageID != dependency.PackageID {
		t.Fatalf("dependency update plan=%+v", plan)
	}
	if _, err := h.installer.InstallPlanned(h.ctx, dependency.PackageID, "owner"); err != nil {
		t.Fatal(err)
	}
	plan, err = h.installer.PlanUpdate(h.ctx, installed.InstallationID, v2.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != InstallPlanReady || len(plan.Steps) != 0 {
		t.Fatalf("ready update plan=%+v", plan)
	}
}

func TestRepairRestoresTamperedPayloadFromImmutableVault(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := h.register(t, "example.repair", "1.0.0", nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(h.installer.installedRoot, filepath.FromSlash(installed.PayloadRelativePath))
	if err := os.Chmod(payloadPath, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payloadPath, []byte("tampered payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	defer supervisor.Close()
	repaired, err := supervisor.RepairInstallation(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !repaired.PayloadRepaired || repaired.AlreadyHealthy {
		t.Fatalf("repair result=%+v", repaired)
	}
	if _, err := h.installer.Get(h.ctx, installed.InstallationID); err != nil {
		t.Fatalf("repaired installation did not verify: %v", err)
	}
	info, err := os.Stat(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != installedPayloadMode {
		t.Fatalf("repaired payload mode=%#o", info.Mode().Perm())
	}
	healthy, err := supervisor.RepairInstallation(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !healthy.AlreadyHealthy || healthy.PayloadRepaired {
		t.Fatalf("healthy repair result=%+v", healthy)
	}
}

func TestMaintenanceRequiresDisabledStoppedRuntime(t *testing.T) {
	h := newDependencyTestHarness(t)
	v1 := h.register(t, "example.maintenance", "1.0.0", nil)
	v2 := h.register(t, "example.maintenance", "2.0.0", nil)
	installed, err := h.installer.InstallPlanned(h.ctx, v1.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.installer.library.store.SetExtensionRuntimeDesiredState(h.ctx, installed.InstallationID, store.ExtensionRuntimeDesiredEnabled, "owner"); err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, v1)
	defer supervisor.Close()
	if _, err := supervisor.UpdateInstallation(h.ctx, installed.InstallationID, v2.PackageID, "owner"); !errors.Is(err, ErrExtensionRuntimeMustBeDisabled) {
		t.Fatalf("enabled update err=%v", err)
	}
	if _, err := supervisor.RepairInstallation(h.ctx, installed.InstallationID, "owner"); !errors.Is(err, ErrExtensionRuntimeMustBeDisabled) {
		t.Fatalf("enabled repair err=%v", err)
	}
}
