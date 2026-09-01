package extension

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestOfflineBundleImportRejectsOfficialDowngradesTrustAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	importer, cleanup := newOfflineBundleTestImporter(t, ctx)
	defer cleanup()
	payload := []byte("offline local extension payload")
	localBundle := buildOfflineBundle(t, SourceLocal, payload, payload, false)

	first, err := importer.Import(ctx, bytes.NewReader(localBundle), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if first.TrustedOfficial || first.AlreadyRegistered || first.Package.Manifest.Source != SourceLocal {
		t.Fatalf("first import=%+v", first)
	}
	status, err := importer.library.software.Get(ctx, first.Package.PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if status.ProductionReady || status.Package.SigningStatus != store.SoftwareSigningUnknown || status.Package.ReleaseChannel != store.SoftwareChannelDevelopment {
		t.Fatalf("operator import retained unverified trust metadata: %+v", status)
	}

	second, err := importer.Import(ctx, bytes.NewReader(localBundle), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if !second.AlreadyRegistered || second.Package.PackageID != first.Package.PackageID {
		t.Fatalf("idempotent import changed package identity: first=%+v second=%+v", first, second)
	}

	officialBundle := buildOfflineBundle(t, SourceOfficial, payload, payload, false)
	if _, err := importer.Import(ctx, bytes.NewReader(officialBundle), "owner"); !errors.Is(err, ErrOfflineBundleSource) {
		t.Fatalf("operator import official err=%v", err)
	}
}

func TestOfflineBundleImportRejectsTamperAndUnexpectedEntries(t *testing.T) {
	ctx := context.Background()
	importer, cleanup := newOfflineBundleTestImporter(t, ctx)
	defer cleanup()
	declared := []byte("declared extension payload")
	tampered := []byte("tampered extension payload")
	bundle := buildOfflineBundle(t, SourceLocal, tampered, declared, false)
	if _, err := importer.Import(ctx, bytes.NewReader(bundle), "owner"); !errors.Is(err, ErrOfflineBundleIntegrity) {
		t.Fatalf("tampered import err=%v", err)
	}
	packages, err := importer.library.List(ctx, "example.osc-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 0 {
		t.Fatalf("tampered bundle registered extension packages: %+v", packages)
	}

	extra := buildOfflineBundle(t, SourceLocal, declared, declared, true)
	if _, err := importer.Import(ctx, bytes.NewReader(extra), "owner"); !errors.Is(err, ErrOfflineBundleInvalid) {
		t.Fatalf("extra-entry import err=%v", err)
	}
}

func TestTrustedCatalogImportsOfficialAndRejectsSymlink(t *testing.T) {
	ctx := context.Background()
	importer, cleanup := newOfflineBundleTestImporter(t, ctx)
	defer cleanup()
	root := t.TempDir()
	catalog, err := NewTrustedCatalog(importer, root)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("trusted official extension payload")
	bundle := buildOfflineBundle(t, SourceOfficial, payload, payload, false)
	bundlePath := filepath.Join(root, "example.scext")
	if err := os.WriteFile(bundlePath, bundle, 0o640); err != nil {
		t.Fatal(err)
	}

	first, err := catalog.Sync(ctx, "stagecore-catalog")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Imported) != 1 || first.Imported[0].AlreadyRegistered {
		t.Fatalf("first sync=%+v", first)
	}
	pkg, err := importer.library.Get(ctx, first.Imported[0].PackageID)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Source != SourceOfficial || !pkg.ProductionReady {
		t.Fatalf("trusted catalog package=%+v", pkg)
	}

	second, err := catalog.Sync(ctx, "stagecore-catalog")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Imported) != 1 || !second.Imported[0].AlreadyRegistered || second.Imported[0].PackageID != first.Imported[0].PackageID {
		t.Fatalf("idempotent trusted sync first=%+v second=%+v", first, second)
	}

	if err := os.Symlink(bundlePath, filepath.Join(root, "linked.scext")); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Sync(ctx, "stagecore-catalog"); err == nil {
		t.Fatal("trusted catalog accepted symlink bundle")
	}
}

func newOfflineBundleTestImporter(t *testing.T, ctx context.Context) (*OfflineBundleImporter, func()) {
	t.Helper()
	dataRoot := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Real{})
	v, err := vault.Open(filepath.Join(dataRoot, "vault"), s)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	repository, err := software.New(v, s, software.CurrentHubAPIVersion)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	library, err := NewLibrary(s, repository)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	importer, err := NewOfflineBundleImporter(library)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	return importer, func() { _ = h.Close() }
}

func buildOfflineBundle(t *testing.T, source Source, payload, hashPayload []byte, extra bool) []byte {
	t.Helper()
	sum := sha256.Sum256(hashPayload)
	metadata := OfflineBundleMetadata{
		Format: OfflineBundleFormatV1, ProductID: "example.osc-plugin", Version: "1.2.3",
		Platform: "linux", Architecture: "arm64", MinAPIVersion: 1, MaxAPIVersion: 1,
		OriginalFilename: "example-plugin", SigningStatus: store.SoftwareSigningSigned,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable, ReleaseChannel: store.SoftwareChannelRelease,
		ReleaseNotes: "offline bundle test", PayloadSHA256: hex.EncodeToString(sum[:]), PayloadSizeBytes: int64(len(payload)),
	}
	if len(hashPayload) != len(payload) {
		metadata.PayloadSizeBytes = int64(len(hashPayload))
	}
	metadataRaw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	manifest := bytes.Replace(installerTestManifest("1.2.3"), []byte(`"source":"LOCAL"`), []byte(`"source":"`+string(source)+`"`), 1)
	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)
	write := func(name string, content []byte) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	write(OfflineBundleMetadataName, metadataRaw)
	write(OfflineBundleManifestName, manifest)
	write(OfflineBundlePayloadName, payload)
	if extra {
		write("unexpected.txt", []byte("not allowed"))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
