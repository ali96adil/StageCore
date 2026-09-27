package executionenv

import (
	"bytes"
	"strings"
	"testing"
)

func int64ptr(value int64) *int64 { return &value }

func validSnapshot() Snapshot {
	return Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		EnvironmentKey: "video-primary",
		AdapterKey: "stagecore.adapter.vdmx",
		SourceManifestSHA256: strings.Repeat("a", 64),
		CaptureStatus: SnapshotPartial,
		Items: []SnapshotItem{
			{Key: "workspace", Name: "Workspace template", Kind: SnapshotTemplate, Provenance: ProvenanceApplicationExport, Capture: ItemCaptured, Portability: SnapshotContentBound, ContentHash: strings.Repeat("b", 64), SizeBytes: int64ptr(42)},
			{Key: "oscquery", Name: "Published controls", Kind: SnapshotControlNamespace, Provenance: ProvenanceOSCQuery, Capture: ItemObserved, Portability: SnapshotDescriptiveOnly, Notes: "Published namespace only; not a project substitute."},
			{Key: "media-bin", Name: "Media bin export", Kind: SnapshotProjectExport, Provenance: ProvenanceApplicationExport, Capture: ItemMissing, Portability: SnapshotReferenceOnly, Locator: "/Users/operator/Show/media-bin.json"},
		},
	}
}

func TestSnapshotCanonicalIdentityIsDeterministic(t *testing.T) {
	left := validSnapshot()
	right := validSnapshot()
	right.Items[0], right.Items[2] = right.Items[2], right.Items[0]
	leftBytes, err := SnapshotCanonicalBytes(left)
	if err != nil { t.Fatal(err) }
	rightBytes, err := SnapshotCanonicalBytes(right)
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(leftBytes, rightBytes) { t.Fatalf("canonical bytes differ\n%s\n%s", leftBytes, rightBytes) }
	leftHash, _ := SnapshotContentHash(left)
	rightHash, _ := SnapshotContentHash(right)
	if leftHash != rightHash { t.Fatalf("hash mismatch: %s != %s", leftHash, rightHash) }
	if _, err := DecodeCanonicalSnapshot(leftBytes); err != nil { t.Fatalf("decode canonical: %v", err) }
}

func TestSnapshotRebuildMetadataIsCanonicalAndPreserved(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.RebuildPlanVersion = SnapshotRebuildPlanVersion
	snapshot.ReconstructionFingerprint = strings.Repeat("C", 64)
	snapshot.RebuildPlan = []SnapshotRebuildStep{
		{
			Step: 1, Action: "OPEN_VDMX_APPLICATION", Status: "SUPPORTED",
			ProvenanceClass: SnapshotProvenanceObserved,
			Notes: "Open the observed application.",
		},
		{
			Step: 2, Action: "CREATE_UNSAVED_WORKSPACE", Status: "MANUAL",
			ProvenanceClass: SnapshotProvenanceUserDeclared,
		},
	}
	snapshot.Items[1].ProvenanceClass = SnapshotProvenanceObserved

	canonical, err := SnapshotCanonicalBytes(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalSnapshot(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.RebuildPlanVersion != SnapshotRebuildPlanVersion ||
		decoded.ReconstructionFingerprint != strings.Repeat("c", 64) ||
		len(decoded.RebuildPlan) != 2 ||
		decoded.RebuildPlan[1].ProvenanceClass != SnapshotProvenanceUserDeclared ||
		decoded.Items[1].ProvenanceClass != SnapshotProvenanceObserved {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestSnapshotRejectsInvalidRebuildMetadata(t *testing.T) {
	cases := []func(*Snapshot){
		func(s *Snapshot) {
			s.RebuildPlanVersion = SnapshotRebuildPlanVersion
			s.ReconstructionFingerprint = strings.Repeat("a", 64)
		},
		func(s *Snapshot) {
			s.RebuildPlanVersion = SnapshotRebuildPlanVersion
			s.ReconstructionFingerprint = "not-a-digest"
			s.RebuildPlan = []SnapshotRebuildStep{{
				Step: 1, Action: "OPEN", Status: "SUPPORTED",
				ProvenanceClass: SnapshotProvenanceObserved,
			}}
		},
		func(s *Snapshot) {
			s.RebuildPlanVersion = SnapshotRebuildPlanVersion
			s.ReconstructionFingerprint = strings.Repeat("a", 64)
			s.RebuildPlan = []SnapshotRebuildStep{{
				Step: 2, Action: "OPEN", Status: "SUPPORTED",
				ProvenanceClass: SnapshotProvenanceObserved,
			}}
		},
		func(s *Snapshot) {
			s.RebuildPlanVersion = SnapshotRebuildPlanVersion
			s.ReconstructionFingerprint = strings.Repeat("a", 64)
			s.RebuildPlan = []SnapshotRebuildStep{{
				Step: 1, Action: "OPEN", Status: "SUPPORTED",
				ProvenanceClass: "INVENTED",
			}}
		},
	}
	for i, mutate := range cases {
		snapshot := validSnapshot()
		mutate(&snapshot)
		if _, err := NormalizeSnapshot(snapshot); err == nil {
			t.Fatalf("case %d unexpectedly accepted", i)
		}
	}
}

func TestSnapshotRejectsFalseContentBoundClaims(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.Items[0].Capture = ItemObserved
	if _, err := NormalizeSnapshot(snapshot); err == nil { t.Fatal("expected CONTENT_BOUND observed item to be rejected") }
	snapshot = validSnapshot()
	snapshot.Items[0].SizeBytes = nil
	if _, err := NormalizeSnapshot(snapshot); err == nil { t.Fatal("expected missing size to be rejected") }
}

func TestSnapshotCompleteRequiresCapturedItems(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.CaptureStatus = SnapshotComplete
	if _, err := NormalizeSnapshot(snapshot); err == nil { t.Fatal("expected non-captured item in COMPLETE snapshot to fail") }
}

func TestSnapshotUnsupportedCannotPretendPartialState(t *testing.T) {
	snapshot := validSnapshot()
	snapshot.CaptureStatus = SnapshotUnsupported
	if _, err := NormalizeSnapshot(snapshot); err == nil { t.Fatal("expected UNSUPPORTED snapshot with items to fail") }
}

func TestDecodeCanonicalSnapshotRejectsNonCanonicalBytes(t *testing.T) {
	payload, err := SnapshotCanonicalBytes(validSnapshot())
	if err != nil { t.Fatal(err) }
	payload = append([]byte(" "), payload...)
	if _, err := DecodeCanonicalSnapshot(payload); err == nil { t.Fatal("expected non-canonical whitespace to be rejected") }
}
