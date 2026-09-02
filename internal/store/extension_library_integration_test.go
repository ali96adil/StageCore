package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestExtensionLibraryPersistsVerifiedPackageAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	vaultRoot := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Fixed{Time: fixedTime})
	v, err := vault.Open(vaultRoot, s)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(v, s, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("verified extension payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(s, repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := validLocalExtensionManifest()
	registered, err := library.Register(ctx, pkg.ID, manifest, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if registered.PackageID != pkg.ID || registered.Manifest.ExtensionID != pkg.ProductID || registered.ManifestSHA256 == "" {
		t.Fatalf("registered=%+v", registered)
	}
	version, err := db.SchemaVersion(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	if version != 20 {
		t.Fatalf("schema version=%d want=20", version)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	h, err = db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s = store.New(h.DB, clock.Real{})
	v, err = vault.Open(vaultRoot, s)
	if err != nil {
		t.Fatal(err)
	}
	repository, err = software.New(v, s, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	library, err = extension.NewLibrary(s, repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := library.Get(ctx, pkg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ManifestSHA256 != registered.ManifestSHA256 || loaded.Manifest.Name.ArIQ == "" || !loaded.Compatible {
		t.Fatalf("reloaded=%+v", loaded)
	}
}

func TestExtensionLibraryRejectsSelfAssertedOfficialAndShowMutation(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	v, err := vault.Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := software.New(v, s, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, strings.NewReader("extension payload"))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(s, repository)
	if err != nil {
		t.Fatal(err)
	}
	official := strings.Replace(string(validLocalExtensionManifest()), "\"source\":\"LOCAL\"", "\"source\":\"OFFICIAL\"", 1)
	if official == string(validLocalExtensionManifest()) {
		t.Fatal("test setup did not set OFFICIAL source")
	}
	if _, err := library.Register(ctx, pkg.ID, []byte(official), "owner"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("self-asserted official err=%v", err)
	}
}
