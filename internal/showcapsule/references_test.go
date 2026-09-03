package showcapsule

import (
	"context"
	"strings"
	"testing"
)

func TestValidateObjectReferencesRejectsMissingAndOrphanObjects(t *testing.T) {
	fixture := newCapsuleFixture(t)
	defer fixture.db.Close()
	manifest, err := fixture.service.BuildManifest(context.Background(), fixture.projectID, BuildOptions{Mode: ExportManifestOnly})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateObjectReferences(manifest); err != nil {
		t.Fatalf("valid builder manifest rejected: %v", err)
	}
	without := manifest
	without.Objects = nil
	if err := validateObjectReferences(without); err == nil || !strings.Contains(err.Error(), "no ObjectEntry") {
		t.Fatalf("missing object reference error=%v", err)
	}
	orphan := manifest
	orphan.Objects = append(append([]ObjectEntry(nil), manifest.Objects...), ObjectEntry{
		ContentSHA256: strings.Repeat("a", 64),
		SizeBytes: 1,
		Required: true,
		Purposes: []string{"unrelated"},
	})
	if err := validateObjectReferences(orphan); err == nil || !strings.Contains(err.Error(), "not referenced") {
		t.Fatalf("orphan object error=%v", err)
	}
}
