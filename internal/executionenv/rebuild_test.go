package executionenv

import (
	"strings"
	"testing"
)

func rebuildManifest(t *testing.T) Manifest {
	t.Helper()
	size := int64(12)
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		EnvironmentKey: "video-primary",
		Name: "Primary Video",
		AdapterKey: "stagecore.adapter.vdmx",
		Application: ApplicationRequirement{Key: "vdmx", Name: "VDMX", VersionConstraint: "6.x-tested", Hosts: []HostRequirement{{OS: "darwin", Architecture: "arm64"}}},
		Assets: []AssetRequirement{{Key: "project", Name: "Project", Kind: AssetProjectFile, CapturePolicy: CaptureContentBound, ContentHash: strings.Repeat("1", 64), SizeBytes: &size}},
		Launch: &LaunchTarget{Kind: LaunchAsset, AssetKey: "project"},
	}
	if _, err := Normalize(manifest); err != nil { t.Fatal(err) }
	return manifest
}

func TestBuildRebuildPlanIsTruthfulAndDeterministic(t *testing.T) {
	manifest := rebuildManifest(t)
	hash, _ := ContentHash(manifest)
	snapshot := validSnapshot()
	snapshot.SourceManifestSHA256 = hash
	plan, err := BuildRebuildPlan(manifest, &snapshot)
	if err != nil { t.Fatal(err) }
	rendered := RenderRebuildPlan(plan)
	if !strings.Contains(rendered, "Destination readiness requires a fresh StageCore execution-environment inspection") { t.Fatalf("missing readiness disclaimer: %s", rendered) }
	if !strings.Contains(rendered, "Reference-only dependencies") || !strings.Contains(rendered, "Manual reconstruction") { t.Fatalf("missing truthful reconstruction sections: %s", rendered) }
	plan2, err := BuildRebuildPlan(manifest, &snapshot)
	if err != nil { t.Fatal(err) }
	if RenderRebuildPlan(plan2) != rendered { t.Fatal("rebuild rendering is not deterministic") }
}

func TestBuildRebuildPlanRejectsSnapshotFromDifferentManifest(t *testing.T) {
	manifest := rebuildManifest(t)
	snapshot := validSnapshot()
	if _, err := BuildRebuildPlan(manifest, &snapshot); err == nil { t.Fatal("expected source manifest mismatch") }
}
