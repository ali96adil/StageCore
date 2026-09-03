package showcapsule

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

var capsuleTestTime = time.Date(2026, 9, 3, 9, 15, 0, 0, time.UTC)

type capsuleFixture struct {
	service       *Service
	db            *db.Handle
	store         *store.Store
	projectID     string
	revisionID    string
	snapshotID    string
	mediaAssetID  string
	contentHash   string
	contentBytes  []byte
}

func TestManifestOnlyAndSelfContainedExportVerify(t *testing.T) {
	fixture := newCapsuleFixture(t)
	defer fixture.db.Close()
	ctx := context.Background()

	manifestOnly, err := fixture.service.Export(ctx, fixture.projectID, t.TempDir(), BuildOptions{
		RuntimeSnapshotID: fixture.snapshotID,
		Mode: ExportManifestOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifestOnly.Manifest.ExportMode != ExportManifestOnly || len(manifestOnly.Manifest.Objects) != 1 {
		t.Fatalf("manifest-only capsule=%+v", manifestOnly.Manifest)
	}
	if manifestOnly.Manifest.Objects[0].Included || manifestOnly.Manifest.Objects[0].ArchivePath != "" {
		t.Fatalf("manifest-only object claims included bytes: %+v", manifestOnly.Manifest.Objects[0])
	}
	if _, err := Verify(manifestOnly.Path); err != nil {
		t.Fatalf("verify manifest-only capsule: %v", err)
	}

	selfContained, err := fixture.service.Export(ctx, fixture.projectID, t.TempDir(), BuildOptions{
		RuntimeSnapshotID: fixture.snapshotID,
		Mode: ExportSelfContained,
	})
	if err != nil {
		t.Fatal(err)
	}
	if selfContained.Manifest.ExportMode != ExportSelfContained || len(selfContained.Manifest.Objects) != 1 {
		t.Fatalf("self-contained capsule=%+v", selfContained.Manifest)
	}
	object := selfContained.Manifest.Objects[0]
	if !object.Included || object.ContentSHA256 != fixture.contentHash || object.SizeBytes != int64(len(fixture.contentBytes)) {
		t.Fatalf("self-contained object=%+v", object)
	}
	objectPath := filepath.Join(selfContained.Path, filepath.FromSlash(object.ArchivePath))
	content, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(fixture.contentBytes) {
		t.Fatalf("exported content=%q", string(content))
	}
	verified, err := Verify(selfContained.Path)
	if err != nil {
		t.Fatalf("verify self-contained capsule: %v", err)
	}
	if verified.RuntimeSnapshot.RuntimeSnapshotID != fixture.snapshotID || verified.Project.ProjectID != fixture.projectID {
		t.Fatalf("verified identity=%+v", verified)
	}

	if err := os.WriteFile(objectPath, []byte("tampered"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(selfContained.Path); err == nil {
		t.Fatal("tampered capsule object unexpectedly verified")
	}
}

func TestSelfContainedRejectsRequiredReferenceOnlyMedia(t *testing.T) {
	fixture := newCapsuleFixture(t)
	defer fixture.db.Close()
	ctx := context.Background()

	if _, err := fixture.db.DB.ExecContext(ctx, `UPDATE media_assets SET asset_policy = 'REFERENCE_ONLY' WHERE media_asset_id = ?`, fixture.mediaAssetID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.BuildManifest(ctx, fixture.projectID, BuildOptions{
		RuntimeSnapshotID: fixture.snapshotID,
		Mode: ExportSelfContained,
	}); err == nil || !strings.Contains(err.Error(), "REFERENCE_ONLY") {
		t.Fatalf("self-contained REFERENCE_ONLY error=%v", err)
	}

	manifest, err := fixture.service.BuildManifest(ctx, fixture.projectID, BuildOptions{
		RuntimeSnapshotID: fixture.snapshotID,
		Mode: ExportManifestOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Media) != 1 || manifest.Media[0].ContentIncluded {
		t.Fatalf("manifest-only reference media=%+v", manifest.Media)
	}
	if len(manifest.Warnings) == 0 || !strings.Contains(strings.Join(manifest.Warnings, "\n"), "REFERENCE_ONLY") {
		t.Fatalf("reference-only warning missing: %v", manifest.Warnings)
	}
}

func TestManifestChecksumTamperingFailsVerification(t *testing.T) {
	fixture := newCapsuleFixture(t)
	defer fixture.db.Close()
	result, err := fixture.service.Export(context.Background(), fixture.projectID, t.TempDir(), BuildOptions{Mode: ExportManifestOnly})
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(result.Path, ManifestFileName)
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(manifestPath, content, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(result.Path); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered manifest verify error=%v", err)
	}
}

func newCapsuleFixture(t *testing.T) capsuleFixture {
	t.Helper()
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(handle.DB, clock.Fixed{Time: capsuleTestTime})
	stageVault, err := vault.Open(t.TempDir(), stageStore)
	if err != nil {
		handle.Close()
		t.Fatal(err)
	}
	service, err := New(stageStore, stageVault, clock.Fixed{Time: capsuleTestTime})
	if err != nil {
		handle.Close()
		t.Fatal(err)
	}
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Capsule Test Show", Description: "Portable test show", CreatedBy: "owner", ChangeNote: "capsule fixture",
	})
	if err != nil {
		handle.Close()
		t.Fatal(err)
	}
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "VIDEO-MAIN", LogicalType: "VIDEO", TargetRef: "VIDEO-MAIN",
		ProjectConfig: []byte(`{"host":"127.0.0.1"}`),
	}); err != nil {
		handle.Close()
		t.Fatal(err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1,
		Enabled: true, NotesSummary: "Stand by projection",
	}, []domain.Action{{
		OrderIndex: 1, TargetRef: "VIDEO-MAIN", CapabilityKey: "osc.send", Enabled: true,
	}}); err != nil {
		handle.Close()
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "video-main", DisplayName: "Video Main", RequiredCapabilities: []string{"osc.send"}, Required: true,
	})
	if err != nil {
		handle.Close()
		t.Fatal(err)
	}
	content := []byte("show-capsule-media-payload")
	managed, err := stageVault.ImportManaged(ctx, vault.ImportParams{
		ProjectID: project.ID, Name: "Opening Clip", AssetPolicy: store.MediaPolicyArchiveRequired, OriginalFilename: "opening.mov",
	}, strings.NewReader(string(content)))
	if err != nil {
		handle.Close()
		t.Fatal(err)
	}
	if _, err := stageStore.AddMachineRoleMediaRequirement(ctx, role.ID, managed.Version.ID, true); err != nil {
		handle.Close()
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		handle.Close()
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		handle.Close()
		t.Fatal(err)
	}
	return capsuleFixture{
		service: service, db: handle, store: stageStore, projectID: project.ID, revisionID: revision.ID,
		snapshotID: runtimeSnapshot.ID, mediaAssetID: managed.Asset.ID, contentHash: managed.Version.ContentHash,
		contentBytes: content,
	}
}
