package showcapsule

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ali96adil/StageCore/internal/canonicaljson"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

type MaterializeOptions struct {
	ImportedBy string `json:"imported_by"`
}

type MaterializeResult struct {
	CapsuleID            string   `json:"capsule_id"`
	ProjectID            string   `json:"project_id"`
	RevisionID           string   `json:"revision_id"`
	RuntimeSnapshotID    string   `json:"runtime_snapshot_id"`
	ImportedObjects      []string `json:"imported_objects,omitempty"`
	ReusedObjects        []string `json:"reused_objects,omitempty"`
	ImportedExtensions   []string `json:"imported_extensions,omitempty"`
	ReplacementHostReady bool     `json:"replacement_host_ready"`
	Plan                 ImportPlan `json:"plan"`
}

func (s *Service) Materialize(ctx context.Context, capsulePath string, options MaterializeOptions) (MaterializeResult, error) {
	actor := strings.TrimSpace(options.ImportedBy)
	if actor == "" || len(actor) > 256 {
		return MaterializeResult{}, fmt.Errorf("%w: bounded import actor is required", domain.ErrInvalidInput)
	}
	plan, err := s.PlanImport(ctx, capsulePath)
	if err != nil {
		return MaterializeResult{}, err
	}
	if !plan.MaterializationReady {
		return MaterializeResult{}, fmt.Errorf("%w: show capsule is not ready for materialization", domain.ErrConflict)
	}

	importedObjects := make([]string, 0, len(plan.IncludedObjects))
	projectCommitted := false
	defer func() {
		if projectCommitted || len(importedObjects) == 0 {
			return
		}
		cleanupCtx := context.WithoutCancel(ctx)
		for _, contentHash := range importedObjects {
			_, _ = s.vault.RemoveObjectIfUnreferenced(cleanupCtx, contentHash)
		}
	}()

	for _, object := range plan.IncludedObjects {
		path := filepath.Join(filepath.Clean(capsulePath), filepath.FromSlash(object.ArchivePath))
		file, err := os.Open(path)
		if err != nil {
			return MaterializeResult{}, fmt.Errorf("open capsule object %s: %w", object.ContentSHA256, err)
		}
		imported, importErr := s.vault.ImportObject(ctx, file)
		closeErr := file.Close()
		if importErr != nil {
			return MaterializeResult{}, fmt.Errorf("import capsule object %s: %w", object.ContentSHA256, importErr)
		}
		if closeErr != nil {
			return MaterializeResult{}, fmt.Errorf("close capsule object %s: %w", object.ContentSHA256, closeErr)
		}
		if imported.ContentHash != object.ContentSHA256 || imported.SizeBytes != object.SizeBytes {
			return MaterializeResult{}, fmt.Errorf("%w: imported capsule object identity changed", domain.ErrConflict)
		}
		importedObjects = append(importedObjects, imported.ContentHash)
	}

	// Re-evaluate after object promotion. This catches storage metadata conflicts,
	// an operator starting SHOW, or any identity appearing between planning and
	// the transactional Project write.
	plan, err = s.PlanImport(ctx, capsulePath)
	if err != nil {
		return MaterializeResult{}, err
	}
	if !plan.MaterializationReady {
		return MaterializeResult{}, fmt.Errorf("%w: show capsule readiness changed before materialization", domain.ErrConflict)
	}

	bundle, err := portableImportFromManifest(plan.Manifest, actor)
	if err != nil {
		return MaterializeResult{}, err
	}
	imported, err := s.store.ImportPortableProject(ctx, bundle)
	if err != nil {
		return MaterializeResult{}, err
	}
	projectCommitted = true

	finalPlan, err := s.planImportedProject(ctx, capsulePath, plan.Manifest)
	if err != nil {
		return MaterializeResult{}, err
	}
	reused := make([]string, 0, len(plan.ReusableObjects))
	for _, object := range plan.ReusableObjects {
		reused = append(reused, object.ContentSHA256)
	}
	return MaterializeResult{
		CapsuleID: plan.CapsuleID,
		ProjectID: imported.ProjectID,
		RevisionID: imported.RevisionID,
		RuntimeSnapshotID: imported.RuntimeSnapshotID,
		ImportedObjects: importedObjects,
		ReusedObjects: reused,
		ImportedExtensions: imported.ImportedExtensions,
		ReplacementHostReady: finalPlan.ReplacementHostReady,
		Plan: finalPlan,
	}, nil
}

