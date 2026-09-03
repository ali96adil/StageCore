package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

type PortableProjectImport struct {
	ProjectID          string
	Name               string
	Description        string
	RevisionID         string
	RevisionNumber     int64
	RuntimeSnapshot    PortableRuntimeSnapshotImport
	Aliases            []PortableAliasImport
	Cues               []PortableCueImport
	Inputs             []PortableInputImport
	Outputs            []PortableOutputImport
	Routes             []PortableRouteImport
	MachineRoles       []PortableMachineRoleImport
	Media              []PortableMediaImport
	ExecutionEnvironments []PortableExecutionEnvironmentImport
	Extensions         []PortableExtensionImport
	ImportedBy         string
}

type PortableRuntimeSnapshotImport struct {
	ID           string
	Version      int64
	ContentHash  string
	ManifestJSON json.RawMessage
}

type PortableAliasImport struct {
	ID            string
	LogicalName   string
	LogicalType   string
	TargetRef     string
	GroupName     string
	Configuration json.RawMessage
}

type PortableCueImport struct {
	ID              string
	DisplayLabel    string
	Name            string
	OrderIndex      int
	CueType         string
	Criticality     string
	Enabled         bool
	ExecutionPolicy json.RawMessage
	NotesSummary    string
	Actions         []PortableActionImport
}

type PortableActionImport struct {
	ID            string
	OrderIndex    int
	ExecutionMode string
	TargetRef     string
	CapabilityKey string
	Parameters    json.RawMessage
	TimeoutPolicy json.RawMessage
	ErrorPolicy   json.RawMessage
	PriorityClass string
	Enabled       bool
}

type PortableInputImport struct {
	ID          string
	Name        string
	SourceRef   string
	EventType   string
	ValueSchema json.RawMessage
	Enabled     bool
}

type PortableOutputImport struct {
	ID            string
	Name          string
	TargetRef     string
	CapabilityKey string
	ValueSchema   json.RawMessage
	Criticality   string
}

type PortableRouteImport struct {
	ID                  string
	Name                string
	InputID             string
	ConditionDefinition json.RawMessage
	TransformDefinition json.RawMessage
	DelayMS             *int64
	DebounceMS          *int64
	PriorityClass       string
	ErrorPolicy         json.RawMessage
	Enabled             bool
	Actions             []PortableRouteActionImport
}

type PortableRouteActionImport struct {
	ID         string
	OrderIndex int
	OutputID   *string
	CueID      *string
	Parameters json.RawMessage
}

type PortableMachineRoleImport struct {
	ID                        string
	RoleKey                   string
	DisplayName               string
	RequiredCapabilities      []string
	RequiredRuntimeSnapshotID *string
	RequiredConfigHash        string
	Required                  bool
}

type PortableMediaImport struct {
	MachineRoleID    string
	MediaAssetID     string
	ContentVersionID string
	Name             string
	AssetPolicy      string
	ContentHash      string
	SizeBytes        int64
	OriginalFilename string
	Required         bool
}

type PortableExecutionEnvironmentImport struct {
	ManifestID       string
	MachineRoleID    *string
	ContentHash      string
	Manifest         executionenv.Manifest
	SnapshotID       string
	SnapshotHash     string
	Snapshot         *executionenv.Snapshot
}

type PortableExtensionImport struct {
	PackageID            string
	ProductID            string
	SoftwareVersion      string
	Platform             string
	Architecture         string
	MinAPIVersion        int
	MaxAPIVersion        int
	ContentHash          string
	SizeBytes            int64
	OriginalFilename     string
	SigningStatus        string
	NotarizationStatus   string
	ReleaseChannel       string
	ExtensionID          string
	ExtensionVersion     string
	Kind                 string
	Source               string
	ManifestJSON         json.RawMessage
	ManifestSHA256       string
}

type PortableProjectImportResult struct {
	ProjectID       string
	RevisionID      string
	RuntimeSnapshotID string
	ImportedExtensions []string
}

