package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

func executionEnvironmentSnapshotFixture(manifest store.ExecutionEnvironmentManifest) executionenv.Snapshot {
	return executionenv.Snapshot{
		SchemaVersion:        executionenv.SnapshotSchemaVersion,
		EnvironmentKey:       manifest.Manifest.EnvironmentKey,
		AdapterKey:           manifest.Manifest.AdapterKey,
		SourceManifestSHA256: manifest.ContentSHA256,
		CaptureStatus:        executionenv.SnapshotPartial,
		Items: []executionenv.SnapshotItem{{
			Key:         "published-controls",
			Name:        "Published controls",
			Kind:        executionenv.SnapshotControlNamespace,
			Provenance:  executionenv.ProvenanceOSCQuery,
			Capture:     executionenv.ItemObserved,
			Portability: executionenv.SnapshotDescriptiveOnly,
			Notes:       "Partial namespace observation only.",
		}},
	}
}

func TestExecutionEnvironmentSnapshotPersistenceIntegrityAndDraftRules(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 snapshot", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionEnvironmentFixture("snapshot-main"), "test.operator")
	if err != nil {
		t.Fatal(err)
	}

	snapshot := executionEnvironmentSnapshotFixture(manifest)
	created, err := s.CreateExecutionEnvironmentSnapshot(ctx, manifest.ID, snapshot, "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if created.EnvironmentManifestID != manifest.ID || created.RevisionID != revision.ID || !created.CreatedAt.Equal(fixedTime) {
		t.Fatalf("created=%+v", created)
	}
	wantHash, err := executionenv.SnapshotContentHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if created.ContentSHA256 != wantHash {
		t.Fatalf("hash=%s want=%s", created.ContentSHA256, wantHash)
	}
	listed, err := s.ListExecutionEnvironmentSnapshots(ctx, manifest.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	latest, err := s.GetLatestExecutionEnvironmentSnapshot(ctx, manifest.ID)
	if err != nil || latest.ID != created.ID {
		t.Fatalf("latest=%+v err=%v", latest, err)
	}
	if _, err := s.CreateExecutionEnvironmentSnapshot(ctx, manifest.ID, snapshot, "test.operator"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate snapshot err=%v", err)
	}

	missingHash := strings.Repeat("c", 64)
	missingSize := int64(12)
	contentSnapshot := executionEnvironmentSnapshotFixture(manifest)
	contentSnapshot.Items = []executionenv.SnapshotItem{{
		Key: "workspace-template", Name: "Workspace template", Kind: executionenv.SnapshotTemplate,
		Provenance: executionenv.ProvenanceApplicationExport, Capture: executionenv.ItemCaptured,
		Portability: executionenv.SnapshotContentBound, ContentHash: missingHash, SizeBytes: &missingSize,
	}}
	if _, err := s.CreateExecutionEnvironmentSnapshot(ctx, manifest.ID, contentSnapshot, "test.operator"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("missing Vault object err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `INSERT INTO vault_objects (content_hash, size_bytes, relative_path, created_at_us) VALUES (?, ?, ?, ?)`, missingHash, missingSize, "f025/snapshot-object", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateExecutionEnvironmentSnapshot(ctx, manifest.ID, contentSnapshot, "test.operator"); err != nil {
		t.Fatalf("verified Vault-backed snapshot: %v", err)
	}

	if _, err := handle.DB.ExecContext(ctx, `UPDATE execution_environment_snapshots SET snapshot_json = ' ' || snapshot_json WHERE environment_snapshot_id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetExecutionEnvironmentSnapshot(ctx, created.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("tampered snapshot err=%v", err)
	}

	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	fresh := executionEnvironmentSnapshotFixture(manifest)
	fresh.Notes = "new frozen snapshot"
	if _, err := s.CreateExecutionEnvironmentSnapshot(ctx, manifest.ID, fresh, "test.operator"); !errors.Is(err, domain.ErrRevisionFrozen) {
		t.Fatalf("frozen snapshot create err=%v", err)
	}
}

func TestExecutionEnvironmentSnapshotSHOWLockAllowsReadsAndRejectsMutation(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	manifestValue := executionEnvironmentFixture("snapshot-show")
	manifestCanonical, err := executionenv.CanonicalBytes(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifestHash, err := executionenv.ContentHash(manifestValue)
	if err != nil {
		t.Fatal(err)
	}
	manifestID := "00000000-0000-7000-8000-000000000127"
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO execution_environment_manifests (
			environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
			manifest_json, content_sha256, created_by, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'test', 1)`,
		manifestID, runtimeSnapshot.RevisionID, manifestValue.EnvironmentKey, manifestValue.AdapterKey,
		manifestValue.Application.Key, string(manifestCanonical), manifestHash,
	); err != nil {
		t.Fatal(err)
	}
	manifest, err := s.GetExecutionEnvironmentManifest(ctx, manifestID)
	if err != nil {
		t.Fatal(err)
	}
	snapshotValue := executionEnvironmentSnapshotFixture(manifest)
	snapshotCanonical, err := executionenv.SnapshotCanonicalBytes(snapshotValue)
	if err != nil {
		t.Fatal(err)
	}
	snapshotHash, err := executionenv.SnapshotContentHash(snapshotValue)
	if err != nil {
		t.Fatal(err)
	}
	snapshotID := "00000000-0000-7000-8000-000000000128"
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO execution_environment_snapshots (
			environment_snapshot_id, environment_manifest_id, revision_id, source_manifest_sha256,
			snapshot_json, content_sha256, created_by, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, 'test', 1)`,
		snapshotID, manifestID, runtimeSnapshot.RevisionID, manifestHash, string(snapshotCanonical), snapshotHash,
	); err != nil {
		t.Fatal(err)
	}

	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "F-025 snapshot lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetExecutionEnvironmentSnapshot(ctx, snapshotID); err != nil {
		t.Fatalf("SHOW read failed: %v", err)
	}
	if err := s.DeleteExecutionEnvironmentSnapshot(ctx, snapshotID); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("SHOW delete err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE execution_environment_snapshots SET created_by = 'unsafe' WHERE environment_snapshot_id = ?`, snapshotID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct SHOW update err=%v", err)
	}
	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
}
