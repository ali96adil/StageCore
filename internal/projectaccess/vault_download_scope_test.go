package projectaccess_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestScopedVaultObjectReadBindsUserProjectMediaVersionAndHash(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	mediaStore := store.New(f.db.DB, clock.Real{})
	sharedHash := strings.Repeat("a", 64)
	otherHash := strings.Repeat("b", 64)

	// Different Projects can legitimately reference the same underlying blob.
	create := func(project, name, hash, path string) store.ManagedMedia {
		t.Helper()
		m, err := mediaStore.RegisterManagedMedia(ctx, store.RegisterManagedMediaParams{
			ProjectID: project, Name: name, AssetPolicy: store.MediaPolicyManaged,
			ContentHash: hash, SizeBytes: 1024, RelativePath: path,
			OriginalFilename: "file.wav",
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := create(f.projectA, "A media", sharedHash, "objects/shared")
	b := create(f.projectB, "B media", sharedHash, "objects/shared")
	otherA := create(f.projectA, "Other A media", otherHash, "objects/other")

	check := func(user, project, version, hash string) error {
		t.Helper()
		return f.a.CheckScopedVaultObjectRead(ctx, user, project, version, hash)
	}
	denied := func(user, project, version, hash string) {
		t.Helper()
		if err := check(user, project, version, hash); !errors.Is(err, projectaccess.ErrDenied) {
			t.Fatalf("unexpected scoped download user=%s project=%s version=%s hash=%s: %v", user, project, version, hash, err)
		}
	}

	denied(f.operatorID, f.projectA, a.Version.ID, sharedHash)
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	denied(f.operatorID, f.projectA, a.Version.ID, sharedHash) // view alone is not media.read

	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.MediaRead}); err != nil {
		t.Fatal(err)
	}
	if err := check(f.operatorID, f.projectA, a.Version.ID, sharedHash); err != nil {
		t.Fatalf("valid A download denied: %v", err)
	}
	if err := check(f.operatorID, f.projectA, otherA.Version.ID, otherHash); err != nil {
		t.Fatalf("valid second A download denied: %v", err)
	}
	// Same content_hash does not make the Project B version accessible.
	denied(f.operatorID, f.projectA, b.Version.ID, sharedHash)
	denied(f.operatorID, f.projectB, b.Version.ID, sharedHash)
	denied(f.operatorID, f.projectB, a.Version.ID, sharedHash)
	denied(f.operatorID, f.projectA, a.Version.ID, otherHash)
	denied(f.operatorID, f.projectA, otherA.Version.ID, sharedHash)
	denied(f.operatorID, f.projectA, "nonexistent-version", sharedHash)
	denied(f.operatorID, f.projectA, "", sharedHash)
	denied(f.operatorID, f.projectA, a.Version.ID, strings.ToUpper(sharedHash))
	denied(f.operatorID, f.projectA, a.Version.ID, strings.Repeat("z", 64))
	denied(f.operatorID, f.projectA, a.Version.ID, "a")
	denied(f.operatorID, "", a.Version.ID, sharedHash)
	denied("", f.projectA, a.Version.ID, sharedHash)

	// Owner authority is still scoped to a real Project/version/hash tuple.
	if err := check(f.ownerID, f.projectB, b.Version.ID, sharedHash); err != nil {
		t.Fatalf("Owner valid Project B download denied: %v", err)
	}
	denied(f.ownerID, f.projectA, b.Version.ID, sharedHash)

	// Grant revocation and account disable must be applied at each call.
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA, nil); err != nil {
		t.Fatal(err)
	}
	denied(f.operatorID, f.projectA, a.Version.ID, sharedHash)
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.MediaRead}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.users.SetUserEnabled(ctx, f.operatorID, false); err != nil {
		t.Fatal(err)
	}
	denied(f.operatorID, f.projectA, a.Version.ID, sharedHash)
}

func TestScopedVaultObjectReadRequiresRegisteredMediaReference(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	hash := strings.Repeat("c", 64)
	_, err := f.db.DB.ExecContext(ctx, `
		INSERT INTO vault_objects (content_hash, size_bytes, relative_path, created_at_us)
		VALUES (?, 100, 'objects/unreferenced', 1)
	`, hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.a.CheckScopedVaultObjectRead(ctx, f.ownerID, f.projectA, "guessed-media-version", hash);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("orphan Vault hash granted an owner Project download: %v", err)
	}
	var nilAccess *projectaccess.Service
	if err := nilAccess.CheckScopedVaultObjectRead(ctx, f.ownerID, f.projectA, "guessed-media-version", hash);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("nil access service did not fail closed: %v", err)
	}
}
