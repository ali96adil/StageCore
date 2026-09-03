package showcapsule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/extension"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
)

const (
	ManifestSchemaVersion = 1
	ManifestFileName      = "show-capsule.json"
	ManifestChecksumFile  = "show-capsule.sha256"
)

type ExportMode string

const (
	ExportManifestOnly  ExportMode = "MANIFEST_ONLY"
	ExportSelfContained ExportMode = "SELF_CONTAINED"
)

type Manifest struct {
	SchemaVersion         int                          `json:"schema_version"`
	CapsuleID             string                       `json:"capsule_id"`
	ExportMode            ExportMode                   `json:"export_mode"`
	CreatedAt             time.Time                    `json:"created_at"`
	Project               ProjectIdentity              `json:"project"`
	RuntimeSnapshot       RuntimeSnapshotIdentity      `json:"runtime_snapshot"`
	MachineRoles          []MachineRoleIdentity        `json:"machine_roles,omitempty"`
	OperatorMetadata      OperatorMetadata             `json:"operator_metadata"`
	Media                 []MediaRequirement           `json:"media,omitempty"`
	ExecutionEnvironments []ExecutionEnvironmentRecord `json:"execution_environments,omitempty"`
	Extensions            []ExtensionRequirement       `json:"extensions,omitempty"`
	Objects               []ObjectEntry                `json:"objects,omitempty"`
	Presentation          PresentationPortability      `json:"presentation"`
	Warnings              []string                     `json:"warnings,omitempty"`
}

