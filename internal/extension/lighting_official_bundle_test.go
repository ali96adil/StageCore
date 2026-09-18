package extension

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	lightingcontrollerbundle "github.com/ali96adil/StageCore/extensions/stagecore.lighting-controller"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestBootstrapOfficialLightingControllerIsNonExecutableVaultBackedAddon(t *testing.T) {
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

	pkg, err := library.BootstrapOfficial(ctx, BundledOfficialPackage{
		Manifest:         lightingcontrollerbundle.ManifestBytes(),
		Payload:          lightingcontrollerbundle.PayloadBytes(),
		Platform:         "linux",
		Architecture:     "amd64",
		OriginalFilename: lightingcontrollerbundle.ProductID + "-" + lightingcontrollerbundle.Version + ".addon",
		ReleaseNotes:     "Bundled StageCore Lighting Controller ADDON.",
	}, "stagecore:bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.ExtensionID != lightingcontrollerbundle.ProductID ||
		pkg.Manifest.Version != lightingcontrollerbundle.Version ||
		pkg.Manifest.Kind != KindAddon ||
		pkg.Manifest.Source != SourceOfficial ||
		!pkg.Compatible || !pkg.ProductionReady {
		t.Fatalf("unexpected lighting package=%+v", pkg)
	}

	file, _, err := repository.OpenPackage(ctx, pkg.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if !bytes.Equal(content, lightingcontrollerbundle.PayloadBytes()) {
		t.Fatalf("Vault payload differs: %q", content)
	}
	if bytes.Contains(content, []byte(`"runtime_process": true`)) {
		t.Fatalf("Lighting Controller ADDON must remain non-executable: %s", content)
	}
	if !bytes.Contains(content, []byte(`"runtime_process": false`)) ||
		!bytes.Contains(content, []byte(`"transport": "STAGE_DEVICE_V1"`)) {
		t.Fatalf("unexpected Lighting Controller payload: %s", content)
	}

	second, err := library.BootstrapOfficial(ctx, BundledOfficialPackage{
		Manifest: lightingcontrollerbundle.ManifestBytes(), Payload: lightingcontrollerbundle.PayloadBytes(),
		Platform: "linux", Architecture: "amd64",
	}, "stagecore:bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if second.PackageID != pkg.PackageID {
		t.Fatalf("bootstrap is not idempotent: first=%s second=%s", pkg.PackageID, second.PackageID)
	}
}
