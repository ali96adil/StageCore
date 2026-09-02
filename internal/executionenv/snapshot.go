package executionenv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
)

const (
	SnapshotSchemaVersion       = 1
	maxSnapshotItems            = 512
	maxSnapshotNotesBytes       = 8192
	maxSnapshotItemNotesBytes   = 4096
)

type SnapshotCaptureStatus string

const (
	SnapshotComplete    SnapshotCaptureStatus = "COMPLETE"
	SnapshotPartial     SnapshotCaptureStatus = "PARTIAL"
	SnapshotUnsupported SnapshotCaptureStatus = "UNSUPPORTED"
)

type SnapshotItemKind string

const (
	SnapshotProjectExport      SnapshotItemKind = "PROJECT_EXPORT"
	SnapshotTemplate           SnapshotItemKind = "TEMPLATE"
	SnapshotPreset             SnapshotItemKind = "PRESET"
	SnapshotConfig             SnapshotItemKind = "CONFIG"
	SnapshotResource           SnapshotItemKind = "RESOURCE"
	SnapshotControlNamespace   SnapshotItemKind = "CONTROL_NAMESPACE"
	SnapshotControlState       SnapshotItemKind = "CONTROL_STATE"
	SnapshotExtensionInventory SnapshotItemKind = "EXTENSION_INVENTORY"
	SnapshotOutputNotes        SnapshotItemKind = "OUTPUT_NOTES"
	SnapshotReferenceMaterial  SnapshotItemKind = "REFERENCE_MATERIAL"
	SnapshotOther              SnapshotItemKind = "OTHER"
)

type SnapshotItemProvenance string

const (
	ProvenanceApplicationExport  SnapshotItemProvenance = "APPLICATION_EXPORT"
	ProvenanceFilesystemResource SnapshotItemProvenance = "FILESYSTEM_RESOURCE"
	ProvenanceAdapterObservation SnapshotItemProvenance = "ADAPTER_OBSERVATION"
	ProvenanceOSCQuery           SnapshotItemProvenance = "OSCQUERY"
	ProvenanceOperatorReference  SnapshotItemProvenance = "OPERATOR_REFERENCE"
	ProvenanceOther              SnapshotItemProvenance = "OTHER"
)

type SnapshotItemCaptureStatus string

const (
	ItemCaptured    SnapshotItemCaptureStatus = "CAPTURED"
	ItemObserved    SnapshotItemCaptureStatus = "OBSERVED"
	ItemMissing     SnapshotItemCaptureStatus = "MISSING"
	ItemUnsupported SnapshotItemCaptureStatus = "UNSUPPORTED"
)

type SnapshotPortability string

const (
	SnapshotContentBound    SnapshotPortability = "CONTENT_BOUND"
	SnapshotReferenceOnly   SnapshotPortability = "REFERENCE_ONLY"
	SnapshotDescriptiveOnly SnapshotPortability = "DESCRIPTIVE_ONLY"
)

type Snapshot struct {
	SchemaVersion        int                   `json:"schema_version"`
	EnvironmentKey       string                `json:"environment_key"`
	AdapterKey           string                `json:"adapter_key"`
	SourceManifestSHA256 string                `json:"source_manifest_sha256"`
	CaptureStatus        SnapshotCaptureStatus `json:"capture_status"`
	Items                []SnapshotItem        `json:"items,omitempty"`
	Notes                string                `json:"notes,omitempty"`
}

type SnapshotItem struct {
	Key         string                    `json:"key"`
	Name        string                    `json:"name"`
	Kind        SnapshotItemKind          `json:"kind"`
	Provenance  SnapshotItemProvenance    `json:"provenance"`
	Capture     SnapshotItemCaptureStatus `json:"capture_status"`
	Portability SnapshotPortability       `json:"portability"`
	Locator     string                    `json:"locator,omitempty"`
	ContentHash string                    `json:"content_hash,omitempty"`
	SizeBytes   *int64                    `json:"size_bytes,omitempty"`
	Notes       string                    `json:"notes,omitempty"`
}

func NormalizeSnapshot(snapshot Snapshot) (Snapshot, error) {
	normalized := snapshot
	normalized.Items = append([]SnapshotItem(nil), snapshot.Items...)
	if normalized.SchemaVersion != SnapshotSchemaVersion {
		return Snapshot{}, fmt.Errorf("schema_version must be %d", SnapshotSchemaVersion)
	}
	if err := validateKey("environment_key", normalized.EnvironmentKey); err != nil {
		return Snapshot{}, err
	}
	if err := validateKey("adapter_key", normalized.AdapterKey); err != nil {
		return Snapshot{}, err
	}
	if !isSHA256(normalized.SourceManifestSHA256) {
		return Snapshot{}, fmt.Errorf("source_manifest_sha256 must be a 64-character SHA-256 hex digest")
	}
	normalized.SourceManifestSHA256 = strings.ToLower(normalized.SourceManifestSHA256)
	switch normalized.CaptureStatus {
	case SnapshotComplete, SnapshotPartial, SnapshotUnsupported:
	default:
		return Snapshot{}, fmt.Errorf("unsupported capture_status %q", normalized.CaptureStatus)
	}
	if err := validateText("notes", normalized.Notes, maxSnapshotNotesBytes, false); err != nil {
		return Snapshot{}, err
	}
	if len(normalized.Items) > maxSnapshotItems {
		return Snapshot{}, fmt.Errorf("items exceeds maximum of %d", maxSnapshotItems)
	}
	if normalized.CaptureStatus == SnapshotComplete && len(normalized.Items) == 0 {
		return Snapshot{}, fmt.Errorf("COMPLETE snapshot must contain at least one captured item")
	}
	if normalized.CaptureStatus == SnapshotUnsupported && len(normalized.Items) != 0 {
		return Snapshot{}, fmt.Errorf("UNSUPPORTED snapshot must not contain items")
	}
	seen := make(map[string]struct{}, len(normalized.Items))
	for i := range normalized.Items {
		item := &normalized.Items[i]
		if err := normalizeSnapshotItem(item); err != nil {
			return Snapshot{}, fmt.Errorf("items[%d]: %w", i, err)
		}
		if _, exists := seen[item.Key]; exists {
			return Snapshot{}, fmt.Errorf("duplicate snapshot item key %q", item.Key)
		}
		seen[item.Key] = struct{}{}
		if normalized.CaptureStatus == SnapshotComplete && item.Capture != ItemCaptured {
			return Snapshot{}, fmt.Errorf("COMPLETE snapshot item %q must be CAPTURED", item.Key)
		}
	}
	sort.Slice(normalized.Items, func(i, j int) bool { return normalized.Items[i].Key < normalized.Items[j].Key })
	return normalized, nil
}