type ProjectIdentity struct {
	ProjectID      string `json:"project_id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	RevisionID     string `json:"revision_id"`
	RevisionNumber int64  `json:"revision_number"`
}

type RuntimeSnapshotIdentity struct {
	RuntimeSnapshotID string            `json:"runtime_snapshot_id"`
	SnapshotVersion   int64             `json:"snapshot_version"`
	ContentSHA256     string            `json:"content_sha256"`
	Manifest          snapshot.Manifest `json:"manifest"`
}

type MachineRoleIdentity struct {
	MachineRoleID             string   `json:"machine_role_id"`
	RoleKey                   string   `json:"role_key"`
	DisplayName               string   `json:"display_name"`
	RequiredCapabilities      []string `json:"required_capabilities,omitempty"`
	RequiredRuntimeSnapshotID *string  `json:"required_runtime_snapshot_id,omitempty"`
	RequiredConfigHash        string   `json:"required_config_hash,omitempty"`
	Required                  bool     `json:"required"`
}

type OperatorMetadata struct {
	CueNotes []CueNote `json:"cue_notes,omitempty"`
}

type CueNote struct {
	CueID string `json:"cue_id"`
	Text  string `json:"text"`
}

type MediaRequirement struct {
	MachineRoleID    string `json:"machine_role_id"`
	RoleKey          string `json:"role_key"`
	MediaAssetID     string `json:"media_asset_id"`
	ContentVersionID string `json:"content_version_id"`
	AssetPolicy      string `json:"asset_policy"`
	ContentSHA256    string `json:"content_sha256"`
	SizeBytes        int64  `json:"size_bytes"`
	Required         bool   `json:"required"`
	ContentIncluded  bool   `json:"content_included"`
}

type ExecutionEnvironmentRecord struct {
	ManifestID      string                        `json:"manifest_id"`
	MachineRoleID   *string                       `json:"machine_role_id,omitempty"`
	ContentSHA256   string                        `json:"content_sha256"`
	Manifest        executionenv.Manifest         `json:"manifest"`
	LatestSnapshot  *ExecutionEnvironmentSnapshot `json:"latest_snapshot,omitempty"`
}

type ExecutionEnvironmentSnapshot struct {
	SnapshotID    string                `json:"snapshot_id"`
	ContentSHA256 string                `json:"content_sha256"`
	Snapshot      executionenv.Snapshot `json:"snapshot"`
}

type ExtensionRequirement struct {
	PackageID            string                `json:"package_id"`
	ExtensionID          string                `json:"extension_id"`
	Version              string                `json:"version"`
	Kind                 string                `json:"kind"`
	Source               string                `json:"source"`
	ManifestSHA256       string                `json:"manifest_sha256"`
	Manifest             json.RawMessage       `json:"manifest"`
	Software             SoftwarePackageRecord `json:"software"`
	RequiredCapabilities []string              `json:"required_capabilities,omitempty"`
	Reason               string                `json:"reason"`
	ContentIncluded      bool                  `json:"content_included"`
}

type SoftwarePackageRecord struct {
	ProductID          string `json:"product_id"`
	Version            string `json:"version"`
	Platform           string `json:"platform"`
	Architecture       string `json:"architecture"`
	MinAPIVersion      int    `json:"min_api_version"`
	MaxAPIVersion      int    `json:"max_api_version"`
	ContentSHA256      string `json:"content_sha256"`
	SizeBytes          int64  `json:"size_bytes"`
	OriginalFilename   string `json:"original_filename,omitempty"`
	SigningStatus      string `json:"signing_status"`
	NotarizationStatus string `json:"notarization_status"`
	ReleaseChannel     string `json:"release_channel"`
}

type ObjectEntry struct {
	ContentSHA256 string   `json:"content_sha256"`
	SizeBytes     int64    `json:"size_bytes"`
	ArchivePath   string   `json:"archive_path,omitempty"`
	Included      bool     `json:"included"`
	Required      bool     `json:"required"`
	Purposes      []string `json:"purposes"`
}

type PresentationPortability struct {
	AppearanceDeviceLocal bool   `json:"appearance_device_local"`
	WorkspaceDeviceLocal  bool   `json:"workspace_device_local"`
	Exported               bool   `json:"exported"`
	Note                   string `json:"note"`
}

func CanonicalBytes(manifest Manifest) ([]byte, error) {
	normalized, err := Normalize(manifest)
	if err != nil {
		return nil, err
	}
	return canonicaljson.Marshal(normalized)
}

func Normalize(manifest Manifest) (Manifest, error) {
	normalized := manifest
	if normalized.SchemaVersion != ManifestSchemaVersion {
		return Manifest{}, fmt.Errorf("show capsule schema_version must be %d", ManifestSchemaVersion)
	}
	if err := stageid.ValidateCanonical(strings.TrimSpace(normalized.CapsuleID)); err != nil {
		return Manifest{}, fmt.Errorf("invalid capsule_id: %w", err)
	}
	normalized.CapsuleID = strings.TrimSpace(normalized.CapsuleID)
	switch normalized.ExportMode {
	case ExportManifestOnly, ExportSelfContained:
	default:
		return Manifest{}, fmt.Errorf("unsupported show capsule export_mode %q", normalized.ExportMode)
	}
	if normalized.CreatedAt.IsZero() {
		return Manifest{}, fmt.Errorf("show capsule created_at is required")
	}
	normalized.CreatedAt = normalized.CreatedAt.UTC()
	if err := validateProjectAndSnapshot(&normalized); err != nil {
		return Manifest{}, err
	}

	normalized.MachineRoles = append([]MachineRoleIdentity(nil), manifest.MachineRoles...)
	for i := range normalized.MachineRoles {
		role := &normalized.MachineRoles[i]
		if err := stageid.ValidateCanonical(strings.TrimSpace(role.MachineRoleID)); err != nil {
			return Manifest{}, fmt.Errorf("machine_roles[%d] invalid machine_role_id: %w", i, err)
		}
		role.RoleKey = strings.TrimSpace(role.RoleKey)
		if role.RoleKey == "" {
			return Manifest{}, fmt.Errorf("machine_roles[%d] role_key is required", i)
		}
		role.RequiredCapabilities = normalizeStringSet(role.RequiredCapabilities)
	}
	sort.Slice(normalized.MachineRoles, func(i, j int) bool {
		if normalized.MachineRoles[i].RoleKey == normalized.MachineRoles[j].RoleKey {
			return normalized.MachineRoles[i].MachineRoleID < normalized.MachineRoles[j].MachineRoleID
		}
		return normalized.MachineRoles[i].RoleKey < normalized.MachineRoles[j].RoleKey
	})

	normalized.OperatorMetadata.CueNotes = append([]CueNote(nil), manifest.OperatorMetadata.CueNotes...)
	for i := range normalized.OperatorMetadata.CueNotes {
		note := &normalized.OperatorMetadata.CueNotes[i]
		if strings.TrimSpace(note.CueID) == "" {
			return Manifest{}, fmt.Errorf("operator_metadata.cue_notes[%d] cue_id is required", i)
		}
		note.Text = strings.TrimSpace(note.Text)
	}
	sort.Slice(normalized.OperatorMetadata.CueNotes, func(i, j int) bool {
		return normalized.OperatorMetadata.CueNotes[i].CueID < normalized.OperatorMetadata.CueNotes[j].CueID
	})

	normalized.Media = append([]MediaRequirement(nil), manifest.Media...)
	for i := range normalized.Media {
		item := &normalized.Media[i]
		item.AssetPolicy = strings.ToUpper(strings.TrimSpace(item.AssetPolicy))
		item.ContentSHA256 = strings.ToLower(strings.TrimSpace(item.ContentSHA256))
		if !isSHA256(item.ContentSHA256) || item.SizeBytes < 0 {
			return Manifest{}, fmt.Errorf("media[%d] has invalid content identity", i)
		}
		if item.MachineRoleID == "" || item.RoleKey == "" || item.MediaAssetID == "" || item.ContentVersionID == "" {
			return Manifest{}, fmt.Errorf("media[%d] identity is incomplete", i)
		}
		if normalized.ExportMode == ExportSelfContained && item.AssetPolicy != "REFERENCE_ONLY" && !item.ContentIncluded {
			return Manifest{}, fmt.Errorf("media[%d] portable content is not included in SELF_CONTAINED capsule", i)
		}
		if normalized.ExportMode == ExportManifestOnly && item.ContentIncluded {
			return Manifest{}, fmt.Errorf("media[%d] cannot include bytes in MANIFEST_ONLY capsule", i)
		}
	}
	sort.Slice(normalized.Media, func(i, j int) bool {
		if normalized.Media[i].RoleKey != normalized.Media[j].RoleKey {
			return normalized.Media[i].RoleKey < normalized.Media[j].RoleKey
		}
		if normalized.Media[i].MediaAssetID != normalized.Media[j].MediaAssetID {
			return normalized.Media[i].MediaAssetID < normalized.Media[j].MediaAssetID
		}
		return normalized.Media[i].ContentVersionID < normalized.Media[j].ContentVersionID
	})

	normalized.ExecutionEnvironments = append([]ExecutionEnvironmentRecord(nil), manifest.ExecutionEnvironments...)
	for i := range normalized.ExecutionEnvironments {
		env := &normalized.ExecutionEnvironments[i]
		hash, err := executionenv.ContentHash(env.Manifest)
		if err != nil {
			return Manifest{}, fmt.Errorf("execution_environments[%d] manifest: %w", i, err)
		}
		if !strings.EqualFold(hash, env.ContentSHA256) {
			return Manifest{}, fmt.Errorf("execution_environments[%d] manifest hash mismatch", i)
		}
		env.ContentSHA256 = strings.ToLower(hash)
		if env.LatestSnapshot != nil {
			hash, err := executionenv.SnapshotContentHash(env.LatestSnapshot.Snapshot)
			if err != nil {
				return Manifest{}, fmt.Errorf("execution_environments[%d] snapshot: %w", i, err)
			}
			if !strings.EqualFold(hash, env.LatestSnapshot.ContentSHA256) {
				return Manifest{}, fmt.Errorf("execution_environments[%d] snapshot hash mismatch", i)
			}
			env.LatestSnapshot.ContentSHA256 = strings.ToLower(hash)
		}
	}
	sort.Slice(normalized.ExecutionEnvironments, func(i, j int) bool {
		if normalized.ExecutionEnvironments[i].Manifest.EnvironmentKey == normalized.ExecutionEnvironments[j].Manifest.EnvironmentKey {
			return normalized.ExecutionEnvironments[i].ManifestID < normalized.ExecutionEnvironments[j].ManifestID
		}
		return normalized.ExecutionEnvironments[i].Manifest.EnvironmentKey < normalized.ExecutionEnvironments[j].Manifest.EnvironmentKey
	})

	normalized.Extensions = append([]ExtensionRequirement(nil), manifest.Extensions...)
	for i := range normalized.Extensions {
		item := &normalized.Extensions[i]
		parsed, canonical, err := extension.ParseManifest(item.Manifest)
		if err != nil {
			return Manifest{}, fmt.Errorf("extensions[%d] manifest: %w", i, err)
		}
		digest := sha256.Sum256(canonical)
		hash := hex.EncodeToString(digest[:])
		if parsed.ExtensionID != item.ExtensionID || parsed.Version != item.Version || !strings.EqualFold(hash, item.ManifestSHA256) {
			return Manifest{}, fmt.Errorf("extensions[%d] identity or manifest hash mismatch", i)
		}
		item.Manifest = append(json.RawMessage(nil), canonical...)
		item.ManifestSHA256 = hash
		item.RequiredCapabilities = normalizeStringSet(item.RequiredCapabilities)
		item.Software.ContentSHA256 = strings.ToLower(strings.TrimSpace(item.Software.ContentSHA256))
		if !isSHA256(item.Software.ContentSHA256) || item.Software.SizeBytes < 0 {
			return Manifest{}, fmt.Errorf("extensions[%d] software content identity is invalid", i)
		}
		if normalized.ExportMode == ExportSelfContained && !item.ContentIncluded {
			return Manifest{}, fmt.Errorf("extensions[%d] package bytes are not included in SELF_CONTAINED capsule", i)
		}
		if normalized.ExportMode == ExportManifestOnly && item.ContentIncluded {
			return Manifest{}, fmt.Errorf("extensions[%d] cannot include bytes in MANIFEST_ONLY capsule", i)
		}
	}
	sort.Slice(normalized.Extensions, func(i, j int) bool {
		if normalized.Extensions[i].ExtensionID != normalized.Extensions[j].ExtensionID {
			return normalized.Extensions[i].ExtensionID < normalized.Extensions[j].ExtensionID
		}
		if normalized.Extensions[i].Version != normalized.Extensions[j].Version {
			return normalized.Extensions[i].Version < normalized.Extensions[j].Version
		}
		return normalized.Extensions[i].PackageID < normalized.Extensions[j].PackageID
	})

	normalized.Objects = append([]ObjectEntry(nil), manifest.Objects...)
	seenObjects := map[string]struct{}{}
	for i := range normalized.Objects {
		object := &normalized.Objects[i]
		object.ContentSHA256 = strings.ToLower(strings.TrimSpace(object.ContentSHA256))
		if !isSHA256(object.ContentSHA256) || object.SizeBytes < 0 {
			return Manifest{}, fmt.Errorf("objects[%d] content identity is invalid", i)
		}
		if _, exists := seenObjects[object.ContentSHA256]; exists {
			return Manifest{}, fmt.Errorf("duplicate capsule object %s", object.ContentSHA256)
		}
		seenObjects[object.ContentSHA256] = struct{}{}
		object.Purposes = normalizeStringSet(object.Purposes)
		if len(object.Purposes) == 0 {
			return Manifest{}, fmt.Errorf("objects[%d] must declare at least one purpose", i)
		}
		if normalized.ExportMode == ExportSelfContained {
			if !object.Included {
				return Manifest{}, fmt.Errorf("objects[%d] is not included in SELF_CONTAINED capsule", i)
			}
			expectedPath := objectArchivePath(object.ContentSHA256)
			if filepath.ToSlash(object.ArchivePath) != expectedPath {
				return Manifest{}, fmt.Errorf("objects[%d] archive_path is not content-addressed", i)
			}
		} else if object.Included || strings.TrimSpace(object.ArchivePath) != "" {
			return Manifest{}, fmt.Errorf("objects[%d] MANIFEST_ONLY entry must not claim included bytes", i)
		}
	}
	sort.Slice(normalized.Objects, func(i, j int) bool { return normalized.Objects[i].ContentSHA256 < normalized.Objects[j].ContentSHA256 })

	normalized.Presentation.AppearanceDeviceLocal = true
	normalized.Presentation.WorkspaceDeviceLocal = true
	normalized.Presentation.Exported = false
	if strings.TrimSpace(normalized.Presentation.Note) == "" {
		normalized.Presentation.Note = "Appearance and workspace profiles are device-local presentation state and are intentionally not exported by the Show Capsule."
	}
	normalized.Warnings = normalizeStringSet(manifest.Warnings)
	return normalized, nil
}

func validateProjectAndSnapshot(manifest *Manifest) error {
	if err := stageid.ValidateCanonical(strings.TrimSpace(manifest.Project.ProjectID)); err != nil {
		return fmt.Errorf("invalid project_id: %w", err)
	}
	if err := stageid.ValidateCanonical(strings.TrimSpace(manifest.Project.RevisionID)); err != nil {
		return fmt.Errorf("invalid revision_id: %w", err)
	}
	if manifest.Project.RevisionNumber <= 0 || strings.TrimSpace(manifest.Project.Name) == "" {
		return fmt.Errorf("project name and positive revision_number are required")
	}
	if err := stageid.ValidateCanonical(strings.TrimSpace(manifest.RuntimeSnapshot.RuntimeSnapshotID)); err != nil {
		return fmt.Errorf("invalid runtime_snapshot_id: %w", err)
	}
	manifest.RuntimeSnapshot.ContentSHA256 = strings.ToLower(strings.TrimSpace(manifest.RuntimeSnapshot.ContentSHA256))
	if !isSHA256(manifest.RuntimeSnapshot.ContentSHA256) || manifest.RuntimeSnapshot.SnapshotVersion <= 0 {
		return fmt.Errorf("runtime snapshot content identity and positive version are required")
	}
	if manifest.RuntimeSnapshot.Manifest.ProjectID != manifest.Project.ProjectID || manifest.RuntimeSnapshot.Manifest.RevisionID != manifest.Project.RevisionID || manifest.RuntimeSnapshot.Manifest.RevisionNumber != manifest.Project.RevisionNumber {
		return fmt.Errorf("runtime snapshot manifest project/revision identity mismatch")
	}
	raw, err := canonicaljson.Marshal(manifest.RuntimeSnapshot.Manifest)
	if err != nil {
		return fmt.Errorf("canonicalize runtime snapshot manifest: %w", err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != manifest.RuntimeSnapshot.ContentSHA256 {
		return fmt.Errorf("runtime snapshot manifest content hash mismatch")
	}
	return nil
}

func Decode(raw []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode show capsule manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return Manifest{}, fmt.Errorf("show capsule manifest contains trailing JSON")
	}
	return Normalize(manifest)
}

func objectArchivePath(contentHash string) string {
	return filepath.ToSlash(filepath.Join("objects", "sha256", contentHash[:2], contentHash[2:4], contentHash))
}

func normalizeStringSet(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
