package extension

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestUninstallBlocksRequiredDependencyUntilDependentsAreRemoved(t *testing.T) {
	h := newDependencyTestHarness(t)
	dependency := h.register(t, "dep.core", "1.0.0", nil)
	dependent := h.register(t, "app.root", "1.0.0", []Dependency{{ExtensionID: "dep.core", MinVersion: "1.0.0"}})

	dependencyInstall, err := h.installer.InstallPlanned(h.ctx, dependency.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	dependentInstall, err := h.installer.InstallPlanned(h.ctx, dependent.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.installer.uninstallDisabled(h.ctx, dependencyInstall.InstallationID)
	if !errors.Is(err, ErrExtensionRequiredByInstalled) {
		t.Fatalf("dependency uninstall err=%v want required-by-installed", err)
	}
	var dependencyErr *UninstallDependencyError
	if !errors.As(err, &dependencyErr) || len(dependencyErr.Blockers) != 1 {
		t.Fatalf("dependency blockers=%+v err=%v", dependencyErr, err)
	}
	blocker := dependencyErr.Blockers[0]
	if blocker.ExtensionID != "dep.core" || blocker.RequiredBy != "app.root" || blocker.RequiredByVersion != "1.0.0" || blocker.MinVersion != "1.0.0" {
		t.Fatalf("dependency blocker=%+v", blocker)
	}
	if _, err := h.installer.library.store.GetExtensionInstallation(h.ctx, dependencyInstall.InstallationID); err != nil {
		t.Fatalf("blocked uninstall removed dependency record: %v", err)
	}

	if _, err := h.installer.uninstallDisabled(h.ctx, dependentInstall.InstallationID); err != nil {
		t.Fatalf("uninstall dependent: %v", err)
	}
	result, err := h.installer.uninstallDisabled(h.ctx, dependencyInstall.InstallationID)
	if err != nil {
		t.Fatalf("uninstall dependency after dependent: %v", err)
	}
	if !result.PayloadRemoved || result.ExtensionID != "dep.core" {
		t.Fatalf("uninstall result=%+v", result)
	}
}

func TestRuntimeSupervisorUninstallRequiresDisabledStoppedAndCleansDurableState(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	payloadPath := filepath.Join(h.installer.installedRoot, filepath.FromSlash(installed.PayloadRelativePath))

	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	defer supervisor.Close()
	if _, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.Uninstall(h.ctx, installed.InstallationID, "owner"); !errors.Is(err, ErrExtensionRuntimeMustBeDisabled) {
		t.Fatalf("enabled uninstall err=%v want runtime-must-be-disabled", err)
	}
	if _, err := h.installer.library.store.GetExtensionInstallation(h.ctx, installed.InstallationID); err != nil {
		t.Fatalf("blocked enabled uninstall removed installation: %v", err)
	}

	if _, err := supervisor.Disable(h.ctx, installed.InstallationID, "owner"); err != nil {
		t.Fatal(err)
	}
	result, err := supervisor.Uninstall(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !result.PayloadRemoved || result.CleanupWarning != "" || result.InstallationID != installed.InstallationID {
		t.Fatalf("uninstall result=%+v", result)
	}
	if _, err := os.Lstat(payloadPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("payload still present after uninstall: %v", err)
	}
	if _, err := h.installer.library.store.GetExtensionInstallation(h.ctx, installed.InstallationID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("installation record after uninstall err=%v", err)
	}
	if _, err := supervisor.Status(h.ctx, installed.InstallationID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("runtime status after uninstall err=%v", err)
	}

	for _, table := range []string{"extension_permission_reviews", "extension_runtime_lifecycle", "extension_installations"} {
		var count int
		if err := h.handle.DB.QueryRowContext(h.ctx, "SELECT COUNT(*) FROM "+table+" WHERE installation_id = ?", installed.InstallationID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows after uninstall", table, count)
		}
	}
}

func TestUninstallRemovesPermissionReviewsButRetainsPackageMetadataForReinstall(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, []string{"network.udp.send"})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.send", PermissionDecisionApproved, "owner"); err != nil {
		t.Fatal(err)
	}

	if _, err := h.installer.uninstallDisabled(h.ctx, installed.InstallationID); err != nil {
		t.Fatal(err)
	}
	var reviewCount int
	if err := h.handle.DB.QueryRowContext(h.ctx, `SELECT COUNT(*) FROM extension_permission_reviews WHERE installation_id = ?`, installed.InstallationID).Scan(&reviewCount); err != nil {
		t.Fatal(err)
	}
	if reviewCount != 0 {
		t.Fatalf("permission reviews retained after uninstall: %d", reviewCount)
	}
	if _, err := h.library.Get(h.ctx, pkg.PackageID); err != nil {
		t.Fatalf("uninstall removed immutable package metadata: %v", err)
	}
	reinstalled, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatalf("reinstall retained package: %v", err)
	}
	if reinstalled.InstallationID == installed.InstallationID {
		t.Fatalf("reinstall reused deleted installation identity: %s", reinstalled.InstallationID)
	}
}
