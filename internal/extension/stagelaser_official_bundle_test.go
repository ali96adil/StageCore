package extension

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	stagelasercontrollerbundle "github.com/ali96adil/StageCore/extensions/stagecore.stagelaser-controller"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestBootstrapOfficialStageLaserControllerIsNonExecutableV2Addon(t *testing.T) {
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

	bundle := BundledOfficialPackage{
		Manifest:         stagelasercontrollerbundle.ManifestBytes(),
		Payload:          stagelasercontrollerbundle.PayloadBytes(),
		Platform:         "linux",
		Architecture:     "amd64",
		OriginalFilename: stagelasercontrollerbundle.ProductID + "-" + stagelasercontrollerbundle.Version + ".addon",
		ReleaseNotes:     "Bundled StageCore StageLaser Controller ADDON.",
	}
	pkg, err := library.BootstrapOfficial(ctx, bundle, "stagecore:bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.ExtensionID != stagelasercontrollerbundle.ProductID ||
		pkg.Manifest.Version != stagelasercontrollerbundle.Version ||
		pkg.Manifest.Kind != KindAddon ||
		pkg.Manifest.Source != SourceOfficial ||
		!pkg.Compatible || !pkg.ProductionReady {
		t.Fatalf("unexpected StageLaser package=%+v", pkg)
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
	if !bytes.Equal(content, stagelasercontrollerbundle.PayloadBytes()) {
		t.Fatalf("Vault payload differs: %q", content)
	}
	for _, required := range [][]byte{
		[]byte(`"runtime_process": false`),
		[]byte(`"transport": "STAGE_DEVICE_V2"`),
		[]byte(`"profile_id": "stagecore.esp32-stagelaser"`),
		[]byte(`"control_contract": "stagecore.stagelaser/1"`),
		[]byte(`"managed_safe_off": true`),
		[]byte(`"raw_toggle_exposed": false`),
	} {
		if !bytes.Contains(content, required) {
			t.Fatalf("StageLaser ADDON payload missing %s: %s", required, content)
		}
	}
	if bytes.Contains(content, []byte(`"runtime_process": true`)) ||
		bytes.Contains(content, []byte(`"raw_toggle_exposed": true`)) {
		t.Fatalf("StageLaser Controller ADDON must stay non-executable and toggle-free: %s", content)
	}

	second, err := library.BootstrapOfficial(ctx, bundle, "stagecore:bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if second.PackageID != pkg.PackageID {
		t.Fatalf("StageLaser ADDON bootstrap is not idempotent: first=%s second=%s", pkg.PackageID, second.PackageID)
	}
}