func portableImportFromManifest(manifest Manifest, actor string) (store.PortableProjectImport, error) {
	runtimeJSON, err := canonicaljson.Marshal(manifest.RuntimeSnapshot.Manifest)
	if err != nil {
		return store.PortableProjectImport{}, fmt.Errorf("canonicalize imported Runtime Snapshot: %w", err)
	}
	notes := map[string]string{}
	for _, note := range manifest.OperatorMetadata.CueNotes {
		notes[note.CueID] = note.Text
	}
	bundle := store.PortableProjectImport{
		ProjectID: manifest.Project.ProjectID,
		Name: manifest.Project.Name,
		Description: manifest.Project.Description,
		RevisionID: manifest.Project.RevisionID,
		RevisionNumber: manifest.Project.RevisionNumber,
		RuntimeSnapshot: store.PortableRuntimeSnapshotImport{
			ID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
			Version: manifest.RuntimeSnapshot.SnapshotVersion,
			ContentHash: manifest.RuntimeSnapshot.ContentSHA256,
			ManifestJSON: json.RawMessage(runtimeJSON),
		},
		ImportedBy: actor,
	}
	for _, target := range manifest.RuntimeSnapshot.Manifest.Targets {
		bundle.Aliases = append(bundle.Aliases, store.PortableAliasImport{
			ID: target.AliasID,
			LogicalName: target.TargetRef,
			LogicalType: target.LogicalType,
			TargetRef: "",
			GroupName: "",
			Configuration: append(json.RawMessage(nil), target.Configuration...),
		})
	}
	for _, cue := range manifest.RuntimeSnapshot.Manifest.Cues {
		item := store.PortableCueImport{
			ID: cue.ID,
			DisplayLabel: cue.DisplayLabel,
			Name: cue.Name,
			OrderIndex: cue.OrderIndex,
			CueType: cue.CueType,
			Criticality: cue.Criticality,
			Enabled: cue.Enabled,
			ExecutionPolicy: append(json.RawMessage(nil), cue.ExecutionPolicy...),
			NotesSummary: notes[cue.ID],
		}
		for _, action := range cue.Actions {
			item.Actions = append(item.Actions, store.PortableActionImport{
				ID: action.ID,
				OrderIndex: action.OrderIndex,
				ExecutionMode: action.ExecutionMode,
				TargetRef: action.TargetRef,
				CapabilityKey: action.CapabilityKey,
				Parameters: append(json.RawMessage(nil), action.Parameters...),
				TimeoutPolicy: append(json.RawMessage(nil), action.TimeoutPolicy...),
				ErrorPolicy: append(json.RawMessage(nil), action.ErrorPolicy...),
				PriorityClass: action.PriorityClass,
				Enabled: action.Enabled,
			})
		}
		bundle.Cues = append(bundle.Cues, item)
	}
	for _, input := range manifest.RuntimeSnapshot.Manifest.Inputs {
		bundle.Inputs = append(bundle.Inputs, store.PortableInputImport{
			ID: input.ID, Name: input.Name, SourceRef: input.SourceRef, EventType: input.EventType,
			ValueSchema: append(json.RawMessage(nil), input.ValueSchema...), Enabled: input.Enabled,
		})
	}
	for _, output := range manifest.RuntimeSnapshot.Manifest.Outputs {
		bundle.Outputs = append(bundle.Outputs, store.PortableOutputImport{
			ID: output.ID, Name: output.Name, TargetRef: output.TargetRef, CapabilityKey: output.CapabilityKey,
			ValueSchema: append(json.RawMessage(nil), output.ValueSchema...), Criticality: output.Criticality,
		})
	}
	for _, route := range manifest.RuntimeSnapshot.Manifest.Routes {
		item := store.PortableRouteImport{
			ID: route.ID, Name: route.Name, InputID: route.InputID,
			ConditionDefinition: append(json.RawMessage(nil), route.ConditionDefinition...),
			TransformDefinition: append(json.RawMessage(nil), route.TransformDefinition...),
			DelayMS: cloneInt64(route.DelayMS), DebounceMS: cloneInt64(route.DebounceMS),
			PriorityClass: route.PriorityClass, ErrorPolicy: append(json.RawMessage(nil), route.ErrorPolicy...), Enabled: route.Enabled,
		}
		for _, action := range route.Actions {
			item.Actions = append(item.Actions, store.PortableRouteActionImport{
				ID: action.ID, OrderIndex: action.OrderIndex,
				OutputID: cloneString(action.OutputID), CueID: cloneString(action.CueID),
				Parameters: append(json.RawMessage(nil), action.Parameters...),
			})
		}
		bundle.Routes = append(bundle.Routes, item)
	}
	for _, role := range manifest.MachineRoles {
		bundle.MachineRoles = append(bundle.MachineRoles, store.PortableMachineRoleImport{
			ID: role.MachineRoleID, RoleKey: role.RoleKey, DisplayName: role.DisplayName,
			RequiredCapabilities: append([]string(nil), role.RequiredCapabilities...),
			RequiredRuntimeSnapshotID: cloneString(role.RequiredRuntimeSnapshotID),
			RequiredConfigHash: role.RequiredConfigHash, Required: role.Required,
		})
	}
	for _, media := range manifest.Media {
		bundle.Media = append(bundle.Media, store.PortableMediaImport{
			MachineRoleID: media.MachineRoleID,
			MediaAssetID: media.MediaAssetID,
			ContentVersionID: media.ContentVersionID,
			Name: media.MediaAssetID,
			AssetPolicy: media.AssetPolicy,
			ContentHash: media.ContentSHA256,
			SizeBytes: media.SizeBytes,
			OriginalFilename: "",
			Required: media.Required,
		})
	}
	for _, environment := range manifest.ExecutionEnvironments {
		item := store.PortableExecutionEnvironmentImport{
			ManifestID: environment.ManifestID,
			MachineRoleID: cloneString(environment.MachineRoleID),
			ContentHash: environment.ContentSHA256,
			Manifest: environment.Manifest,
		}
		if environment.LatestSnapshot != nil {
			item.SnapshotID = environment.LatestSnapshot.SnapshotID
			item.SnapshotHash = environment.LatestSnapshot.ContentSHA256
			snapshotCopy := environment.LatestSnapshot.Snapshot
			item.Snapshot = &snapshotCopy
		}
		bundle.ExecutionEnvironments = append(bundle.ExecutionEnvironments, item)
	}
	for _, extension := range manifest.Extensions {
		bundle.Extensions = append(bundle.Extensions, store.PortableExtensionImport{
			PackageID: extension.PackageID,
			ProductID: extension.Software.ProductID,
			SoftwareVersion: extension.Software.Version,
			Platform: extension.Software.Platform,
			Architecture: extension.Software.Architecture,
			MinAPIVersion: extension.Software.MinAPIVersion,
			MaxAPIVersion: extension.Software.MaxAPIVersion,
			ContentHash: extension.Software.ContentSHA256,
			SizeBytes: extension.Software.SizeBytes,
			OriginalFilename: extension.Software.OriginalFilename,
			SigningStatus: extension.Software.SigningStatus,
			NotarizationStatus: extension.Software.NotarizationStatus,
			ReleaseChannel: extension.Software.ReleaseChannel,
			ExtensionID: extension.ExtensionID,
			ExtensionVersion: extension.Version,
			Kind: extension.Kind,
			Source: extension.Source,
			ManifestJSON: append(json.RawMessage(nil), extension.Manifest...),
			ManifestSHA256: extension.ManifestSHA256,
		})
	}
	return bundle, nil
}