// ImportPortableProject materializes one already-verified Show Capsule graph in
// one SQLite transaction. Project/revision/runtime identities are preserved so
// the immutable Runtime Snapshot hash remains the authority after transfer.
// It never overwrites an existing identity and it deliberately does not create
// extension installations, permission reviews, role assignments or sessions.
func (s *Store) ImportPortableProject(ctx context.Context, p PortableProjectImport) (PortableProjectImportResult, error) {
	if s == nil || s.db == nil {
		return PortableProjectImportResult{}, fmt.Errorf("Store is unavailable")
	}
	p.ProjectID = strings.TrimSpace(p.ProjectID)
	p.RevisionID = strings.TrimSpace(p.RevisionID)
	p.RuntimeSnapshot.ID = strings.TrimSpace(p.RuntimeSnapshot.ID)
	p.ImportedBy = strings.TrimSpace(p.ImportedBy)
	if p.ProjectID == "" || p.RevisionID == "" || p.RuntimeSnapshot.ID == "" || p.ImportedBy == "" {
		return PortableProjectImportResult{}, fmt.Errorf("%w: portable Project identity and import actor are required", domain.ErrInvalidInput)
	}
	if p.RevisionNumber <= 0 || p.RuntimeSnapshot.Version <= 0 || len(strings.TrimSpace(p.RuntimeSnapshot.ContentHash)) != 64 || !json.Valid(p.RuntimeSnapshot.ManifestJSON) {
		return PortableProjectImportResult{}, fmt.Errorf("%w: portable revision/runtime snapshot identity is invalid", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(p.Name) == "" {
		return PortableProjectImportResult{}, fmt.Errorf("%w: portable Project name is required", domain.ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PortableProjectImportResult{}, fmt.Errorf("begin portable Project import: %w", err)
	}
	defer tx.Rollback()

	var activeShows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE status = 'ACTIVE' AND session_type = 'SHOW'`).Scan(&activeShows); err != nil {
		return PortableProjectImportResult{}, fmt.Errorf("check active SHOW before portable import: %w", err)
	}
	if activeShows != 0 {
		return PortableProjectImportResult{}, fmt.Errorf("%w: portable Project import is blocked while SHOW is active", domain.ErrShowConfigurationLocked)
	}
	if err := ensurePortablePrimaryIdentitiesAvailable(ctx, tx, p); err != nil {
		return PortableProjectImportResult{}, err
	}

	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO projects (project_id, name, description, lifecycle_state, current_revision_id, created_at_us, updated_at_us)
		VALUES (?, ?, ?, 'ACTIVE', NULL, ?, ?)`, p.ProjectID, p.Name, p.Description, nowUS, nowUS); err != nil {
		return PortableProjectImportResult{}, fmt.Errorf("insert portable Project: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_revisions (revision_id, project_id, revision_number, status, parent_revision_id, created_at_us, created_by, change_note)
		VALUES (?, ?, ?, 'VALIDATED', NULL, ?, ?, 'Imported from verified Show Capsule')`,
		p.RevisionID, p.ProjectID, p.RevisionNumber, nowUS, p.ImportedBy); err != nil {
		return PortableProjectImportResult{}, fmt.Errorf("insert portable revision: %w", err)
	}

	for _, alias := range p.Aliases {
		cfg := jsonOrDefault(alias.Configuration, `{}`)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_device_aliases (alias_id, project_id, logical_name, logical_type, target_ref, group_name, project_config_json)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, alias.ID, p.ProjectID, alias.LogicalName, alias.LogicalType, alias.TargetRef, alias.GroupName, cfg); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable alias %s: %w", alias.ID, err)
		}
	}
	for _, cue := range p.Cues {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cues (cue_id, revision_id, display_label, name, order_index, cue_type, criticality, enabled, execution_policy_json, notes_summary)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, cue.ID, p.RevisionID, cue.DisplayLabel, cue.Name, cue.OrderIndex, cue.CueType, cue.Criticality, boolInt(cue.Enabled), jsonOrDefault(cue.ExecutionPolicy, `{}`), cue.NotesSummary); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable cue %s: %w", cue.ID, err)
		}
		for _, action := range cue.Actions {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO actions (action_id, cue_id, order_index, execution_mode, target_ref, capability_key, parameters_json, timeout_policy_json, error_policy_json, priority_class, enabled)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, action.ID, cue.ID, action.OrderIndex, action.ExecutionMode, action.TargetRef, action.CapabilityKey, jsonOrDefault(action.Parameters, `{}`), jsonOrDefault(action.TimeoutPolicy, `{}`), jsonOrDefault(action.ErrorPolicy, `{}`), action.PriorityClass, boolInt(action.Enabled)); err != nil {
				return PortableProjectImportResult{}, fmt.Errorf("insert portable action %s: %w", action.ID, err)
			}
		}
	}
	for _, input := range p.Inputs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO input_definitions (input_id, revision_id, name, source_ref, event_type, value_schema_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, input.ID, p.RevisionID, input.Name, input.SourceRef, input.EventType, jsonOrDefault(input.ValueSchema, `{}`), boolInt(input.Enabled)); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable input %s: %w", input.ID, err)
		}
	}
	for _, output := range p.Outputs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO output_definitions (output_id, revision_id, name, target_ref, capability_key, value_schema_json, criticality)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, output.ID, p.RevisionID, output.Name, output.TargetRef, output.CapabilityKey, jsonOrDefault(output.ValueSchema, `{}`), output.Criticality); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable output %s: %w", output.ID, err)
		}
	}
	for _, route := range p.Routes {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO routes (route_id, revision_id, name, input_id, condition_definition_json, transform_definition_json, delay_ms, debounce_ms, priority_class, error_policy_json, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, route.ID, p.RevisionID, route.Name, route.InputID, jsonOrDefault(route.ConditionDefinition, `null`), jsonOrDefault(route.TransformDefinition, `null`), route.DelayMS, route.DebounceMS, route.PriorityClass, jsonOrDefault(route.ErrorPolicy, `{}`), boolInt(route.Enabled)); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable route %s: %w", route.ID, err)
		}
		for _, action := range route.Actions {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO route_actions (route_action_id, route_id, order_index, output_id, cue_id, parameters_json)
				VALUES (?, ?, ?, ?, ?, ?)`, action.ID, route.ID, action.OrderIndex, action.OutputID, action.CueID, jsonOrDefault(action.Parameters, `{}`)); err != nil {
				return PortableProjectImportResult{}, fmt.Errorf("insert portable route action %s: %w", action.ID, err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runtime_snapshots (runtime_snapshot_id, project_id, revision_id, snapshot_version, created_at_us, created_by, content_hash, manifest_json, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'PUBLISHED')`,
		p.RuntimeSnapshot.ID, p.ProjectID, p.RevisionID, p.RuntimeSnapshot.Version, nowUS, p.ImportedBy,
		strings.ToLower(strings.TrimSpace(p.RuntimeSnapshot.ContentHash)), string(p.RuntimeSnapshot.ManifestJSON)); err != nil {
		return PortableProjectImportResult{}, fmt.Errorf("insert portable Runtime Snapshot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET current_revision_id = ? WHERE project_id = ?`, p.RevisionID, p.ProjectID); err != nil {
		return PortableProjectImportResult{}, fmt.Errorf("bind portable current revision: %w", err)
	}

	for _, role := range p.MachineRoles {
		capabilities, err := json.Marshal(normalizeStringList(role.RequiredCapabilities))
		if err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("encode portable role capabilities: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO machine_roles (machine_role_id, project_id, role_key, display_name, required_capabilities_json, required_runtime_snapshot_id, required_config_hash, required, created_at_us, updated_at_us)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, role.ID, p.ProjectID, role.RoleKey, role.DisplayName, string(capabilities), role.RequiredRuntimeSnapshotID, role.RequiredConfigHash, boolInt(role.Required), nowUS, nowUS); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable Machine Role %s: %w", role.ID, err)
		}
	}

	for _, media := range p.Media {
		var objectSize int64
		if err := tx.QueryRowContext(ctx, `SELECT size_bytes FROM vault_objects WHERE content_hash = ?`, strings.ToLower(strings.TrimSpace(media.ContentHash))).Scan(&objectSize); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("portable media %s Vault object is unavailable: %w", media.MediaAssetID, err)
		}
		if objectSize != media.SizeBytes {
			return PortableProjectImportResult{}, fmt.Errorf("%w: portable media %s Vault size mismatch", domain.ErrConflict, media.MediaAssetID)
		}
		name := strings.TrimSpace(media.Name)
		if name == "" {
			name = media.MediaAssetID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_assets (media_asset_id, project_id, name, asset_policy, created_at_us, updated_at_us)
			VALUES (?, ?, ?, ?, ?, ?)`, media.MediaAssetID, p.ProjectID, name, media.AssetPolicy, nowUS, nowUS); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable media asset %s: %w", media.MediaAssetID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_content_versions (content_version_id, media_asset_id, content_hash, original_filename, size_bytes, created_at_us)
			VALUES (?, ?, ?, ?, ?, ?)`, media.ContentVersionID, media.MediaAssetID, strings.ToLower(strings.TrimSpace(media.ContentHash)), media.OriginalFilename, media.SizeBytes, nowUS); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable media version %s: %w", media.ContentVersionID, err)
		}
		locationID, err := stageid.New()
		if err != nil {
			return PortableProjectImportResult{}, err
		}
		object, err := portableVaultObjectTx(ctx, tx, media.ContentHash)
		if err != nil {
			return PortableProjectImportResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_locations (media_location_id, content_version_id, location_type, locator, status, verified_at_us)
			VALUES (?, ?, 'HUB', ?, 'AVAILABLE', ?)`, locationID, media.ContentVersionID, object.RelativePath, nowUS); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable media location: %w", err)
		}
		requirementID, err := stageid.New()
		if err != nil {
			return PortableProjectImportResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO machine_role_media_requirements (media_requirement_id, machine_role_id, content_version_id, required, created_at_us)
			VALUES (?, ?, ?, ?, ?)`, requirementID, media.MachineRoleID, media.ContentVersionID, boolInt(media.Required), nowUS); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable media requirement: %w", err)
		}
	}

	for _, environment := range p.ExecutionEnvironments {
		manifestJSON, err := executionenv.CanonicalBytes(environment.Manifest)
		if err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("canonicalize portable execution environment: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO execution_environment_manifests (environment_manifest_id, revision_id, environment_key, adapter_key, application_key, manifest_json, content_sha256, created_by, created_at_us, machine_role_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, environment.ManifestID, p.RevisionID, environment.Manifest.EnvironmentKey, environment.Manifest.AdapterKey, environment.Manifest.Application.Key, string(manifestJSON), strings.ToLower(environment.ContentHash), p.ImportedBy, nowUS, environment.MachineRoleID); err != nil {
			return PortableProjectImportResult{}, fmt.Errorf("insert portable execution environment %s: %w", environment.ManifestID, err)
		}
		if environment.Snapshot != nil {
			snapshotJSON, err := executionenv.SnapshotCanonicalBytes(*environment.Snapshot)
			if err != nil {
				return PortableProjectImportResult{}, fmt.Errorf("canonicalize portable execution environment snapshot: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO execution_environment_snapshots (environment_snapshot_id, environment_manifest_id, revision_id, source_manifest_sha256, snapshot_json, content_sha256, created_by, created_at_us)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, environment.SnapshotID, environment.ManifestID, p.RevisionID, strings.ToLower(environment.ContentHash), string(snapshotJSON), strings.ToLower(environment.SnapshotHash), p.ImportedBy, nowUS); err != nil {
				return PortableProjectImportResult{}, fmt.Errorf("insert portable execution environment snapshot %s: %w", environment.SnapshotID, err)
			}
		}
	}

	importedExtensions := make([]string, 0, len(p.Extensions))
	for _, extension := range p.Extensions {
		if err := importPortableExtensionTx(ctx, tx, extension, p.ImportedBy, nowUS); err != nil {
			return PortableProjectImportResult{}, err
		}
		importedExtensions = append(importedExtensions, extension.ExtensionID)
	}

	if err := tx.Commit(); err != nil {
		return PortableProjectImportResult{}, fmt.Errorf("commit portable Project import: %w", err)
	}
	return PortableProjectImportResult{
		ProjectID: p.ProjectID,
		RevisionID: p.RevisionID,
		RuntimeSnapshotID: p.RuntimeSnapshot.ID,
		ImportedExtensions: importedExtensions,
	}, nil
}

func ensurePortablePrimaryIdentitiesAvailable(ctx context.Context, tx *sql.Tx, p PortableProjectImport) error {
	checks := []struct {
		table  string
		column string
		value  string
	}{
		{"projects", "project_id", p.ProjectID},
		{"project_revisions", "revision_id", p.RevisionID},
		{"runtime_snapshots", "runtime_snapshot_id", p.RuntimeSnapshot.ID},
	}
	for _, check := range checks {
		var exists int
		err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE %s = ?`, check.table, check.column), check.value).Scan(&exists)
		if err == nil {
			return fmt.Errorf("%w: portable identity %s already exists", domain.ErrConflict, check.value)
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("check portable identity %s: %w", check.value, err)
		}
	}
	return nil
}

func portableVaultObjectTx(ctx context.Context, tx *sql.Tx, contentHash string) (VaultObject, error) {
	var object VaultObject
	var createdUS int64
	if err := tx.QueryRowContext(ctx, `
		SELECT content_hash, size_bytes, relative_path, created_at_us
		FROM vault_objects WHERE content_hash = ?`, strings.ToLower(strings.TrimSpace(contentHash))).Scan(
		&object.ContentHash, &object.SizeBytes, &object.RelativePath, &createdUS,
	); err != nil {
		return VaultObject{}, fmt.Errorf("read portable Vault object: %w", err)
	}
	object.CreatedAt = clock.FromUnixMicros(createdUS)
	return object, nil
}

func importPortableExtensionTx(ctx context.Context, tx *sql.Tx, extension PortableExtensionImport, actor string, nowUS int64) error {
	var existingHash string
	err := tx.QueryRowContext(ctx, `SELECT content_hash FROM software_packages WHERE package_id = ?`, extension.PackageID).Scan(&existingHash)
	if err == nil {
		if !strings.EqualFold(existingHash, extension.ContentHash) {
			return fmt.Errorf("%w: extension package identity %s conflicts with existing software content", domain.ErrConflict, extension.PackageID)
		}
		var extensionHash string
		if err := tx.QueryRowContext(ctx, `SELECT manifest_sha256 FROM extension_packages WHERE package_id = ?`, extension.PackageID).Scan(&extensionHash); err != nil {
			return fmt.Errorf("existing software package %s is not the expected extension package: %w", extension.PackageID, err)
		}
		if !strings.EqualFold(extensionHash, extension.ManifestSHA256) {
			return fmt.Errorf("%w: extension package %s manifest identity conflicts with existing package", domain.ErrConflict, extension.PackageID)
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check portable extension package %s: %w", extension.PackageID, err)
	}
	var objectSize int64
	if err := tx.QueryRowContext(ctx, `SELECT size_bytes FROM vault_objects WHERE content_hash = ?`, strings.ToLower(extension.ContentHash)).Scan(&objectSize); err != nil {
		return fmt.Errorf("portable extension %s Vault object is unavailable: %w", extension.ExtensionID, err)
	}
	if objectSize != extension.SizeBytes {
		return fmt.Errorf("%w: portable extension %s Vault size mismatch", domain.ErrConflict, extension.ExtensionID)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO software_packages (package_id, product_id, version, platform, architecture, min_api_version, max_api_version, content_hash, size_bytes, original_filename, signing_status, notarization_status, release_channel, release_notes, created_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`, extension.PackageID, extension.ProductID, extension.SoftwareVersion, extension.Platform, extension.Architecture, extension.MinAPIVersion, extension.MaxAPIVersion, strings.ToLower(extension.ContentHash), extension.SizeBytes, extension.OriginalFilename, extension.SigningStatus, extension.NotarizationStatus, extension.ReleaseChannel, nowUS); err != nil {
		return fmt.Errorf("insert portable software package %s: %w", extension.PackageID, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO extension_packages (package_id, extension_id, version, kind, source, manifest_json, manifest_sha256, registered_by, registered_at_us)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, extension.PackageID, extension.ExtensionID, extension.ExtensionVersion, extension.Kind, extension.Source, string(extension.ManifestJSON), strings.ToLower(extension.ManifestSHA256), actor, nowUS); err != nil {
		return fmt.Errorf("insert portable extension package %s: %w", extension.PackageID, err)
	}
	return nil
}

func jsonOrDefault(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 {
		return fallback
	}
	return string(raw)
}
