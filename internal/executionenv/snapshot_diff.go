package executionenv

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
)

// SnapshotDiff is a deterministic, read-only comparison between two captures
// of the exact same execution-environment manifest identity.
type SnapshotDiff struct {
	Identical      bool               `json:"identical"`
	TopLevelFields []string           `json:"top_level_fields"`
	AddedItems     []string           `json:"added_items"`
	RemovedItems   []string           `json:"removed_items"`
	ChangedItems   []SnapshotItemDiff `json:"changed_items"`
}

// SnapshotItemDiff reports only fields whose normalized values changed.
type SnapshotItemDiff struct {
	Key           string   `json:"key"`
	ChangedFields []string `json:"changed_fields"`
}

// DiffSnapshots compares canonicalized snapshots from the same environment,
// adapter and source manifest. It does not infer compatibility between
// unrelated manifests and never mutates either input.
func DiffSnapshots(before, after Snapshot) (SnapshotDiff, error) {
	left, err := NormalizeSnapshot(before)
	if err != nil {
		return SnapshotDiff{}, fmt.Errorf("normalize before snapshot: %w", err)
	}
	right, err := NormalizeSnapshot(after)
	if err != nil {
		return SnapshotDiff{}, fmt.Errorf("normalize after snapshot: %w", err)
	}
	if left.EnvironmentKey != right.EnvironmentKey ||
		left.AdapterKey != right.AdapterKey ||
		left.SourceManifestSHA256 != right.SourceManifestSHA256 {
		return SnapshotDiff{}, fmt.Errorf("snapshots do not share the same execution-environment manifest identity")
	}

	result := SnapshotDiff{}
	if left.CaptureStatus != right.CaptureStatus {
		result.TopLevelFields = append(result.TopLevelFields, "capture_status")
	}
	if left.RebuildPlanVersion != right.RebuildPlanVersion {
		result.TopLevelFields = append(result.TopLevelFields, "rebuild_plan_version")
	}
	if left.ReconstructionFingerprint != right.ReconstructionFingerprint {
		result.TopLevelFields = append(result.TopLevelFields, "reconstruction_fingerprint")
	}
	if !reflect.DeepEqual(left.RebuildPlan, right.RebuildPlan) {
		result.TopLevelFields = append(result.TopLevelFields, "rebuild_plan")
	}
	if left.Notes != right.Notes {
		result.TopLevelFields = append(result.TopLevelFields, "notes")
	}

	leftItems := make(map[string]SnapshotItem, len(left.Items))
	rightItems := make(map[string]SnapshotItem, len(right.Items))
	for _, item := range left.Items {
		leftItems[item.Key] = item
	}
	for _, item := range right.Items {
		rightItems[item.Key] = item
	}
	for key := range leftItems {
		if _, ok := rightItems[key]; !ok {
			result.RemovedItems = append(result.RemovedItems, key)
		}
	}
	for key, afterItem := range rightItems {
		beforeItem, ok := leftItems[key]
		if !ok {
			result.AddedItems = append(result.AddedItems, key)
			continue
		}
		if fields := diffSnapshotItemFields(beforeItem, afterItem); len(fields) != 0 {
			result.ChangedItems = append(result.ChangedItems, SnapshotItemDiff{
				Key: key, ChangedFields: fields,
			})
		}
	}

	sort.Strings(result.TopLevelFields)
	sort.Strings(result.AddedItems)
	sort.Strings(result.RemovedItems)
	sort.Slice(result.ChangedItems, func(i, j int) bool {
		return result.ChangedItems[i].Key < result.ChangedItems[j].Key
	})
	result.Identical = len(result.TopLevelFields) == 0 &&
		len(result.AddedItems) == 0 &&
		len(result.RemovedItems) == 0 &&
		len(result.ChangedItems) == 0
	return result, nil
}

func diffSnapshotItemFields(before, after SnapshotItem) []string {
	fields := make([]string, 0, 12)
	if before.Name != after.Name {
		fields = append(fields, "name")
	}
	if before.Kind != after.Kind {
		fields = append(fields, "kind")
	}
	if before.Provenance != after.Provenance {
		fields = append(fields, "provenance")
	}
	if before.ProvenanceClass != after.ProvenanceClass {
		fields = append(fields, "provenance_class")
	}
	if before.Capture != after.Capture {
		fields = append(fields, "capture_status")
	}
	if before.Portability != after.Portability {
		fields = append(fields, "portability")
	}
	if before.Locator != after.Locator {
		fields = append(fields, "locator")
	}
	if before.ContentHash != after.ContentHash {
		fields = append(fields, "content_hash")
	}
	if !equalOptionalInt64(before.SizeBytes, after.SizeBytes) {
		fields = append(fields, "size_bytes")
	}
	if before.Notes != after.Notes {
		fields = append(fields, "notes")
	}
	if !bytes.Equal(before.Metadata, after.Metadata) {
		fields = append(fields, "metadata")
	}
	sort.Strings(fields)
	return fields
}

func equalOptionalInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
