package executionenv

import (
	"strings"
	"testing"
)

func assistedRebuildSourceFixture() Snapshot {
	return Snapshot{
		SchemaVersion:              SnapshotSchemaVersion,
		EnvironmentKey:             "video-main",
		AdapterKey:                 "stagecore.adapter.vdmx",
		SourceManifestSHA256:       strings.Repeat("a", 64),
		CaptureStatus:              SnapshotPartial,
		RebuildPlanVersion:         SnapshotRebuildPlanVersion,
		ReconstructionFingerprint: strings.Repeat("b", 64),
		RebuildPlan: []SnapshotRebuildStep{
			{Step: 1, Action: "Open VDMX", Status: "OBSERVED", ProvenanceClass: SnapshotProvenanceObserved},
			{Step: 2, Action: "Recreate unsupported internals manually", Status: "MANUAL", ProvenanceClass: SnapshotProvenanceUnsupported},
		},
		Items: []SnapshotItem{{
			Key: "vdmx-app", Name: "VDMX", Kind: SnapshotOther,
			Provenance: ProvenanceAdapterObservation,
			ProvenanceClass: SnapshotProvenanceObserved,
			Capture: ItemObserved, Portability: SnapshotDescriptiveOnly,
		}},
	}
}

func TestSeedAssistedRebuildPlanPreservesImmutableSourceLineage(t *testing.T) {
	source := assistedRebuildSourceFixture()
	plan, err := SeedAssistedRebuildPlan(source)
	if err != nil { t.Fatal(err) }
	if plan.SchemaVersion != AssistedRebuildPlanSchemaVersion ||
		len(plan.Steps) != 2 ||
		plan.Steps[0].SourceStep == nil || *plan.Steps[0].SourceStep != 1 ||
		plan.Steps[0].ProvenanceClass != SnapshotProvenanceObserved {
		t.Fatalf("plan=%+v", plan)
	}
	wantHash, err := SnapshotContentHash(source)
	if err != nil { t.Fatal(err) }
	if plan.SourceSnapshotSHA256 != wantHash ||
		plan.SourceReconstructionFingerprint != source.ReconstructionFingerprint {
		t.Fatalf("source lineage=%+v", plan)
	}
}

func TestAssistedRebuildPlanRequiresUserDeclaredProvenanceForEdits(t *testing.T) {
	source := assistedRebuildSourceFixture()
	plan, err := SeedAssistedRebuildPlan(source)
	if err != nil { t.Fatal(err) }
	plan.Steps[0].Action = "Open VDMX and select the show workspace"
	if _, err := NormalizeAssistedRebuildPlan(plan, source); err == nil {
		t.Fatal("edited observed step retained OBSERVED provenance")
	}
	plan.Steps[0].ProvenanceClass = SnapshotProvenanceUserDeclared
	if _, err := NormalizeAssistedRebuildPlan(plan, source); err != nil {
		t.Fatalf("user-declared edited source step: %v", err)
	}
	plan.Steps = append(plan.Steps, AssistedRebuildStep{
		Step: 3, Action: "Verify projector output manually", Status: "MANUAL",
		ProvenanceClass: SnapshotProvenanceObserved,
	})
	if _, err := NormalizeAssistedRebuildPlan(plan, source); err == nil {
		t.Fatal("operator-added step retained non-user provenance")
	}
	plan.Steps[2].ProvenanceClass = SnapshotProvenanceUserDeclared
	if _, err := NormalizeAssistedRebuildPlan(plan, source); err != nil {
		t.Fatalf("user-declared manual step: %v", err)
	}
}

func TestAssistedRebuildPlanRejectsWrongSourceIdentityAndDuplicateLineage(t *testing.T) {
	source := assistedRebuildSourceFixture()
	plan, err := SeedAssistedRebuildPlan(source)
	if err != nil { t.Fatal(err) }
	plan.SourceSnapshotSHA256 = strings.Repeat("c", 64)
	if _, err := NormalizeAssistedRebuildPlan(plan, source); err == nil {
		t.Fatal("wrong source snapshot hash accepted")
	}

	plan, err = SeedAssistedRebuildPlan(source)
	if err != nil { t.Fatal(err) }
	duplicate := 1
	plan.Steps[1].SourceStep = &duplicate
	plan.Steps[1].Action = plan.Steps[0].Action
	plan.Steps[1].Status = plan.Steps[0].Status
	plan.Steps[1].Notes = plan.Steps[0].Notes
	plan.Steps[1].ProvenanceClass = plan.Steps[0].ProvenanceClass
	if _, err := NormalizeAssistedRebuildPlan(plan, source); err == nil {
		t.Fatal("duplicate source lineage accepted")
	}
}

func TestAssistedRebuildPlanCanonicalRoundTrip(t *testing.T) {
	source := assistedRebuildSourceFixture()
	plan, err := SeedAssistedRebuildPlan(source)
	if err != nil { t.Fatal(err) }
	payload, err := AssistedRebuildPlanCanonicalBytes(plan, source)
	if err != nil { t.Fatal(err) }
	decoded, err := DecodeCanonicalAssistedRebuildPlan(payload, source)
	if err != nil { t.Fatal(err) }
	if len(decoded.Steps) != len(plan.Steps) ||
		decoded.SourceSnapshotSHA256 != plan.SourceSnapshotSHA256 {
		t.Fatalf("decoded=%+v plan=%+v", decoded, plan)
	}
}
