package showcapsule

import (
	"context"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/store"
)

func TestPlanImportBlocksUnresolvedReferenceOnlyMedia(t *testing.T) {
	ctx := context.Background()
	source := newCapsuleFixture(t)
	defer source.db.Close()
	if _, err := source.db.DB.ExecContext(ctx, `UPDATE media_assets SET asset_policy = ? WHERE media_asset_id = ?`, store.MediaPolicyReferenceOnly, source.mediaAssetID); err != nil {
		t.Fatal(err)
	}
	exported, err := source.service.Export(ctx, source.projectID, t.TempDir(), BuildOptions{Mode: ExportManifestOnly})
	if err != nil {
		t.Fatal(err)
	}

	destination := newEmptyCapsuleService(t)
	defer destination.db.Close()
	plan, err := destination.service.PlanImport(ctx, exported.Path)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaterializationReady {
		t.Fatalf("REFERENCE_ONLY media without exact local bytes unexpectedly materialization-ready: %+v", plan.Checks)
	}
	foundBlock := false
	for _, check := range plan.Checks {
		if check.Code == "media.reference_only_unresolved" && check.Severity == ReadinessBlock {
			foundBlock = true
		}
	}
	if !foundBlock {
		t.Fatalf("missing REFERENCE_ONLY materialization blocker: %+v", plan.Checks)
	}

	object, err := destination.vault.ImportObject(ctx, strings.NewReader(string(source.contentBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if object.ContentHash != source.contentHash {
		t.Fatalf("preloaded reference content hash=%s want=%s", object.ContentHash, source.contentHash)
	}
	resolved, err := destination.service.PlanImport(ctx, exported.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.MaterializationReady {
		t.Fatalf("exact locally resolved REFERENCE_ONLY media should allow materialization: %+v", resolved.Checks)
	}
	if resolved.ReplacementHostReady {
		t.Fatal("REFERENCE_ONLY policy should remain visible as replacement-host review work")
	}
}
