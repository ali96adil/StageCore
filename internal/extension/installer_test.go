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
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestInstallerStagesVerifiesPersistsAndReopens(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	vaultRoot := filepath.Join(dataRoot, "vault")
	installRoot := filepath.Join(dataRoot, "extensions")

	h, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(h.DB, clock.Real{})
	v, err := vault.Open(vaultRoot, stageStore)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(v, stageStore, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	payload := "secure extension installation payload"
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader(payload))
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
	installer, err := NewInstaller(library, installRoot)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installer.Install(ctx, pkg.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if installed.PackageID != pkg.ID || installed.ExtensionID != "example.osc-plugin" || installed.LifecycleState != store.ExtensionInstallationInstalled {
		t.Fatalf("installed=%+v", installed)
	}
	if filepath.IsAbs(installed.PayloadRelativePath) || strings.Contains(installed.PayloadRelativePath, dataRoot) {
		t.Fatalf("payload path leaked absolute storage root: %q", installed.PayloadRelativePath)
	}
	installedPath := filepath.Join(installer.installedRoot, filepath.FromSlash(installed.PayloadRelativePath))
	info, err := os.Lstat(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != installedPayloadMode {
		t.Fatalf("installed payload mode=%v", info.Mode())
	}
	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != payload {
		t.Fatalf("installed payload=%q", content)
	}
	stagingEntries, err := os.ReadDir(installer.stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(stagingEntries) != 0 {
		t.Fatalf("staging not empty after install: %v", stagingEntries)
	}

	idempotent, err := installer.Install(ctx, pkg.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if idempotent.InstallationID != installed.InstallationID || !idempotent.InstalledAt.Equal(installed.InstalledAt) {
		t.Fatalf("idempotent install changed identity: first=%+v second=%+v", installed, idempotent)
	}

	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	stageStore = store.New(h.DB, clock.Real{})
	v, err = vault.Open(vaultRoot, stageStore)
	if err != nil {
		t.Fatal(err)
	}
	repository, err = software.New(v, stageStore, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	library, err = NewLibrary(stageStore, repository)
	if err != nil {
		t.Fatal(err)
	}
	installer, err = NewInstaller(library, installRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := installer.Get(ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.PackageID != installed.PackageID || reopened.ContentSHA256 != installed.ContentSHA256 || reopened.PayloadRelativePath != installed.PayloadRelativePath {
		t.Fatalf("reopened=%+v installed=%+v", reopened, installed)
	}
}

func TestInstallerRejectsSecondVersionAndManagedDirectorySymlink(t *testing.T) {
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
	importAndRegister := func(version, payload string) store.SoftwarePackage {
		pkg, err := repository.ImportPackage(ctx, software.ImportParams{
			ProductID: "example.osc-plugin", Version: version, Platform: "linux", Architecture: "arm64",
			MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin-" + version,
			SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
			ReleaseChannel: store.SoftwareChannelRelease,
		}, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := library.Register(ctx, pkg.ID, installerTestManifest(version), "owner"); err != nil {
			t.Fatal(err)
		}
		return pkg
	}
	first := importAndRegister("1.2.3", "first payload")
	second := importAndRegister("1.2.4", "second payload")
	installer, err := NewInstaller(library, filepath.Join(dataRoot, "extensions"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(ctx, first.ID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(ctx, second.ID, "owner"); !errors.Is(err, ErrDifferentPackageInstalled) {
		t.Fatalf("second version install err=%v", err)
	}

	symlinkRoot := filepath.Join(t.TempDir(), "extensions")
	if err := os.Mkdir(symlinkRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(symlinkRoot, "installed")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewInstaller(library, symlinkRoot); err == nil {
		t.Fatal("installer accepted symlinked managed installed directory")
	}
}

func installerTestManifest(version string) []byte {
	return []byte(`{
		"schema_version":1,
		"extension_id":"example.osc-plugin",
		"version":"` + version + `",
		"kind":"PLUGIN",
		"source":"LOCAL",
		"name":{"en":"Example OSC Plugin","ar-IQ":"إضافة OSC تجريبية"},
		"summary":{"en":"Sends OSC messages.","ar-IQ":"ترسل رسائل OSC إلى أجهزة المسرح."},
		"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},
		"permissions":["network.udp.send"],
		"capabilities":["osc.send"]
	}`)
}
