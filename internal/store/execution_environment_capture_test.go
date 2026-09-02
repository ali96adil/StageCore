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

func referenceOnlyExecutionEnvironmentFixture(key string) executionenv.Manifest {
	manifest := executionEnvironmentFixture(key)
	manifest.Assets[0].CapturePolicy = executionenv.CaptureReferenceOnly
	manifest.Assets[0].ContentHash = ""
	manifest.Assets[0].SizeBytes = nil
	return manifest
}

func TestCaptureExecutionEnvironmentAssetRequiresVerifiedVaultIdentity(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 capture", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, referenceOnlyExecutionEnvironmentFixture("video-main"), "test.operator")
	if err != nil {
		t.Fatal(err)
	}

	contentHash := strings.Repeat("b", 64)
	const sizeBytes int64 = 8192
	if _, err := s.RegisterVaultObject(ctx, store.RegisterVaultObjectParams{
		ContentHash: contentHash,
		SizeBytes: sizeBytes,
		RelativePath: "objects/sha256/bb/bb/" + contentHash,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.CaptureExecutionEnvironmentAsset(ctx, created.ID, "missing", contentHash, sizeBytes); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown asset err=%v", err)
	}
	if _, err := s.CaptureExecutionEnvironmentAsset(ctx, created.ID, "workspace", strings.Repeat("c", 64), sizeBytes); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("missing Vault object err=%v", err)
	}
	if _, err := s.CaptureExecutionEnvironmentAsset(ctx, created.ID, "workspace", contentHash, sizeBytes+1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Vault size mismatch err=%v", err)
	}

	updated, err := s.CaptureExecutionEnvironmentAsset(ctx, created.ID, "workspace", contentHash, sizeBytes)
	if err != nil {
		t.Fatal(err)
	}
	asset := updated.Manifest.Assets[0]
	if asset.CapturePolicy != executionenv.CaptureContentBound || asset.ContentHash != contentHash || asset.SizeBytes == nil || *asset.SizeBytes != sizeBytes {
		t.Fatalf("captured asset=%+v", asset)
	}
	if asset.Locator != "/Users/show/Stage.vdmx5" {
		t.Fatalf("captured locator=%q", asset.Locator)
	}
	wantManifestHash, err := executionenv.ContentHash(updated.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ContentSHA256 != wantManifestHash || updated.ContentSHA256 == created.ContentSHA256 {
		t.Fatalf("manifest hash got=%s before=%s want=%s", updated.ContentSHA256, created.ContentSHA256, wantManifestHash)
	}

	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CaptureExecutionEnvironmentAsset(ctx, created.ID, "workspace", contentHash, sizeBytes); !errors.Is(err, domain.ErrRevisionFrozen) {
		t.Fatalf("frozen capture err=%v", err)
	}
}

func TestCaptureExecutionEnvironmentAssetSHOWLockPrecedesFrozenRevision(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)

	manifest := referenceOnlyExecutionEnvironmentFixture("video-main")
	canonical, err := executionenv.CanonicalBytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestHash, err := executionenv.ContentHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestID := "00000000-0000-7000-8000-000000000125"
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO execution_environment_manifests (
			environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
			manifest_json, content_sha256, created_by, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		manifestID, runtimeSnapshot.RevisionID, manifest.EnvironmentKey, manifest.AdapterKey, manifest.Application.Key,
		string(canonical), manifestHash, "test", 1,
	); err != nil {
		t.Fatal(err)
	}

	contentHash := strings.Repeat("d", 64)
	const sizeBytes int64 = 1024
	if _, err := s.RegisterVaultObject(ctx, store.RegisterVaultObjectParams{
		ContentHash: contentHash,
		SizeBytes: sizeBytes,
		RelativePath: "objects/sha256/dd/dd/" + contentHash,
	}); err != nil {
		t.Fatal(err)
	}

	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "F-025 capture lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CaptureExecutionEnvironmentAsset(ctx, manifestID, "workspace", contentHash, sizeBytes); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("SHOW capture err=%v", err)
	}
	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CaptureExecutionEnvironmentAsset(ctx, manifestID, "workspace", contentHash, sizeBytes); !errors.Is(err, domain.ErrRevisionFrozen) {
		t.Fatalf("post-SHOW frozen capture err=%v", err)
	}
}
