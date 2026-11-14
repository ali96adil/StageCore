package showcapsule

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/extension"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

func TestSelfContainedMaterializePreservesExecutionEnvironmentAndExtensionAssets(t *testing.T) {
	ctx := context.Background()
	source := newEmptyCapsuleService(t)
	defer source.db.Close()

	project, revision, err := source.store.CreateProject(ctx, store.CreateProjectParams{
		Name: "Capsule Asset Round Trip", CreatedBy: "capsule-test", ChangeNote: "portable asset qualification",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.store.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "OSC-PLUGIN", LogicalType: "PLUGIN", TargetRef: "OSC-PLUGIN",
		ProjectConfig: []byte(`{"host":"127.0.0.1","port":9000}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.store.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Portable OSC Cue", OrderIndex: 1, Enabled: true,
	}, []domain.Action{{OrderIndex: 1, TargetRef: "OSC-PLUGIN", CapabilityKey: "osc.send", Enabled: true}}); err != nil {
		t.Fatal(err)
	}

	environmentAsset := []byte("isolated execution environment project file")
	environmentAssetHash := importCapsuleTestObject(t, ctx, source.vault, environmentAsset)
	environmentAssetSize := int64(len(environmentAsset))
	environmentManifest, err := source.store.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionenv.Manifest{
		SchemaVersion: executionenv.ManifestSchemaVersion, EnvironmentKey: "osc-workstation", Name: "OSC Workstation",
		AdapterKey: "stagecore.adapter.test", Application: executionenv.ApplicationRequirement{
			Key: "osc-host", Name: "OSC Host", VersionConstraint: "1.x",
			Hosts: []executionenv.HostRequirement{{OS: "linux", Architecture: "arm64"}},
		},
		Assets: []executionenv.AssetRequirement{{
			Key: "project-file", Kind: executionenv.AssetProjectFile, Name: "OSC project",
			CapturePolicy: executionenv.CaptureContentBound, ContentHash: environmentAssetHash, SizeBytes: &environmentAssetSize,
		}},
	}, "capsule-test")
	if err != nil {
		t.Fatal(err)
	}

	snapshotAsset := []byte("isolated execution environment captured state")
	snapshotAssetHash := importCapsuleTestObject(t, ctx, source.vault, snapshotAsset)
	snapshotAssetSize := int64(len(snapshotAsset))
	environmentSnapshot, err := source.store.CreateExecutionEnvironmentSnapshot(ctx, environmentManifest.ID, executionenv.Snapshot{
		SchemaVersion: executionenv.SnapshotSchemaVersion, EnvironmentKey: environmentManifest.Manifest.EnvironmentKey,
		AdapterKey: environmentManifest.Manifest.AdapterKey, SourceManifestSHA256: environmentManifest.ContentSHA256,
		CaptureStatus: executionenv.SnapshotComplete,
		Items: []executionenv.SnapshotItem{{
			Key: "published-state", Name: "Published state", Kind: executionenv.SnapshotConfig,
			Provenance: executionenv.ProvenanceApplicationExport, Capture: executionenv.ItemCaptured,
			Portability: executionenv.SnapshotContentBound, ContentHash: snapshotAssetHash, SizeBytes: &snapshotAssetSize,
		}},
	}, "capsule-test")
	if err != nil {
		t.Fatal(err)
	}

	softwareRepository, err := software.New(source.vault, source.store, software.CurrentHubAPIVersion)
	if err != nil {
		t.Fatal(err)
	}
	extensionPayload := []byte("unexecuted OSC extension package bytes")
	extensionPackage, err := softwareRepository.ImportPackage(ctx, software.ImportParams{
		ProductID: "example.osc-plugin", Version: "1.2.3", Platform: "linux", Architecture: "arm64",
		MinAPIVersion: 1, MaxAPIVersion: 1, OriginalFilename: "example-plugin",
		SigningStatus: store.SoftwareSigningSigned, NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader(extensionPayload))
	if err != nil {
		t.Fatal(err)
	}
	library, err := extension.NewLibrary(source.store, softwareRepository)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schema_version":1,"extension_id":"example.osc-plugin","version":"1.2.3","kind":"PLUGIN","source":"LOCAL","name":{"en":"Example OSC Plugin","ar-IQ":"إضافة OSC تجريبية"},"summary":{"en":"Test only","ar-IQ":"للاختبار فقط"},"compatibility":{"api_min":1,"api_max":1,"platforms":["linux"],"architectures":["arm64"]},"permissions":["network.udp.send"],"capabilities":["osc.send"]}`)
	if _, err := library.Register(ctx, extensionPackage.ID, manifest, "capsule-test"); err != nil {
		t.Fatal(err)
	}
	installer, err := extension.NewInstaller(library, filepath.Join(t.TempDir(), "isolated-extension-install"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installer.Install(ctx, extensionPackage.ID, "capsule-test"); err != nil {
		t.Fatal(err)
	}

	if err := source.store.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(source.store).Create(ctx, revision.ID, "capsule-test")
	if err != nil {
		t.Fatal(err)
	}
	exported, err := source.service.Export(ctx, project.ID, t.TempDir(), BuildOptions{
		RuntimeSnapshotID: runtimeSnapshot.ID, Mode: ExportSelfContained,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(exported.Manifest.ExecutionEnvironments); got != 1 {
		t.Fatalf("exported execution environments=%d, want 1", got)
	}
	if record := exported.Manifest.ExecutionEnvironments[0]; record.ManifestID != environmentManifest.ID || record.LatestSnapshot == nil ||
		record.LatestSnapshot.SnapshotID != environmentSnapshot.ID || record.Manifest.Assets[0].ContentHash != environmentAssetHash ||
		record.LatestSnapshot.Snapshot.Items[0].ContentHash != snapshotAssetHash {
		t.Fatalf("exported execution environment assets were not preserved: %+v", record)
	}
	if got := len(exported.Manifest.Extensions); got != 1 {
		t.Fatalf("exported extensions=%d, want 1", got)
	}
	if record := exported.Manifest.Extensions[0]; record.ExtensionID != "example.osc-plugin" || record.Reason != "CAPABILITY_USE" ||
		!record.ContentIncluded || record.Software.ContentSHA256 != extensionPackage.ContentHash {
		t.Fatalf("exported extension package was not selected and included: %+v", record)
	}
	for _, expected := range []string{environmentAssetHash, snapshotAssetHash, extensionPackage.ContentHash} {
		if !capsuleHasIncludedObject(exported.Manifest, expected) {
			t.Errorf("self-contained capsule omitted required object %s", expected)
		}
	}

	destination := newEmptyCapsuleService(t)
	defer destination.db.Close()
	plan, err := destination.service.PlanImport(ctx, exported.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.MaterializationReady {
		t.Fatalf("materialization unexpectedly blocked: %+v", plan.Checks)
	}
	result, err := destination.service.Materialize(ctx, exported.Path, MaterializeOptions{ImportedBy: "capsule-test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ImportedExtensions) != 1 || result.ImportedExtensions[0] != "example.osc-plugin" {
		t.Fatalf("materialized extension identities=%v", result.ImportedExtensions)
	}

	importedEnvironments, err := destination.store.ListExecutionEnvironmentManifests(ctx, revision.ID)
	if err != nil || len(importedEnvironments) != 1 {
		t.Fatalf("imported environments=%+v err=%v", importedEnvironments, err)
	}
	importedEnvironment := importedEnvironments[0]
	if importedEnvironment.ID != environmentManifest.ID || importedEnvironment.ContentSHA256 != environmentManifest.ContentSHA256 ||
		len(importedEnvironment.Manifest.Assets) != 1 || importedEnvironment.Manifest.Assets[0].ContentHash != environmentAssetHash {
		t.Fatalf("imported environment manifest changed: %+v", importedEnvironment)
	}
	importedSnapshot, err := destination.store.GetLatestExecutionEnvironmentSnapshot(ctx, importedEnvironment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if importedSnapshot.ID != environmentSnapshot.ID || importedSnapshot.ContentSHA256 != environmentSnapshot.ContentSHA256 ||
		len(importedSnapshot.Snapshot.Items) != 1 || importedSnapshot.Snapshot.Items[0].ContentHash != snapshotAssetHash {
		t.Fatalf("imported environment snapshot changed: %+v", importedSnapshot)
	}

	importedExtension, err := destination.store.GetExtensionPackage(ctx, extensionPackage.ID)
	if err != nil {
		t.Fatal(err)
	}
	importedSoftware, err := destination.store.GetSoftwarePackage(ctx, importedExtension.PackageID)
	if err != nil || importedSoftware.ContentHash != extensionPackage.ContentHash || importedSoftware.SizeBytes != int64(len(extensionPayload)) {
		t.Fatalf("imported extension software=%+v err=%v", importedSoftware, err)
	}
	if _, err := destination.store.GetExtensionInstallationByPackageID(ctx, extensionPackage.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("capsule import unexpectedly installed or ran extension: %v", err)
	}

	wantBytes := map[string][]byte{
		environmentAssetHash: environmentAsset, snapshotAssetHash: snapshotAsset, extensionPackage.ContentHash: extensionPayload,
	}
	for hash, want := range wantBytes {
		file, _, err := destination.vault.OpenObject(ctx, hash)
		if err != nil {
			t.Fatalf("restored Vault object %s: %v", hash, err)
		}
		got, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(got, want) {
			t.Errorf("restored Vault bytes %s mismatch: read=%v close=%v", hash, readErr, closeErr)
		}
	}
}

func importCapsuleTestObject(t *testing.T, ctx context.Context, stageVault *vault.Vault, payload []byte) string {
	t.Helper()
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	if _, err := stageVault.ImportVerifiedObject(ctx, hash, int64(len(payload)), bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	return hash
}

func capsuleHasIncludedObject(manifest Manifest, hash string) bool {
	for _, object := range manifest.Objects {
		if strings.EqualFold(object.ContentSHA256, hash) && object.Included && object.Required {
			return true
		}
	}
	return false
}