func (s *Service) planImportedProject(ctx context.Context, capsulePath string, manifest Manifest) (ImportPlan, error) {
	plan := ImportPlan{
		CapsuleID: manifest.CapsuleID,
		ProjectID: manifest.Project.ProjectID,
		RuntimeSnapshotID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
		ExportMode: manifest.ExportMode,
		MaterializationReady: true,
		ReplacementHostReady: true,
		Manifest: manifest,
	}
	plan.pass("capsule.materialized", "Project graph and immutable Runtime Snapshot were materialized successfully.")
	for _, object := range manifest.Objects {
		file, _, err := s.vault.OpenObject(ctx, object.ContentSHA256)
		if err != nil {
			plan.ReplacementHostReady = false
			plan.Checks = append(plan.Checks, ReadinessCheck{Code: "object.after_import_unavailable", Severity: ReadinessBlock, Message: fmt.Sprintf("Vault object %s is unavailable after import.", object.ContentSHA256)})
			continue
		}
		_ = file.Close()
	}
	for _, media := range manifest.Media {
		if strings.EqualFold(media.AssetPolicy, "REFERENCE_ONLY") {
			plan.warnShow("media.reference_only", fmt.Sprintf("Media %s remains REFERENCE_ONLY and must be resolved before SHOW.", media.MediaAssetID))
		}
	}
	for _, environment := range manifest.ExecutionEnvironments {
		if environment.LatestSnapshot == nil {
			plan.warnShow("environment.snapshot_missing", fmt.Sprintf("Execution environment %s must be re-established before SHOW.", environment.Manifest.EnvironmentKey))
		}
	}
	for _, extension := range manifest.Extensions {
		plan.warnShow("extension.activation_required", fmt.Sprintf("Extension %s@%s package is restored but installation/permission review must be completed through F-015 before SHOW.", extension.ExtensionID, extension.Version))
	}
	if manifest.Presentation.AppearanceDeviceLocal || manifest.Presentation.WorkspaceDeviceLocal {
		plan.Checks = append(plan.Checks, ReadinessCheck{Code: "presentation.device_local", Severity: ReadinessWarning, Message: "Appearance/workspace state is device-local and is intentionally not restored; this does not affect SHOW readiness."})
	}
	return plan, nil
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
