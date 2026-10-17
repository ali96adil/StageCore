package projectaccess_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestScopedProjectFilesIsEmptyInitiallyAndFollowsProjectIdentity(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	projects := store.New(f.db.DB, clock.Real{})
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ListScopedProjectFiles(ctx, f.operatorID, f.projectA);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("project.view alone exposed Project Files: %v", err)
	}
	workspace, err := f.a.ListScopedProjectFiles(ctx, f.ownerID, f.projectA)
	if err != nil || workspace.ProjectID != f.projectA ||
		workspace.ProjectName != "قرية صالح" || len(workspace.Files) != 0 {
		t.Fatalf("new Project should have an empty logical workspace: %+v, %v", workspace, err)
	}

	media := []struct{ name, filename, hash, relative, category string }{
		{"Music", "scene.wav", strings.Repeat("a", 64), "objects/a", "AUDIO"},
		{"Projection", "scene.MOV", strings.Repeat("b", 64), "objects/b", "VIDEO"},
		{"Poster", "poster.png", strings.Repeat("c", 64), "objects/c", "IMAGE"},
		{"Notes", "runbook.txt", strings.Repeat("d", 64), "objects/d", "OTHER"},
	}
	for _, item := range media {
		if _, err := projects.RegisterManagedMedia(ctx, store.RegisterManagedMediaParams{
			ProjectID: f.projectA, Name: item.name, AssetPolicy: store.MediaPolicyManaged,
			ContentHash: item.hash, SizeBytes: 128, RelativePath: item.relative,
			OriginalFilename: item.filename,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.MediaRead}); err != nil {
		t.Fatal(err)
	}
	workspace, err = f.a.ListScopedProjectFiles(ctx, f.operatorID, f.projectA)
	if err != nil || len(workspace.Files) != len(media) {
		t.Fatalf("qualified Project files: %+v err=%v", workspace, err)
	}
	byName := map[string]projectaccess.ProjectFile{}
	for _, item := range workspace.Files {
		if item.MediaAssetID == "" || item.ContentVersionID == "" ||
			item.SizeBytes != 128 || item.Category == "" {
			t.Fatalf("file metadata incomplete: %+v", item)
		}
		byName[item.MediaAssetName] = item
	}
	for _, expected := range media {
		got, ok := byName[expected.name]
		if !ok || got.Category != expected.category || got.OriginalFilename != expected.filename {
			t.Fatalf("file category mismatch: %+v expected=%+v", got, expected)
		}
	}

	// A new Project is a different workspace even if duplication copies
	// the underlying immutable Vault objects.
	clone, _, err := projects.DuplicateProjectAuthoring(ctx, f.projectA, "نسخة جديدة", f.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ListScopedProjectFiles(ctx, f.operatorID, clone.ID);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("duplicated Project inherited operator membership: %v", err)
	}
	copied, err := f.a.ListScopedProjectFiles(ctx, f.ownerID, clone.ID)
	if err != nil || copied.ProjectID != clone.ID || copied.ProjectName != "نسخة جديدة" {
		t.Fatalf("cloned workspace unavailable: %+v %v", copied, err)
	}
	if len(copied.Files) != len(media) {
		t.Fatalf("cloned Project media refs not present: %d", len(copied.Files))
	}
	for _, copiedFile := range copied.Files {
		if source := byName[copiedFile.MediaAssetName]; copiedFile.MediaAssetID == source.MediaAssetID ||
			copiedFile.ContentVersionID == source.ContentVersionID {
			t.Fatalf("copied Project reused source media identity: %+v", copiedFile)
		}
	}

	// Rename only the display name. Do not derive workspace identity from
	// the name, and never move content-hash paths on disk.
	if _, err := f.db.DB.ExecContext(ctx,
		`UPDATE projects SET name = ? WHERE project_id = ?`, "قرية صالح — الاسم الجديد", f.projectA); err != nil {
		t.Fatal(err)
	}
	renamed, err := f.a.ListScopedProjectFiles(ctx, f.operatorID, f.projectA)
	if err != nil || renamed.ProjectID != f.projectA ||
		renamed.ProjectName != "قرية صالح — الاسم الجديد" ||
		len(renamed.Files) != len(media) {
		t.Fatalf("rename broke Project workspace: %+v err=%v", renamed, err)
	}
	if _, err := f.a.ListScopedProjectFiles(ctx, f.operatorID, f.projectB);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("Operator could enumerate an unassigned Project: %v", err)
	}
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ListScopedProjectFiles(ctx, f.operatorID, f.projectA);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("revoked operator kept Project Files access: %v", err)
	}
}

func TestProjectFileWorkspaceBoundsUnpagedListing(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tx, err := f.db.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 501; i++ {
		id := fmt.Sprintf("11111111-2222-4333-8444-%012d", i)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_assets
			(media_asset_id, project_id, name, asset_policy, created_at_us, updated_at_us)
			VALUES (?, ?, ?, 'REFERENCE_ONLY', 1, 1)
		`, id, f.projectA, id); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	workspace, err := f.a.ListScopedProjectFiles(ctx, f.ownerID, f.projectA)
	if !errors.Is(err, projectaccess.ErrWorkspacePaginationRequired) ||
		len(workspace.Files) != 0 {
		t.Fatalf("large workspace silently truncated: %+v err=%v", workspace, err)
	}
	if _, err := f.a.ListScopedProjectFiles(ctx, f.ownerID, "unknown");
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("unknown Project exposed: %v", err)
	}
}
