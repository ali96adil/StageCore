package extension

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

type dependencyTestHarness struct {
	ctx       context.Context
	handle    *db.Handle
	repository *software.Repository
	library   *Library
	installer *Installer
}

func newDependencyTestHarness(t *testing.T) *dependencyTestHarness {
	t.Helper()
	ctx := context.Background()
	dataRoot := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	stageStore := store.New(handle.DB, clock.Real{})
	v, err := vault.Open(filepath.Join(dataRoot, "vault"), stageStore)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(v, stageStore, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	library, err := NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	installer, err := NewInstaller(library, filepath.Join(dataRoot, "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	return &dependencyTestHarness{ctx: ctx, handle: handle, repository: repository, library: library, installer: installer}
}

func (h *dependencyTestHarness) register(t *testing.T, extensionID, version string, dependencies []Dependency) Package {
	t.Helper()
	payload := extensionID + "@" + version + " dependency test payload"
	softwarePackage, err := h.repository.ImportPackage(h.ctx, software.ImportParams{
		ProductID: extensionID,
		Version: version,
		Platform: "linux",
		Architecture: "arm64",
		MinAPIVersion: 1,
		MaxAPIVersion: 1,
		OriginalFilename: extensionID + "-" + version,
		SigningStatus: store.SoftwareSigningSigned,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: extensionID,
		Version: version,
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: extensionID, ArIQ: "إضافة " + extensionID},
		Summary: LocalizedText{EN: "Dependency test extension.", ArIQ: "إضافة تجريبية لاختبار الاعتماديات."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Dependencies: dependencies,
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := h.library.Register(h.ctx, softwarePackage.ID, raw, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return registered
}

func TestDependencyPlannerBacktracksOrdersAndGatesRootInstall(t *testing.T) {
	h := newDependencyTestHarness(t)
	beta := h.register(t, "dep.beta", "2.5.0", nil)
	alphaLow := h.register(t, "dep.alpha", "1.0.0", []Dependency{{ExtensionID: "dep.beta", MinVersion: "2.0.0"}})
	_ = h.register(t, "dep.alpha", "2.0.0", []Dependency{{ExtensionID: "dep.beta", MaxVersion: "1.0.0"}})
	optional := h.register(t, "dep.optional", "1.0.0", nil)
	root := h.register(t, "app.root", "1.0.0", []Dependency{
		{ExtensionID: "dep.alpha"},
		{ExtensionID: "dep.beta", MinVersion: "2.0.0"},
		{ExtensionID: "dep.optional", MinVersion: "1.0.0", Optional: true},
	})

	plan, err := h.installer.PlanInstall(h.ctx, root.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != InstallPlanRequiresDependencies || plan.RootAlreadyInstalled {
		t.Fatalf("plan status=%s root_installed=%v", plan.Status, plan.RootAlreadyInstalled)
	}
	if len(plan.Blockers) != 0 {
		t.Fatalf("unexpected blockers=%+v", plan.Blockers)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("steps=%+v", plan.Steps)
	}
	wantIDs := []string{"dep.beta", "dep.alpha", "app.root"}
	wantPackages := []string{beta.PackageID, alphaLow.PackageID, root.PackageID}
	for index, step := range plan.Steps {
		if step.Order != index+1 || step.ExtensionID != wantIDs[index] || step.PackageID != wantPackages[index] {
			t.Fatalf("step[%d]=%+v want extension=%s package=%s", index, step, wantIDs[index], wantPackages[index])
		}
	}
	if len(plan.Warnings) != 1 || plan.Warnings[0].Code != "OPTIONAL_DEPENDENCY_AVAILABLE" || plan.Warnings[0].ExtensionID != "dep.optional" || plan.Warnings[0].CandidateVersion != optional.Manifest.Version {
		t.Fatalf("warnings=%+v", plan.Warnings)
	}

	_, err = h.installer.InstallPlanned(h.ctx, root.PackageID, "owner")
	if !errors.Is(err, ErrDependenciesRequired) {
		t.Fatalf("root install before dependencies err=%v", err)
	}
	var planErr *InstallPlanError
	if !errors.As(err, &planErr) || planErr.Plan.Status != InstallPlanRequiresDependencies {
		t.Fatalf("install did not return dependency plan: %T %v", err, err)
	}

	if _, err := h.installer.InstallPlanned(h.ctx, beta.PackageID, "owner"); err != nil {
		t.Fatalf("install beta: %v", err)
	}
	if _, err := h.installer.InstallPlanned(h.ctx, alphaLow.PackageID, "owner"); err != nil {
		t.Fatalf("install alpha: %v", err)
	}
	installedRoot, err := h.installer.InstallPlanned(h.ctx, root.PackageID, "owner")
	if err != nil {
		t.Fatalf("install root after prerequisites: %v", err)
	}
	if installedRoot.PackageID != root.PackageID {
		t.Fatalf("installed root=%+v", installedRoot)
	}

	ready, err := h.installer.PlanInstall(h.ctx, root.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != InstallPlanReady || !ready.RootAlreadyInstalled || len(ready.Steps) != 0 {
		t.Fatalf("ready plan=%+v", ready)
	}
}

func TestDependencyPlannerRejectsCycle(t *testing.T) {
	h := newDependencyTestHarness(t)
	root := h.register(t, "cycle.root", "1.0.0", []Dependency{{ExtensionID: "cycle.child"}})
	_ = h.register(t, "cycle.child", "1.0.0", []Dependency{{ExtensionID: "cycle.root"}})

	plan, err := h.installer.PlanInstall(h.ctx, root.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != InstallPlanBlocked || len(plan.Blockers) != 1 || plan.Blockers[0].Code != "DEPENDENCY_CYCLE" {
		t.Fatalf("cycle plan=%+v", plan)
	}
	if _, err := h.installer.InstallPlanned(h.ctx, root.PackageID, "owner"); !errors.Is(err, ErrDependencyPlanBlocked) {
		t.Fatalf("cycle install err=%v", err)
	}
}

func TestDependencyPlannerRejectsInstalledVersionConflict(t *testing.T) {
	h := newDependencyTestHarness(t)
	oldDependency := h.register(t, "dep.locked", "1.0.0", nil)
	_ = h.register(t, "dep.locked", "2.0.0", nil)
	root := h.register(t, "app.locked", "1.0.0", []Dependency{{ExtensionID: "dep.locked", MinVersion: "2.0.0"}})

	if _, err := h.installer.InstallPlanned(h.ctx, oldDependency.PackageID, "owner"); err != nil {
		t.Fatal(err)
	}
	plan, err := h.installer.PlanInstall(h.ctx, root.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != InstallPlanBlocked || len(plan.Blockers) != 1 || plan.Blockers[0].Code != "DEPENDENCY_INSTALLED_VERSION_CONFLICT" || plan.Blockers[0].InstalledVersion != "1.0.0" {
		t.Fatalf("installed conflict plan=%+v", plan)
	}
}

func TestSemanticVersionComparisonAndImpossibleRange(t *testing.T) {
	ordered := []string{"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0"}
	for index := 0; index < len(ordered)-1; index++ {
		if compareSemanticVersions(ordered[index], ordered[index+1]) >= 0 {
			t.Fatalf("version order %q !< %q", ordered[index], ordered[index+1])
		}
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: "range.root",
		Version: "1.0.0",
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: "Range Root", ArIQ: "إضافة النطاق"},
		Summary: LocalizedText{EN: "Range validation.", ArIQ: "اختبار التحقق من نطاق الإصدار."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Dependencies: []Dependency{{ExtensionID: "dep.range", MinVersion: "3.0.0", MaxVersion: "2.0.0"}},
	}
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "min_version greater than max_version") {
		t.Fatalf("impossible range err=%v", err)
	}
}
