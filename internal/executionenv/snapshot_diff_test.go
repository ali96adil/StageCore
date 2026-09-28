package executionenv

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func snapshotDiffFixture() Snapshot {
	return Snapshot{
		SchemaVersion:        SnapshotSchemaVersion,
		EnvironmentKey:       "video-main",
		AdapterKey:           "stagecore.adapter.vdmx",
		SourceManifestSHA256: strings.Repeat("a", 64),
		CaptureStatus:        SnapshotPartial,
		Items: []SnapshotItem{
			{
				Key:             "vdmx-app",
				Name:            "VDMX application",
				Kind:            SnapshotOther,
				Provenance:      ProvenanceAdapterObservation,
				ProvenanceClass: SnapshotProvenanceObserved,
				Capture:         ItemObserved,
				Portability:     SnapshotDescriptiveOnly,
				Metadata:        json.RawMessage(`{"version":"6.0"}`),
			},
			{
				Key:             "vdmx-oscquery",
				Name:            "VDMX OSCQuery",
				Kind:            SnapshotControlNamespace,
				Provenance:      ProvenanceOSCQuery,
				ProvenanceClass: SnapshotProvenanceObserved,
				Capture:         ItemObserved,
				Portability:     SnapshotDescriptiveOnly,
				Metadata:        json.RawMessage(`{"path":"/fader","value":0.5}`),
			},
		},
	}
}

func TestDiffSnapshotsIdenticalAfterNormalization(t *testing.T) {
	before := snapshotDiffFixture()
	after := snapshotDiffFixture()
	after.Items[0], after.Items[1] = after.Items[1], after.Items[0]
	got, err := DiffSnapshots(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Identical ||
		len(got.TopLevelFields) != 0 ||
		len(got.AddedItems) != 0 ||
		len(got.RemovedItems) != 0 ||
		len(got.ChangedItems) != 0 {
		t.Fatalf("diff=%+v", got)
	}
}

func TestDiffSnapshotsReportsFieldLevelChangesDeterministically(t *testing.T) {
	before := snapshotDiffFixture()
	after := snapshotDiffFixture()
	after.CaptureStatus = SnapshotComplete
	after.Notes = "new capture note"
	after.Items[0].Capture = ItemCaptured
	after.Items[1].Capture = ItemCaptured
	after.Items[1].Metadata = json.RawMessage(`{"value":0.75,"path":"/fader"}`)

	got, err := DiffSnapshots(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if got.Identical {
		t.Fatalf("diff unexpectedly identical: %+v", got)
	}
	if !reflect.DeepEqual(got.TopLevelFields, []string{"capture_status", "notes"}) {
		t.Fatalf("top-level=%v", got.TopLevelFields)
	}
	wantChanged := []SnapshotItemDiff{
		{Key: "vdmx-app", ChangedFields: []string{"capture_status"}},
		{Key: "vdmx-oscquery", ChangedFields: []string{"capture_status", "metadata"}},
	}
	if !reflect.DeepEqual(got.ChangedItems, wantChanged) {
		t.Fatalf("changed=%+v want=%+v", got.ChangedItems, wantChanged)
	}
}

func TestDiffSnapshotsReportsAddedAndRemovedItems(t *testing.T) {
	before := snapshotDiffFixture()
	after := snapshotDiffFixture()
	after.Items = []SnapshotItem{
		after.Items[1],
		{
			Key:             "vdmx-output",
			Name:            "VDMX output",
			Kind:            SnapshotOutputNotes,
			Provenance:      ProvenanceAdapterObservation,
			ProvenanceClass: SnapshotProvenanceObserved,
			Capture:         ItemObserved,
			Portability:     SnapshotDescriptiveOnly,
		},
	}
	got, err := DiffSnapshots(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.RemovedItems, []string{"vdmx-app"}) ||
		!reflect.DeepEqual(got.AddedItems, []string{"vdmx-output"}) {
		t.Fatalf("diff=%+v", got)
	}
}

func TestDiffSnapshotsRejectsDifferentManifestIdentity(t *testing.T) {
	before := snapshotDiffFixture()
	after := snapshotDiffFixture()
	after.SourceManifestSHA256 = strings.Repeat("b", 64)
	if _, err := DiffSnapshots(before, after); err == nil {
		t.Fatal("expected manifest identity mismatch")
	}
}

func TestDiffSnapshotsCanonicalizesMetadataBeforeComparison(t *testing.T) {
	before := snapshotDiffFixture()
	after := snapshotDiffFixture()
	before.Items[1].Metadata = json.RawMessage(`{"path":"/fader","value":0.5}`)
	after.Items[1].Metadata = json.RawMessage(`{"value":0.5,"path":"/fader"}`)
	got, err := DiffSnapshots(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Identical {
		t.Fatalf("canonical-equivalent metadata reported changed: %+v", got)
	}
}
