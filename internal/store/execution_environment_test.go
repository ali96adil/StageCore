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

func executionEnvSize(value int64) *int64 { return &value }

func executionEnvironmentFixture(key string) executionenv.Manifest {
	return executionenv.Manifest{
		SchemaVersion:  executionenv.ManifestSchemaVersion,
		EnvironmentKey: key,
		Name:           "Main video workstation",
		AdapterKey:     "stagecore.adapter.vdmx",
		Application: executionenv.ApplicationRequirement{
			Key: "vdmx", Name: "VDMX", Vendor: "VIDVOX", VersionConstraint: "8.x-tested",
			Hosts: []executionenv.HostRequirement{{OS: "darwin", Architecture: "arm64"}},
		},
		Assets: []executionenv.AssetRequirement{{
			Key: "workspace", Kind: executionenv.AssetProjectFile, Name: "Workspace",
			CapturePolicy: executionenv.CaptureContentBound,
			ContentHash: strings.Repeat("a", 64), SizeBytes: executionEnvSize(4096), Locator: "/Users/show/Stage.vdmx5",
		}},
		Bindings: []executionenv.BindingRequirement{{
			Key: "main-output", Kind: executionenv.BindingDisplay, Name: "Main Output",
			ExternalRef: "display:main", Required: true,
		}},
	}
}

func TestExecutionEnvironmentManifestPersistenceIntegrityAndDraftRules(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	manifest := executionEnvironmentFixture("video-main")
	created, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, manifest, "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := executionenv.ContentHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if created.RevisionID != revision.ID || created.Manifest.EnvironmentKey != "video-main" || created.ContentSHA256 != wantHash || !created.CreatedAt.Equal(fixedTime) {
		t.Fatalf("created=%+v", created)
	}

	canonical, err := executionenv.CanonicalBytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var storedJSON, storedHash string
	if err := handle.DB.QueryRowContext(ctx, `SELECT manifest_json, content_sha256 FROM execution_environment_manifests WHERE environment_manifest_id = ?`, created.ID).Scan(&storedJSON, &storedHash); err != nil {
		t.Fatal(err)
	}
	if storedJSON != string(canonical) || storedHash != wantHash {
		t.Fatalf("stored canonical/hash mismatch json=%q hash=%s", storedJSON, storedHash)
	}

	listed, err := s.ListExecutionEnvironmentManifests(ctx, revision.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	if _, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, manifest, "test.operator"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate environment err=%v", err)
	}

	secondManifest := executionEnvironmentFixture("video-secondary")
	second, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, secondManifest, "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteExecutionEnvironmentManifest(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetExecutionEnvironmentManifest(ctx, second.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted get err=%v", err)
	}

	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	frozenManifest := executionEnvironmentFixture("frozen-new")
	if _, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, frozenManifest, "test.operator"); !errors.Is(err, domain.ErrRevisionFrozen) {
		t.Fatalf("frozen revision create err=%v", err)
	}

	// Durable manifest bytes are identity-bearing. Even valid JSON whitespace drift
	// must fail closed instead of being silently normalized on read.
	if _, err := handle.DB.ExecContext(ctx, `UPDATE execution_environment_manifests SET manifest_json = ' ' || manifest_json WHERE environment_manifest_id = ?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetExecutionEnvironmentManifest(ctx, created.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("tampered canonical bytes err=%v", err)
	}
}

func TestExecutionEnvironmentManifestSHOWLockAllowsReadsAndRejectsMutation(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	manifest := executionEnvironmentFixture("video-main")
	canonical, err := executionenv.CanonicalBytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	contentHash, err := executionenv.ContentHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestID := "00000000-0000-7000-8000-000000000025"
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO execution_environment_manifests (
			environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
			manifest_json, content_sha256, created_by, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		manifestID, runtimeSnapshot.RevisionID, manifest.EnvironmentKey, manifest.AdapterKey, manifest.Application.Key,
		string(canonical), contentHash, "test", 1,
	); err != nil {
		t.Fatal(err)
	}

	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "F-025 lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetExecutionEnvironmentManifest(ctx, manifestID); err != nil {
		t.Fatalf("SHOW read failed: %v", err)
	}
	if items, err := s.ListExecutionEnvironmentManifests(ctx, runtimeSnapshot.RevisionID); err != nil || len(items) != 1 {
		t.Fatalf("SHOW list=%+v err=%v", items, err)
	}

	newManifest := executionEnvironmentFixture("video-new")
	if _, err := s.CreateExecutionEnvironmentManifest(ctx, runtimeSnapshot.RevisionID, newManifest, "test.operator"); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("SHOW store create err=%v", err)
	}
	if err := s.DeleteExecutionEnvironmentManifest(ctx, manifestID); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("SHOW store delete err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE execution_environment_manifests SET created_by = 'unsafe' WHERE environment_manifest_id = ?`, manifestID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct SHOW update err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `DELETE FROM execution_environment_manifests WHERE environment_manifest_id = ?`, manifestID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct SHOW delete err=%v", err)
	}

	newCanonical, err := executionenv.CanonicalBytes(newManifest)
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := executionenv.ContentHash(newManifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO execution_environment_manifests (
			environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
			manifest_json, content_sha256, created_by, created_at_us
		) VALUES ('00000000-0000-7000-8000-000000000026', ?, ?, ?, ?, ?, ?, 'test', 1)`,
		runtimeSnapshot.RevisionID, newManifest.EnvironmentKey, newManifest.AdapterKey, newManifest.Application.Key,
		string(newCanonical), newHash,
	); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct SHOW insert err=%v", err)
	}

	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetExecutionEnvironmentManifest(ctx, manifestID); err != nil {
		t.Fatalf("post-SHOW read failed: %v", err)
	}
}
