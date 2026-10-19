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

func TestMediaResourceScopeRequiresActualProjectOwnershipAndMediaRead(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	projects := store.New(f.db.DB, clock.Real{})

	// A single immutable Vault object may be shared by two distinct Projects.
	// A raw content hash is never a sufficient Project authorization token.
	hash := strings.Repeat("a", 64)
	create := func(projectID, name string) store.ManagedMedia {
		t.Helper()
		m, err := projects.RegisterManagedMedia(ctx, store.RegisterManagedMediaParams{
			ProjectID: projectID, Name: name,
			AssetPolicy: store.MediaPolicyManaged,
			ContentHash: hash, SizeBytes: 128,
			RelativePath: "objects/shared-blob",
			OriginalFilename: "shared.wav",
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	assetA := create(f.projectA, "Show A audio")
	assetB := create(f.projectB, "Show B audio")
	if assetA.Object.ContentHash != assetB.Object.ContentHash ||
		assetA.Asset.ID == assetB.Asset.ID || assetA.Version.ID == assetB.Version.ID {
		t.Fatal("fixture must use separate Project media identities over a shared Vault object")
	}

	assetResources := func(m store.ManagedMedia) []struct {
		kind projectaccess.ResourceKind
		id   string
	} {
		return []struct {
			kind projectaccess.ResourceKind
			id   string
		}{
			{projectaccess.ResourceMediaAsset, m.Asset.ID},
			{projectaccess.ResourceMediaVersion, m.Version.ID},
			{projectaccess.ResourceMediaLocation, m.Location.ID},
		}
	}
	aResources := assetResources(assetA)
	bResources := assetResources(assetB)

	// Viewing a Project alone does not allow downloading any of its media.
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView}); err != nil {
		t.Fatal(err)
	}
	for _, resource := range aResources {
		got, err := f.a.ResolveProjectForResource(ctx, resource.kind, resource.id)
		if err != nil || got != f.projectA {
			t.Fatalf("A media owner resolution %v: %q, %v", resource, got, err)
		}
		if err := f.a.CheckProjectResourceRead(ctx, f.operatorID, f.projectA, resource.kind, resource.id);
			!errors.Is(err, projectaccess.ErrDenied) {
			t.Fatalf("project.view alone exposed %v: %v", resource, err)
		}
	}
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA,
		[]projectaccess.Permission{projectaccess.ProjectView, projectaccess.MediaRead}); err != nil {
		t.Fatal(err)
	}
	for _, resource := range aResources {
		if err := f.a.CheckProjectResourceRead(ctx, f.operatorID, f.projectA, resource.kind, resource.id); err != nil {
			t.Fatalf("assigned Project A media unexpectedly denied %v: %v", resource, err)
		}
	}
	for _, resource := range bResources {
		got, err := f.a.ResolveProjectForResource(ctx, resource.kind, resource.id)
		if err != nil || got != f.projectB {
			t.Fatalf("B media owner resolution %v: %q, %v", resource, got, err)
		}
		if err := f.a.CheckProjectResourceRead(ctx, f.operatorID, f.projectA, resource.kind, resource.id);
			!errors.Is(err, projectaccess.ErrDenied) {
			t.Fatalf("foreign Project B media reached through A URL %v: %v", resource, err)
		}
		if err := f.a.CheckProjectResourceRead(ctx, f.operatorID, f.projectB, resource.kind, resource.id);
			!errors.Is(err, projectaccess.ErrDenied) {
			t.Fatalf("unassigned Project B media leaked %v: %v", resource, err)
		}
		if err := f.a.CheckProjectResourceRead(ctx, f.ownerID, f.projectB, resource.kind, resource.id); err != nil {
			t.Fatalf("Owner unable to access B media %v: %v", resource, err)
		}
	}

	// A guessed hash, unknown kind and missing database row must all fail.
	if _, err := f.a.ResolveProjectForResource(ctx, projectaccess.ResourceKind("vault_content_hash"), hash);
		!errors.Is(err, projectaccess.ErrDenied) {
		t.Fatalf("raw Vault hash resolved as Project authority: %v", err)
	}
	for _, kind := range []projectaccess.ResourceKind{
		projectaccess.ResourceMediaAsset,
		projectaccess.ResourceMediaVersion,
		projectaccess.ResourceMediaLocation,
	} {
		if _, err := f.a.ResolveProjectForResource(ctx, kind, "not-an-id");
			!errors.Is(err, projectaccess.ErrNotFound) {
			t.Fatalf("missing %s returned %v, want ErrNotFound", kind, err)
		}
		if err := f.a.CheckProjectResourceRead(ctx, f.operatorID, f.projectA, kind, "not-an-id");
			!errors.Is(err, projectaccess.ErrNotFound) {
			t.Fatalf("missing %s read returned %v, want ErrNotFound", kind, err)
		}
	}
	if err := f.a.SetPermissions(ctx, f.ownerID, f.operatorID, f.projectA, nil); err != nil {
		t.Fatal(err)
	}
	for _, resource := range aResources {
		if err := f.a.CheckProjectResourceRead(ctx, f.operatorID, f.projectA, resource.kind, resource.id);
			!errors.Is(err, projectaccess.ErrDenied) {
			t.Fatalf("revoke left Project A media accessible %v: %v", resource, err)
		}
	}
}