func normalizeSnapshotItem(item *SnapshotItem) error {
	if err := validateKey("snapshot_item.key", item.Key); err != nil {
		return err
	}
	if err := validateText("snapshot_item.name", item.Name, maxNameBytes, true); err != nil {
		return err
	}
	switch item.Kind {
	case SnapshotProjectExport, SnapshotTemplate, SnapshotPreset, SnapshotConfig, SnapshotResource,
		SnapshotControlNamespace, SnapshotControlState, SnapshotExtensionInventory, SnapshotOutputNotes,
		SnapshotReferenceMaterial, SnapshotOther:
	default:
		return fmt.Errorf("snapshot item %q has unsupported kind %q", item.Key, item.Kind)
	}
	switch item.Provenance {
	case ProvenanceApplicationExport, ProvenanceFilesystemResource, ProvenanceAdapterObservation,
		ProvenanceOSCQuery, ProvenanceOperatorReference, ProvenanceOther:
	default:
		return fmt.Errorf("snapshot item %q has unsupported provenance %q", item.Key, item.Provenance)
	}
	switch item.Capture {
	case ItemCaptured, ItemObserved, ItemMissing, ItemUnsupported:
	default:
		return fmt.Errorf("snapshot item %q has unsupported capture_status %q", item.Key, item.Capture)
	}
	if err := validateText("snapshot_item.notes", item.Notes, maxSnapshotItemNotesBytes, false); err != nil {
		return err
	}
	switch item.Portability {
	case SnapshotContentBound:
		if item.Capture != ItemCaptured {
			return fmt.Errorf("snapshot item %q CONTENT_BOUND must be CAPTURED", item.Key)
		}
		if !isSHA256(item.ContentHash) {
			return fmt.Errorf("snapshot item %q content_hash must be a 64-character SHA-256 hex digest", item.Key)
		}
		item.ContentHash = strings.ToLower(item.ContentHash)
		if item.SizeBytes == nil || *item.SizeBytes < 0 {
			return fmt.Errorf("snapshot item %q size_bytes is required and must be non-negative for CONTENT_BOUND", item.Key)
		}
		if item.Locator != "" {
			return fmt.Errorf("snapshot item %q CONTENT_BOUND must not rely on locator", item.Key)
		}
	case SnapshotReferenceOnly:
		if item.ContentHash != "" || item.SizeBytes != nil {
			return fmt.Errorf("snapshot item %q REFERENCE_ONLY must not claim content identity", item.Key)
		}
		if err := validateLocator("snapshot_item.locator", item.Locator); err != nil {
			return err
		}
	case SnapshotDescriptiveOnly:
		if item.ContentHash != "" || item.SizeBytes != nil || item.Locator != "" {
			return fmt.Errorf("snapshot item %q DESCRIPTIVE_ONLY must not claim bytes or locator", item.Key)
		}
	default:
		return fmt.Errorf("snapshot item %q has unsupported portability %q", item.Key, item.Portability)
	}
	return nil
}

func SnapshotCanonicalBytes(snapshot Snapshot) ([]byte, error) {
	normalized, err := NormalizeSnapshot(snapshot)
	if err != nil {
		return nil, err
	}
	return canonicaljson.Marshal(normalized)
}

func SnapshotContentHash(snapshot Snapshot) (string, error) {
	payload, err := SnapshotCanonicalBytes(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func DecodeCanonicalSnapshot(payload []byte) (Snapshot, error) {
	if len(payload) == 0 {
		return Snapshot{}, fmt.Errorf("execution environment snapshot is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode execution environment snapshot: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return Snapshot{}, fmt.Errorf("decode execution environment snapshot trailing data: %w", err)
		}
		return Snapshot{}, fmt.Errorf("execution environment snapshot contains trailing JSON data")
	}
	normalized, err := NormalizeSnapshot(snapshot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("validate execution environment snapshot: %w", err)
	}
	canonical, err := SnapshotCanonicalBytes(normalized)
	if err != nil {
		return Snapshot{}, err
	}
	if !bytes.Equal(payload, canonical) {
		return Snapshot{}, fmt.Errorf("execution environment snapshot bytes are not canonical")
	}
	return normalized, nil
}
