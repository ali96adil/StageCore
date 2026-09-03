package showcapsule

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/software"
)

type ReadinessSeverity string

const (
	ReadinessPass    ReadinessSeverity = "PASS"
	ReadinessWarning ReadinessSeverity = "WARN"
	ReadinessBlock   ReadinessSeverity = "BLOCK"
)

type ReadinessCheck struct {
	Code     string            `json:"code"`
	Severity ReadinessSeverity `json:"severity"`
	Message  string            `json:"message"`
}

type ImportPlan struct {
	CapsuleID            string           `json:"capsule_id"`
	ProjectID            string           `json:"project_id"`
	RuntimeSnapshotID    string           `json:"runtime_snapshot_id"`
	ExportMode           ExportMode       `json:"export_mode"`
	MaterializationReady bool             `json:"materialization_ready"`
	ReplacementHostReady bool             `json:"replacement_host_ready"`
	Checks               []ReadinessCheck `json:"checks"`
	MissingObjects       []ObjectEntry    `json:"missing_objects,omitempty"`
	ReusableObjects      []ObjectEntry    `json:"reusable_objects,omitempty"`
	IncludedObjects      []ObjectEntry    `json:"included_objects,omitempty"`
	Manifest             Manifest         `json:"manifest"`
}

func (s *Service) PlanImport(ctx context.Context, capsulePath string) (ImportPlan, error) {
	if s == nil || s.store == nil || s.vault == nil {
		return ImportPlan{}, fmt.Errorf("show capsule service is unavailable")
	}
	manifest, err := Verify(capsulePath)
	if err != nil {
		return ImportPlan{}, err
	}
	plan := ImportPlan{
		CapsuleID: manifest.CapsuleID,
		ProjectID: manifest.Project.ProjectID,
		RuntimeSnapshotID: manifest.RuntimeSnapshot.RuntimeSnapshotID,
		ExportMode: manifest.ExportMode,
		MaterializationReady: true,
		ReplacementHostReady: true,
		Manifest: manifest,
	}
	plan.pass("capsule.integrity", "Capsule manifest and included content hashes are verified.")

	activeType, err := s.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return ImportPlan{}, fmt.Errorf("read active operational session: %w", err)
	}
	if activeType == domain.SessionShow {
		plan.blockMaterialization("runtime.active_show", "A SHOW session is active; capsule materialization is blocked until SHOW ends.")
	} else {
		plan.pass("runtime.active_show", "No active SHOW session blocks capsule materialization.")
	}

	if _, err := s.store.GetProject(ctx, manifest.Project.ProjectID); err == nil {
		plan.blockMaterialization("identity.project_collision", "Project ID already exists on this Hub; identity-preserving import will not overwrite it.")
	} else if !errors.Is(err, domain.ErrNotFound) {
		return ImportPlan{}, fmt.Errorf("check Project identity collision: %w", err)
	} else {
		plan.pass("identity.project_collision", "Project identity is available on this Hub.")
	}
	if _, err := s.store.GetRevision(ctx, manifest.Project.RevisionID); err == nil {
		plan.blockMaterialization("identity.revision_collision", "Revision ID already exists on this Hub.")
	} else if !errors.Is(err, domain.ErrNotFound) {
		return ImportPlan{}, fmt.Errorf("check revision identity collision: %w", err)
	}
	if _, err := s.store.GetRuntimeSnapshot(ctx, manifest.RuntimeSnapshot.RuntimeSnapshotID); err == nil {
		plan.blockMaterialization("identity.snapshot_collision", "Runtime Snapshot ID already exists on this Hub.")
	} else if !errors.Is(err, domain.ErrNotFound) {
		return ImportPlan{}, fmt.Errorf("check Runtime Snapshot identity collision: %w", err)
	}

	for _, object := range manifest.Objects {
		local, err := s.store.GetVaultObject(ctx, object.ContentSHA256)
		switch {
		case err == nil:
			if local.SizeBytes != object.SizeBytes {
				plan.blockMaterialization("object.metadata_conflict", fmt.Sprintf("Local Vault object %s has conflicting size metadata.", object.ContentSHA256))
				plan.MissingObjects = append(plan.MissingObjects, object)
				continue
			}
			file, _, openErr := s.vault.OpenObject(ctx, object.ContentSHA256)
			if openErr != nil {
				plan.blockMaterialization("object.local_unreadable", fmt.Sprintf("Local Vault object %s is registered but cannot be verified: %v", object.ContentSHA256, openErr))
				plan.MissingObjects = append(plan.MissingObjects, object)
				continue
			}
			_ = file.Close()
			plan.ReusableObjects = append(plan.ReusableObjects, object)
		case errors.Is(err, domain.ErrNotFound) && object.Included:
			plan.IncludedObjects = append(plan.IncludedObjects, object)
		case errors.Is(err, domain.ErrNotFound):
			plan.MissingObjects = append(plan.MissingObjects, object)
			if object.Required {
				plan.blockMaterialization("object.required_missing", fmt.Sprintf("Required object %s is not included and is not present in the local Vault.", object.ContentSHA256))
			} else {
				plan.warnShow("object.optional_missing", fmt.Sprintf("Optional object %s is not present on the replacement host.", object.ContentSHA256))
			}
		default:
			return ImportPlan{}, fmt.Errorf("inspect local Vault object %s: %w", object.ContentSHA256, err)
		}
	}

	for _, item := range manifest.Media {
		if !strings.EqualFold(item.AssetPolicy, "REFERENCE_ONLY") {
			continue
		}
		message := fmt.Sprintf("Media %s for role %s is REFERENCE_ONLY and must be resolved externally.", item.MediaAssetID, item.RoleKey)
		local, err := s.store.GetVaultObject(ctx, item.ContentSHA256)
		if errors.Is(err, domain.ErrNotFound) {
			plan.blockMaterialization("media.reference_only_unresolved", message+" No exact content object is available in the local Vault, so identity-preserving materialization is blocked.")
			continue
		}
		if err != nil {
			return ImportPlan{}, fmt.Errorf("inspect REFERENCE_ONLY media %s: %w", item.MediaAssetID, err)
		}
		if local.SizeBytes != item.SizeBytes {
			plan.blockMaterialization("media.reference_only_conflict", message+" The local content hash has conflicting size metadata.")
			continue
		}
		file, _, err := s.vault.OpenObject(ctx, item.ContentSHA256)
		if err != nil {
			plan.blockMaterialization("media.reference_only_unreadable", message+" The matching local content object cannot be verified.")
			continue
		}
		_ = file.Close()
		plan.warnShow("media.reference_only_resolved", message+" Exact bytes are locally available for materialization, but the REFERENCE_ONLY policy remains visible for replacement-host review.")
	}

	for _, environment := range manifest.ExecutionEnvironments {
		key := environment.Manifest.EnvironmentKey
		if environment.LatestSnapshot == nil {
			plan.warnShow("environment.snapshot_missing", fmt.Sprintf("Execution environment %s has no captured snapshot.", key))
			continue
		}
		snapshot := environment.LatestSnapshot.Snapshot
		if snapshot.CaptureStatus != executionenv.SnapshotComplete {
			plan.warnShow("environment.snapshot_incomplete", fmt.Sprintf("Execution environment %s snapshot status is %s.", key, snapshot.CaptureStatus))
		}
		for _, item := range snapshot.Items {
			switch item.Portability {
			case executionenv.SnapshotReferenceOnly:
				plan.warnShow("environment.reference_only", fmt.Sprintf("Execution environment %s item %s must be resolved from external reference %s.", key, item.Key, item.Locator))
			case executionenv.SnapshotDescriptiveOnly:
				plan.warnShow("environment.descriptive_only", fmt.Sprintf("Execution environment %s item %s is descriptive evidence only and must be re-established on the replacement host.", key, item.Key))
			}
		}
	}

	for _, requirement := range manifest.Extensions {
		if software.CurrentHubAPIVersion < requirement.Software.MinAPIVersion || software.CurrentHubAPIVersion > requirement.Software.MaxAPIVersion {
			plan.blockMaterialization("extension.api_incompatible", fmt.Sprintf("Extension %s@%s requires Hub API %d..%d; current API is %d.", requirement.ExtensionID, requirement.Version, requirement.Software.MinAPIVersion, requirement.Software.MaxAPIVersion, software.CurrentHubAPIVersion))
		}
	}

	if manifest.Presentation.AppearanceDeviceLocal || manifest.Presentation.WorkspaceDeviceLocal {
		plan.warnShow("presentation.device_local", "Appearance/workspace presentation state is device-local and must be chosen again on the replacement device.")
	}
	if len(manifest.Warnings) > 0 {
		for _, warning := range manifest.Warnings {
			plan.Checks = append(plan.Checks, ReadinessCheck{Code: "capsule.warning", Severity: ReadinessWarning, Message: warning})
		}
	}

	sort.Slice(plan.Checks, func(i, j int) bool {
		if plan.Checks[i].Severity != plan.Checks[j].Severity {
			return severityOrder(plan.Checks[i].Severity) < severityOrder(plan.Checks[j].Severity)
		}
		if plan.Checks[i].Code != plan.Checks[j].Code {
			return plan.Checks[i].Code < plan.Checks[j].Code
		}
		return plan.Checks[i].Message < plan.Checks[j].Message
	})
	sort.Slice(plan.MissingObjects, func(i, j int) bool { return plan.MissingObjects[i].ContentSHA256 < plan.MissingObjects[j].ContentSHA256 })
	sort.Slice(plan.ReusableObjects, func(i, j int) bool { return plan.ReusableObjects[i].ContentSHA256 < plan.ReusableObjects[j].ContentSHA256 })
	sort.Slice(plan.IncludedObjects, func(i, j int) bool { return plan.IncludedObjects[i].ContentSHA256 < plan.IncludedObjects[j].ContentSHA256 })
	return plan, nil
}

func (p *ImportPlan) pass(code, message string) {
	p.Checks = append(p.Checks, ReadinessCheck{Code: code, Severity: ReadinessPass, Message: message})
}

func (p *ImportPlan) warnShow(code, message string) {
	p.ReplacementHostReady = false
	p.Checks = append(p.Checks, ReadinessCheck{Code: code, Severity: ReadinessWarning, Message: message})
}

func (p *ImportPlan) blockMaterialization(code, message string) {
	p.MaterializationReady = false
	p.ReplacementHostReady = false
	p.Checks = append(p.Checks, ReadinessCheck{Code: code, Severity: ReadinessBlock, Message: message})
}

func severityOrder(value ReadinessSeverity) int {
	switch value {
	case ReadinessBlock:
		return 0
	case ReadinessWarning:
		return 1
	default:
		return 2
	}
}
