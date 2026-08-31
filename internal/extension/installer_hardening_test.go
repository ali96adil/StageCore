package extension

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestInstallerHonorsRuntimeReserveBeforePayloadCopy(t *testing.T) {
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
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("payload larger than remaining non-reserved capacity"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Register(ctx, pkg.ID, installerTestManifest("1.2.3"), "owner"); err != nil {
		t.Fatal(err)
	}
	policy := storagehealth.NewPolicyWithProbe(100, 15, func(string) (storagehealth.Filesystem, error) {
		return storagehealth.Filesystem{TotalBytes: 1000, FreeBytes: 110}, nil
	})
	installRoot := filepath.Join(dataRoot, "extensions")
	installer, err := NewInstaller(library, installRoot, WithInstallerCapacityPolicy(policy))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(ctx, pkg.ID, "owner"); !errors.Is(err, storagehealth.ErrRuntimeReserve) {
		t.Fatalf("install reserve err=%v", err)
	}
	installations, err := stageStore.ListExtensionInstallations(ctx, "example.osc-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if len(installations) != 0 {
		t.Fatalf("reserve-blocked install persisted state: %+v", installations)
	}
	staging, err := os.ReadDir(installer.stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(staging) != 0 {
		t.Fatalf("reserve-blocked install wrote staging payload: %v", staging)
	}
}

func TestInstallerRejectsSymlinkSubstitutionAfterInstall(t *testing.T) {
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
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("symlink substitution payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.Register(ctx, pkg.ID, installerTestManifest("1.2.3"), "owner"); err != nil {
		t.Fatal(err)
	}
	installer, err := NewInstaller(library, filepath.Join(dataRoot, "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installer.Install(ctx, pkg.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}

	extensionDir := filepath.Join(installer.installedRoot, "example.osc-plugin")
	realDir := filepath.Join(installer.installedRoot, "example.osc-plugin-real")
	if err := os.Rename(extensionDir, realDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, extensionDir); err != nil {
		t.Skipf("symlink substitution test unavailable: %v", err)
	}
	if _, err := installer.Get(ctx, installed.InstallationID); !errors.Is(err, ErrInstalledPayloadIntegrity) {
		t.Fatalf("symlink-substituted installed payload err=%v", err)
	}
}
