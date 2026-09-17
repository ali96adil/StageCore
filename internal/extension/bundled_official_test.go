package extension

import (
	"context"
	"path/filepath"
	"testing"

	tabletcontrolleraddon "github.com/ali96adil/StageCore/extensions/stagecore.tablet-controller"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestBundledTabletControllerBootstrapUsesNormalLibraryAndInstallLifecycle(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	vaultRoot := filepath.Join(dataRoot, "vault")
	installRoot := filepath.Join(dataRoot, "extensions")

	openLibrary := func() (*db.Handle, *Library) {
		t.Helper()
		h, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
		if err != nil {
			t.Fatal(err)
		}
		stageStore := store.New(h.DB, clock.Real{})
		v, err := vault.Open(vaultRoot, stageStore)
		if err != nil {
			_ = h.Close()
			t.Fatal(err)
		}
		repository, err := software.New(v, stageStore, software.CurrentHubAPIVersion)
		if err != nil {
			_ = h.Close()
			t.Fatal(err)
		}
		library, err := NewLibrary(stageStore, repository)
		if err != nil {
			_ = h.Close()
			t.Fatal(err)
		}
		return h, library
	}

	spec := BundledOfficialSpec{
		Manifest: tabletcontrolleraddon.Manifest(), Payload: tabletcontrolleraddon.Payload(),
		Platform: "linux", Architecture: "amd64", OriginalFilename: "stagecore.tablet-controller.addon",
		ReleaseNotes: "Bundled official Tablet Controller operator experience.",
	}

	h, library := openLibrary()
	first, err := library.BootstrapBundledOfficial(ctx, spec, "stagecore-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if !first.Supported || first.AlreadyRegistered {
		t.Fatalf("first bootstrap=%+v", first)
	}
	if first.Package.Manifest.ExtensionID != tabletcontrolleraddon.ExtensionID || first.Package.Manifest.Version != tabletcontrolleraddon.Version {
		t.Fatalf("unexpected bundled package identity: %+v", first.Package.Manifest)
	}
	if first.Package.Manifest.Kind != KindAddon || first.Package.Manifest.Source != SourceOfficial || !first.Package.ProductionReady {
		t.Fatalf("bundled package trust/lifecycle metadata=%+v", first.Package)
	}

	second, err := library.BootstrapBundledOfficial(ctx, spec, "stagecore-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyRegistered || second.Package.PackageID != first.Package.PackageID {
		t.Fatalf("same-process bootstrap changed package identity: first=%+v second=%+v", first, second)
	}
	listed, err := library.List(ctx, tabletcontrolleraddon.ExtensionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("library contains %d Tablet Controller packages, want 1", len(listed))
	}

	installer, err := NewInstaller(library, installRoot)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installer.InstallPlanned(ctx, first.Package.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if installed.ExtensionID != tabletcontrolleraddon.ExtensionID || installed.LifecycleState != store.ExtensionInstallationInstalled {
		t.Fatalf("normal F-015 install result=%+v", installed)
	}

	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, library = openLibrary()
	defer h.Close()
	afterRestart, err := library.BootstrapBundledOfficial(ctx, spec, "stagecore-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if !afterRestart.AlreadyRegistered || afterRestart.Package.PackageID != first.Package.PackageID {
		t.Fatalf("restart bootstrap changed package identity: first=%+v restart=%+v", first, afterRestart)
	}
	listed, err = library.List(ctx, tabletcontrolleraddon.ExtensionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("restart duplicated Tablet Controller package: %+v", listed)
	}
}

func TestBundledOfficialBootstrapSkipsUnsupportedHostWithoutMutation(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	stageStore := store.New(h.DB, clock.Real{})
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
	result, err := library.BootstrapBundledOfficial(ctx, BundledOfficialSpec{
		Manifest: tabletcontrolleraddon.Manifest(), Payload: tabletcontrolleraddon.Payload(),
		Platform: "windows", Architecture: "amd64",
	}, "stagecore-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if result.Supported || result.AlreadyRegistered {
		t.Fatalf("unsupported host bootstrap=%+v", result)
	}
	listed, err := library.List(ctx, tabletcontrolleraddon.ExtensionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("unsupported host mutated library: %+v", listed)
	}
}
