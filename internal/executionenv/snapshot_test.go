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
