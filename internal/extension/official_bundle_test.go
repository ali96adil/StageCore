package extension

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	tabletcontrollerbundle "github.com/ali96adil/StageCore/extensions/stagecore.tablet-controller"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestBootstrapOfficialTabletControllerIsVaultBackedAndRestartIdempotent(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	vaultRoot := filepath.Join(dataRoot, "vault")

	open := func() (*db.Handle, *store.Store, *software.Repository, *Library) {
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
		return h, stageStore, repository, library
	}

	bundle := BundledOfficialPackage{
		Manifest:         tabletcontrollerbundle.ManifestBytes(),
		Payload:          tabletcontrollerbundle.PayloadBytes(),
		Platform:         "linux",
		Architecture:     "amd64",
		OriginalFilename: "stagecore.tablet-controller-0.1.0.addon",
		ReleaseNotes:     "Bundled StageCore Tablet Controller ADDON.",
	}

	h, _, repository, library := open()
	first, err := library.BootstrapOfficial(ctx, bundle, "stagecore:bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if first.Manifest.ExtensionID != tabletcontrollerbundle.ProductID || first.Manifest.Version != tabletcontrollerbundle.Version {
		t.Fatalf("unexpected official package identity: %+v", first)
	}
	if first.Manifest.Kind != KindAddon || first.Manifest.Source != SourceOfficial {
		t.Fatalf("unexpected official package kind/source: %+v", first.Manifest)
	}
	if !first.Compatible || !first.ProductionReady {
		t.Fatalf("official package not ready: %+v", first)
	}

	softwarePackages, err := repository.List(ctx, tabletcontrollerbundle.ProductID, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(softwarePackages) != 1 || softwarePackages[0].Package.ID != first.PackageID {
		t.Fatalf("software packages=%+v first=%+v", softwarePackages, first)
	}
	extensionPackages, err := library.List(ctx, tabletcontrollerbundle.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if len(extensionPackages) != 1 || extensionPackages[0].PackageID != first.PackageID {
		t.Fatalf("extension packages=%+v first=%+v", extensionPackages, first)
	}

	file, status, err := repository.OpenPackage(ctx, first.PackageID)
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
	if !bytes.Equal(content, tabletcontrollerbundle.PayloadBytes()) {
		t.Fatalf("Vault payload differs from bundled payload: %q", content)
	}
	if status.Package.ContentHash != firstPayloadHash(t, tabletcontrollerbundle.PayloadBytes()) {
		t.Fatalf("unexpected immutable payload hash: %s", status.Package.ContentHash)
	}

	second, err := library.BootstrapOfficial(ctx, bundle, "stagecore:bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if second.PackageID != first.PackageID || !second.RegisteredAt.Equal(first.RegisteredAt) {
		t.Fatalf("same-process bootstrap changed identity: first=%+v second=%+v", first, second)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	h, _, repository, library = open()
	defer h.Close()
	restarted, err := library.BootstrapOfficial(ctx, bundle, "stagecore:bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	if restarted.PackageID != first.PackageID || !restarted.RegisteredAt.Equal(first.RegisteredAt) {
		t.Fatalf("restart bootstrap changed identity: first=%+v restarted=%+v", first, restarted)
	}
	softwarePackages, err = repository.List(ctx, tabletcontrollerbundle.ProductID, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	extensionPackages, err = library.List(ctx, tabletcontrollerbundle.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if len(softwarePackages) != 1 || len(extensionPackages) != 1 {
		t.Fatalf("restart bootstrap duplicated records: software=%d extension=%d", len(softwarePackages), len(extensionPackages))
	}
}

func TestBootstrapOfficialRejectsConflictingSameVersionPayload(t *testing.T) {
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

	if _, err := repository.ImportPackage(ctx, software.ImportParams{
		ProductID:          tabletcontrollerbundle.ProductID,
		Version:            tabletcontrollerbundle.Version,
		Platform:           "linux",
		Architecture:       "amd64",
		MinAPIVersion:      1,
		MaxAPIVersion:      1,
		OriginalFilename:   "conflicting.addon",
		SigningStatus:      store.SoftwareSigningSigned,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel:     store.SoftwareChannelRelease,
	}, bytes.NewReader([]byte("different immutable payload"))); err != nil {
		t.Fatal(err)
	}

	_, err = library.BootstrapOfficial(ctx, BundledOfficialPackage{
		Manifest:     tabletcontrollerbundle.ManifestBytes(),
		Payload:      tabletcontrollerbundle.PayloadBytes(),
		Platform:     "linux",
		Architecture: "amd64",
	}, "stagecore:bootstrap")
	if !errors.Is(err, ErrOfficialBundleConflict) {
		t.Fatalf("conflicting bundle err=%v", err)
	}
}

func firstPayloadHash(t *testing.T, payload []byte) string {
	t.Helper()
	objectHash := sha256Hex(payload)
	if len(objectHash) != 64 {
		t.Fatalf("unexpected SHA-256 length: %q", objectHash)
	}
	return objectHash
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
