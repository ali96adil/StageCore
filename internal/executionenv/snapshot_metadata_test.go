package executionenv

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotItemMetadataCanonicalizesAndRoundTrips(t *testing.T) {
	snapshot := Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		EnvironmentKey: "video-main",
		AdapterKey: "stagecore.adapter.vdmx",
		SourceManifestSHA256: strings.Repeat("a", 64),
		CaptureStatus: SnapshotPartial,
		Items: []SnapshotItem{{
			Key: "vdmx-oscquery",
			Name: "VDMX published OSCQuery namespace",
			Kind: SnapshotControlNamespace,
			Provenance: ProvenanceOSCQuery,
			Capture: ItemObserved,
			Portability: SnapshotDescriptiveOnly,
			Metadata: json.RawMessage(`{"namespace":{"VALUE":[0.5],"FULL_PATH":"/fader"},"endpoint":"http://127.0.0.1:8080/"}`),
		}},
	}

	canonical, err := SnapshotCanonicalBytes(snapshot)
	if err != nil {
		t.Fatalf("canonical snapshot: %v", err)
	}
	decoded, err := DecodeCanonicalSnapshot(canonical)
	if err != nil {
		t.Fatalf("decode canonical snapshot: %v", err)
	}
	if len(decoded.Items) != 1 || len(decoded.Items[0].Metadata) == 0 {
		t.Fatalf("metadata missing after round trip: %#v", decoded.Items)
	}
	want := []byte(`{"endpoint":"http://127.0.0.1:8080/","namespace":{"FULL_PATH":"/fader","VALUE":[0.5]}}`)
	if !bytes.Equal(decoded.Items[0].Metadata, want) {
		t.Fatalf("metadata not canonical\nwant: %s\n got: %s", want, decoded.Items[0].Metadata)
	}
}

func TestSnapshotItemMetadataRejectsOversizeAndNonObject(t *testing.T) {
	base := Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		EnvironmentKey: "video-main",
		AdapterKey: "stagecore.adapter.vdmx",
		SourceManifestSHA256: strings.Repeat("a", 64),
		CaptureStatus: SnapshotPartial,
		Items: []SnapshotItem{{
			Key: "vdmx-oscquery",
			Name: "VDMX published OSCQuery namespace",
			Kind: SnapshotControlNamespace,
			Provenance: ProvenanceOSCQuery,
			Capture: ItemObserved,
			Portability: SnapshotDescriptiveOnly,
		}},
	}

	nonObject := base
	nonObject.Items = append([]SnapshotItem(nil), base.Items...)
	nonObject.Items[0].Metadata = json.RawMessage(`["not","an","object"]`)
	if _, err := SnapshotCanonicalBytes(nonObject); err == nil {
		t.Fatal("expected non-object metadata rejection")
	}

	oversize := base
	oversize.Items = append([]SnapshotItem(nil), base.Items...)
	oversize.Items[0].Metadata = json.RawMessage(`{"blob":"` + strings.Repeat("x", maxSnapshotItemMetadataBytes) + `"}`)
	if _, err := SnapshotCanonicalBytes(oversize); err == nil {
		t.Fatal("expected oversized metadata rejection")
	}
}
