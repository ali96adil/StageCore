package showcapsule

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/extension"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

type Service struct {
	store *store.Store
	vault *vault.Vault
	clock clock.Clock
}

type BuildOptions struct {
	RuntimeSnapshotID string
	Mode              ExportMode
}

type ExportResult struct {
	CapsuleID string   `json:"capsule_id"`
	Path      string   `json:"path"`
	Manifest  Manifest `json:"manifest"`
}

func New(stageStore *store.Store, stageVault *vault.Vault, stageClock clock.Clock) (*Service, error) {
	if stageStore == nil || stageVault == nil {
		return nil, fmt.Errorf("show capsule requires Store and Vault")
	}
	if stageClock == nil {
		stageClock = clock.Real{}
	}
	return &Service{store: stageStore, vault: stageVault, clock: stageClock}, nil
}

func (s *Service) BuildManifest(ctx context.Context, projectID string, options BuildOptions) (Manifest, error) {
	if s == nil || s.store == nil || s.vault == nil {
		return Manifest{}, fmt.Errorf("show capsule service is unavailable")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Manifest{}, fmt.Errorf("%w: project is required", domain.ErrInvalidInput)
	}
	if options.Mode == "" {
		options.Mode = ExportManifestOnly
	}
	if options.Mode != ExportManifestOnly && options.Mode != ExportSelfContained {
		return Manifest{}, fmt.Errorf("%w: unsupported show capsule export mode", domain.ErrInvalidInput)
	}

	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return Manifest{}, err
	}
	runtimeSnapshot, runtimeManifest, err := s.resolveRuntimeSnapshot(ctx, projectID, options.RuntimeSnapshotID)
	if err != nil {
		return Manifest{}, err
	}
	revision, err := s.store.GetRevision(ctx, runtimeSnapshot.RevisionID)
	if err != nil {
		return Manifest{}, err
	}
	if revision.ProjectID != projectID || revision.ID != runtimeManifest.RevisionID || revision.RevisionNumber != runtimeManifest.RevisionNumber {
		return Manifest{}, fmt.Errorf("%w: runtime snapshot revision identity is inconsistent", domain.ErrConflict)
	}
	capsuleID, err := stageid.New()
	if err != nil {
		return Manifest{}, err
	}

	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		CapsuleID:     capsuleID,
		ExportMode:    options.Mode,
		CreatedAt:     s.clock.Now().UTC(),
		Project: ProjectIdentity{
			ProjectID: project.ID, Name: project.Name, Description: project.Description,
			RevisionID: revision.ID, RevisionNumber: revision.RevisionNumber,
		},
		RuntimeSnapshot: RuntimeSnapshotIdentity{
			RuntimeSnapshotID: runtimeSnapshot.ID, SnapshotVersion: runtimeSnapshot.SnapshotVersion,
			ContentSHA256: runtimeSnapshot.ContentHash, Manifest: runtimeManifest,
		},
		Presentation: PresentationPortability{
			AppearanceDeviceLocal: true,
			WorkspaceDeviceLocal:  true,
			Exported:              false,
			Note:                  "Appearance and workspace profiles are device-local presentation state and are intentionally not exported by the Show Capsule.",
		},
	}

	roles, err := s.store.ListMachineRoles(ctx, projectID)
	if err != nil {
		return Manifest{}, err
	}
	for _, role := range roles {
		manifest.MachineRoles = append(manifest.MachineRoles, MachineRoleIdentity{
			MachineRoleID: role.ID, RoleKey: role.RoleKey, DisplayName: role.DisplayName,
			RequiredCapabilities: append([]string(nil), role.RequiredCapabilities...),
			RequiredRuntimeSnapshotID: cloneString(role.RequiredRuntimeSnapshotID),
			RequiredConfigHash: role.RequiredConfigHash, Required: role.Required,
		})
	}

	cues, err := s.store.ListCues(ctx, revision.ID)
	if err != nil {
		return Manifest{}, err
	}
	for _, cue := range cues {
		if note := strings.TrimSpace(cue.NotesSummary); note != "" {
			manifest.OperatorMetadata.CueNotes = append(manifest.OperatorMetadata.CueNotes, CueNote{CueID: cue.ID, Text: note})
		}
	}

	objects := map[string]*ObjectEntry{}
	if err := s.collectMedia(ctx, &manifest, runtimeManifest, objects); err != nil {
		return Manifest{}, err
	}
	if err := s.collectExecutionEnvironments(ctx, &manifest, revision.ID, objects); err != nil {
		return Manifest{}, err
	}
	if err := s.collectExtensions(ctx, &manifest, runtimeManifest, objects); err != nil {
		return Manifest{}, err
	}
	for _, object := range objects {
		manifest.Objects = append(manifest.Objects, *object)
	}
	return Normalize(manifest)
}

func (s *Service) Export(ctx context.Context, projectID, destinationRoot string, options BuildOptions) (ExportResult, error) {
	manifest, err := s.BuildManifest(ctx, projectID, options)
	if err != nil {
		return ExportResult{}, err
	}
	destinationRoot = strings.TrimSpace(destinationRoot)
	if destinationRoot == "" {
		return ExportResult{}, fmt.Errorf("%w: destination root is required", domain.ErrInvalidInput)
	}
	destinationRoot = filepath.Clean(destinationRoot)
	if err := os.MkdirAll(destinationRoot, 0o750); err != nil {
		return ExportResult{}, fmt.Errorf("create show capsule destination root: %w", err)
	}
	finalPath := filepath.Join(destinationRoot, manifest.CapsuleID)
	if _, err := os.Stat(finalPath); err == nil {
		return ExportResult{}, fmt.Errorf("%w: show capsule destination already exists", domain.ErrConflict)
	} else if !os.IsNotExist(err) {
		return ExportResult{}, fmt.Errorf("inspect show capsule destination: %w", err)
	}
	stagingPath, err := os.MkdirTemp(destinationRoot, "."+manifest.CapsuleID+".staging-")
	if err != nil {
		return ExportResult{}, fmt.Errorf("create show capsule staging directory: %w", err)
	}
	promoted := false
	defer func() {
		if !promoted {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	if manifest.ExportMode == ExportSelfContained {
		for _, object := range manifest.Objects {
			if err := s.copyVaultObject(ctx, stagingPath, object); err != nil {
				return ExportResult{}, err
			}
		}
	}
	manifestBytes, err := CanonicalBytes(manifest)
	if err != nil {
		return ExportResult{}, err
	}
	if err := writeSyncedFile(filepath.Join(stagingPath, ManifestFileName), manifestBytes, 0o640); err != nil {
		return ExportResult{}, fmt.Errorf("write show capsule manifest: %w", err)
	}
	digest := sha256.Sum256(manifestBytes)
	checksum := []byte(hex.EncodeToString(digest[:]) + "  " + ManifestFileName + "\n")
	if err := writeSyncedFile(filepath.Join(stagingPath, ManifestChecksumFile), checksum, 0o640); err != nil {
		return ExportResult{}, fmt.Errorf("write show capsule manifest checksum: %w", err)
	}
	verified, err := Verify(stagingPath)
	if err != nil {
		return ExportResult{}, fmt.Errorf("verify staged show capsule: %w", err)
	}
	if verified.CapsuleID != manifest.CapsuleID {
		return ExportResult{}, fmt.Errorf("verified capsule identity changed during staging")
	}
	if err := os.Rename(stagingPath, finalPath); err != nil {
		return ExportResult{}, fmt.Errorf("atomically promote show capsule: %w", err)
	}
	promoted = true
	return ExportResult{CapsuleID: manifest.CapsuleID, Path: finalPath, Manifest: verified}, nil
}

func Verify(capsulePath string) (Manifest, error) {
	capsulePath = strings.TrimSpace(capsulePath)
	if capsulePath == "" {
		return Manifest{}, fmt.Errorf("show capsule path is required")
	}
	capsulePath = filepath.Clean(capsulePath)
	manifestPath := filepath.Join(capsulePath, ManifestFileName)
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("read show capsule manifest: %w", err)
	}
	checksumBytes, err := os.ReadFile(filepath.Join(capsulePath, ManifestChecksumFile))
	if err != nil {
		return Manifest{}, fmt.Errorf("read show capsule checksum: %w", err)
	}
	fields := strings.Fields(string(checksumBytes))
	if len(fields) != 2 || fields[1] != ManifestFileName || !isSHA256(strings.ToLower(fields[0])) {
		return Manifest{}, fmt.Errorf("invalid show capsule manifest checksum record")
	}
	digest := sha256.Sum256(manifestBytes)
	if hex.EncodeToString(digest[:]) != strings.ToLower(fields[0]) {
		return Manifest{}, fmt.Errorf("show capsule manifest checksum mismatch")
	}
	manifest, err := Decode(manifestBytes)
	if err != nil {
		return Manifest{}, err
	}
	canonical, err := CanonicalBytes(manifest)
	if err != nil {
		return Manifest{}, err
	}
	if !bytes.Equal(canonical, manifestBytes) {
		return Manifest{}, fmt.Errorf("show capsule manifest is not canonical")
	}
	for _, object := range manifest.Objects {
		if !object.Included {
			continue
		}
		path := filepath.Join(capsulePath, filepath.FromSlash(object.ArchivePath))
		if err := verifyFile(path, object.ContentSHA256, object.SizeBytes); err != nil {
			return Manifest{}, fmt.Errorf("verify capsule object %s: %w", object.ContentSHA256, err)
		}
	}
	return manifest, nil
}

func (s *Service) resolveRuntimeSnapshot(ctx context.Context, projectID, snapshotID string) (domain.RuntimeSnapshot, snapshot.Manifest, error) {
	var runtimeSnapshot domain.RuntimeSnapshot
	var err error
	if snapshotID = strings.TrimSpace(snapshotID); snapshotID != "" {
		runtimeSnapshot, err = s.store.GetRuntimeSnapshot(ctx, snapshotID)
	} else {
		latest, loadErr := s.store.LatestPublishedRuntimeSnapshotForProject(ctx, projectID)
		if loadErr != nil {
			return domain.RuntimeSnapshot{}, snapshot.Manifest{}, loadErr
		}
		if latest == nil {
			return domain.RuntimeSnapshot{}, snapshot.Manifest{}, domain.ErrNotFound
		}
		runtimeSnapshot = *latest
	}
	if err != nil {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, err
	}
	if runtimeSnapshot.ProjectID != projectID || runtimeSnapshot.Status != domain.SnapshotPublished {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, fmt.Errorf("%w: runtime snapshot does not belong to project or is not published", domain.ErrConflict)
	}
	manifest, err := snapshot.Decode(runtimeSnapshot.Manifest)
	if err != nil {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, err
	}
	if manifest.ProjectID != projectID || manifest.RevisionID != runtimeSnapshot.RevisionID {
		return domain.RuntimeSnapshot{}, snapshot.Manifest{}, fmt.Errorf("%w: runtime snapshot manifest identity mismatch", domain.ErrConflict)
	}
	return runtimeSnapshot, manifest, nil
}

func (s *Service) collectMedia(ctx context.Context, manifest *Manifest, runtimeManifest snapshot.Manifest, objects map[string]*ObjectEntry) error {
	for _, requirement := range runtimeManifest.RequiredMedia {
		asset, err := s.store.GetMediaAsset(ctx, requirement.MediaAssetID)
		if err != nil {
			return fmt.Errorf("resolve capsule media asset %s: %w", requirement.MediaAssetID, err)
		}
		if asset.ProjectID != manifest.Project.ProjectID {
			return fmt.Errorf("%w: media asset crosses project boundary", domain.ErrConflict)
		}
		item := MediaRequirement{
			MachineRoleID: requirement.MachineRoleID, RoleKey: requirement.RoleKey,
			MediaAssetID: requirement.MediaAssetID, ContentVersionID: requirement.ContentVersionID,
			AssetPolicy: asset.AssetPolicy, ContentSHA256: requirement.ContentHash,
			SizeBytes: requirement.SizeBytes, Required: requirement.Required,
		}
		if asset.AssetPolicy == store.MediaPolicyReferenceOnly {
			if manifest.ExportMode == ExportSelfContained && requirement.Required {
				return fmt.Errorf("%w: required media %s is REFERENCE_ONLY and cannot be included in a self-contained capsule", domain.ErrConflict, requirement.MediaAssetID)
			}
			manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("Media %s (%s) is REFERENCE_ONLY and must be supplied externally on the replacement host.", requirement.MediaAssetID, requirement.RoleKey))
		} else {
			if err := s.addVaultObject(ctx, objects, requirement.ContentHash, requirement.SizeBytes, requirement.Required, "media:"+requirement.MediaAssetID+":"+requirement.ContentVersionID, manifest.ExportMode); err != nil {
				return err
			}
			item.ContentIncluded = manifest.ExportMode == ExportSelfContained
		}
		manifest.Media = append(manifest.Media, item)
	}
	return nil
}

func (s *Service) collectExecutionEnvironments(ctx context.Context, manifest *Manifest, revisionID string, objects map[string]*ObjectEntry) error {
	environments, err := s.store.ListExecutionEnvironmentManifests(ctx, revisionID)
	if err != nil {
		return err
	}
	for _, environment := range environments {
		record := ExecutionEnvironmentRecord{
			ManifestID: environment.ID, MachineRoleID: cloneString(environment.MachineRoleID),
			ContentSHA256: environment.ContentSHA256, Manifest: environment.Manifest,
		}
		for _, asset := range environment.Manifest.Assets {
			switch asset.CapturePolicy {
			case executionenv.CaptureContentBound:
				if asset.SizeBytes == nil {
					return fmt.Errorf("%w: execution environment asset %s has no content size", domain.ErrConflict, asset.Key)
			}
				if err := s.addVaultObject(ctx, objects, asset.ContentHash, *asset.SizeBytes, true, "environment:"+environment.Manifest.EnvironmentKey+":asset:"+asset.Key, manifest.ExportMode); err != nil {
					return err
				}
			case executionenv.CaptureReferenceOnly:
				manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("Execution environment %s asset %s is REFERENCE_ONLY (%s).", environment.Manifest.EnvironmentKey, asset.Key, asset.Locator))
			}
		}

		latest, err := s.store.GetLatestExecutionEnvironmentSnapshot(ctx, environment.ID)
		if errors.Is(err, domain.ErrNotFound) {
			manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("Execution environment %s has no captured environment snapshot; readiness must be re-established on the replacement host.", environment.Manifest.EnvironmentKey))
		} else if err != nil {
			return err
		} else {
			record.LatestSnapshot = &ExecutionEnvironmentSnapshot{SnapshotID: latest.ID, ContentSHA256: latest.ContentSHA256, Snapshot: latest.Snapshot}
			if latest.Snapshot.CaptureStatus != executionenv.SnapshotComplete {
				manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("Execution environment %s snapshot status is %s; replacement-host readiness must remain blocked until unresolved state is reviewed.", environment.Manifest.EnvironmentKey, latest.Snapshot.CaptureStatus))
			}
			for _, item := range latest.Snapshot.Items {
				switch item.Portability {
				case executionenv.SnapshotContentBound:
					if item.SizeBytes == nil {
						return fmt.Errorf("%w: execution environment snapshot item %s has no content size", domain.ErrConflict, item.Key)
					}
					if err := s.addVaultObject(ctx, objects, item.ContentHash, *item.SizeBytes, true, "environment:"+environment.Manifest.EnvironmentKey+":snapshot:"+item.Key, manifest.ExportMode); err != nil {
						return err
					}
				case executionenv.SnapshotReferenceOnly:
					manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("Execution environment %s snapshot item %s is REFERENCE_ONLY (%s).", environment.Manifest.EnvironmentKey, item.Key, item.Locator))
				case executionenv.SnapshotDescriptiveOnly:
					manifest.Warnings = append(manifest.Warnings, fmt.Sprintf("Execution environment %s snapshot item %s is descriptive evidence only and has no portable bytes.", environment.Manifest.EnvironmentKey, item.Key))
				}
			}
		}
		manifest.ExecutionEnvironments = append(manifest.ExecutionEnvironments, record)
	}
	return nil
}

type installedExtension struct {
	pkg          store.ExtensionPackage
	installation store.ExtensionInstallation
	manifest     extension.Manifest
	canonical    []byte
}

func (s *Service) collectExtensions(ctx context.Context, capsule *Manifest, runtimeManifest snapshot.Manifest, objects map[string]*ObjectEntry) error {
	usedCapabilities := map[string]struct{}{}
	for _, cue := range runtimeManifest.Cues {
		for _, action := range cue.Actions {
			if key := strings.TrimSpace(action.CapabilityKey); key != "" {
				usedCapabilities[key] = struct{}{}
			}
		}
	}
	for _, output := range runtimeManifest.Outputs {
		if key := strings.TrimSpace(output.CapabilityKey); key != "" {
			usedCapabilities[key] = struct{}{}
		}
	}

	packages, err := s.store.ListExtensionPackages(ctx, "")
	if err != nil {
		return err
	}
	installedByID := map[string]installedExtension{}
	for _, pkg := range packages {
		installation, err := s.store.GetExtensionInstallationByPackageID(ctx, pkg.PackageID)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		parsed, canonical, err := extension.ParseManifest(pkg.ManifestJSON)
		if err != nil {
			return fmt.Errorf("parse installed extension %s: %w", pkg.ExtensionID, err)
		}
		digest := sha256.Sum256(canonical)
		if !strings.EqualFold(hex.EncodeToString(digest[:]), pkg.ManifestSHA256) {
			return fmt.Errorf("%w: extension %s stored manifest hash mismatch", domain.ErrConflict, pkg.ExtensionID)
		}
		if previous, exists := installedByID[pkg.ExtensionID]; exists && previous.pkg.PackageID != pkg.PackageID {
			return fmt.Errorf("%w: multiple installed packages found for extension %s", domain.ErrConflict, pkg.ExtensionID)
		}
		installedByID[pkg.ExtensionID] = installedExtension{pkg: pkg, installation: installation, manifest: parsed, canonical: canonical}
	}

	selected := map[string]string{}
	requiredCaps := map[string][]string{}
	for extensionID, item := range installedByID {
		for _, capability := range item.manifest.Capabilities {
			if _, used := usedCapabilities[capability]; used {
				selected[extensionID] = "CAPABILITY_USE"
				requiredCaps[extensionID] = append(requiredCaps[extensionID], capability)
			}
		}
	}
	queue := make([]string, 0, len(selected))
	for extensionID := range selected {
		queue = append(queue, extensionID)
	}
	for index := 0; index < len(queue); index++ {
		parentID := queue[index]
		parent := installedByID[parentID]
		for _, dependency := range parent.manifest.Dependencies {
			dependencyItem, ok := installedByID[dependency.ExtensionID]
			if !ok {
				return fmt.Errorf("%w: extension %s requires installed dependency %s", domain.ErrConflict, parentID, dependency.ExtensionID)
			}
			_ = dependencyItem
			if _, exists := selected[dependency.ExtensionID]; !exists {
				selected[dependency.ExtensionID] = "DEPENDENCY:" + parentID
				queue = append(queue, dependency.ExtensionID)
			}
		}
	}

	ids := make([]string, 0, len(selected))
	for extensionID := range selected {
		ids = append(ids, extensionID)
	}
	sort.Strings(ids)
	for _, extensionID := range ids {
		item := installedByID[extensionID]
		softwarePackage, err := s.store.GetSoftwarePackage(ctx, item.pkg.PackageID)
		if err != nil {
			return fmt.Errorf("load extension software package %s: %w", extensionID, err)
		}
		if err := s.addVaultObject(ctx, objects, softwarePackage.ContentHash, softwarePackage.SizeBytes, true, "extension:"+extensionID+"@"+item.pkg.Version, capsule.ExportMode); err != nil {
			return err
		}
		capsule.Extensions = append(capsule.Extensions, ExtensionRequirement{
			PackageID: item.pkg.PackageID, ExtensionID: item.pkg.ExtensionID, Version: item.pkg.Version,
			Kind: item.pkg.Kind, Source: item.pkg.Source, ManifestSHA256: item.pkg.ManifestSHA256,
			Manifest: append([]byte(nil), item.canonical...), RequiredCapabilities: requiredCaps[extensionID],
			Reason: selected[extensionID], ContentIncluded: capsule.ExportMode == ExportSelfContained,
			Software: SoftwarePackageRecord{
				ProductID: softwarePackage.ProductID, Version: softwarePackage.Version,
				Platform: softwarePackage.Platform, Architecture: softwarePackage.Architecture,
				MinAPIVersion: softwarePackage.MinAPIVersion, MaxAPIVersion: softwarePackage.MaxAPIVersion,
				ContentSHA256: softwarePackage.ContentHash, SizeBytes: softwarePackage.SizeBytes,
				OriginalFilename: softwarePackage.OriginalFilename, SigningStatus: softwarePackage.SigningStatus,
				NotarizationStatus: softwarePackage.NotarizationStatus, ReleaseChannel: softwarePackage.ReleaseChannel,
			},
		})
	}
	return nil
}

func (s *Service) addVaultObject(ctx context.Context, objects map[string]*ObjectEntry, contentHash string, sizeBytes int64, required bool, purpose string, mode ExportMode) error {
	contentHash = strings.ToLower(strings.TrimSpace(contentHash))
	file, object, err := s.vault.OpenObject(ctx, contentHash)
	if err != nil {
		return fmt.Errorf("verify Vault object %s for %s: %w", contentHash, purpose, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close verified Vault object %s: %w", contentHash, err)
	}
	if object.SizeBytes != sizeBytes {
		return fmt.Errorf("%w: Vault object %s size %d does not match required size %d", domain.ErrConflict, contentHash, object.SizeBytes, sizeBytes)
	}
	if existing, ok := objects[contentHash]; ok {
		if existing.SizeBytes != sizeBytes {
			return fmt.Errorf("%w: capsule object %s has conflicting sizes", domain.ErrConflict, contentHash)
		}
		existing.Required = existing.Required || required
		existing.Purposes = append(existing.Purposes, purpose)
		existing.Purposes = normalizeStringSet(existing.Purposes)
		return nil
	}
	entry := &ObjectEntry{
		ContentSHA256: contentHash, SizeBytes: sizeBytes, Required: required,
		Included: mode == ExportSelfContained, Purposes: []string{purpose},
	}
	if entry.Included {
		entry.ArchivePath = objectArchivePath(contentHash)
	}
	objects[contentHash] = entry
	return nil
}

func (s *Service) copyVaultObject(ctx context.Context, stagingRoot string, object ObjectEntry) error {
	file, metadata, err := s.vault.OpenObject(ctx, object.ContentSHA256)
	if err != nil {
		return fmt.Errorf("open capsule source object %s: %w", object.ContentSHA256, err)
	}
	defer file.Close()
	if metadata.SizeBytes != object.SizeBytes {
		return fmt.Errorf("capsule source object %s size changed before export", object.ContentSHA256)
	}
	targetPath := filepath.Join(stagingRoot, filepath.FromSlash(object.ArchivePath))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return fmt.Errorf("create capsule object directory: %w", err)
	}
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("create capsule object %s: %w", object.ContentSHA256, err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(target, hasher), file)
	if syncErr := target.Sync(); copyErr == nil && syncErr != nil {
		copyErr = syncErr
	}
	if closeErr := target.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("copy capsule object %s: %w", object.ContentSHA256, copyErr)
	}
	if written != object.SizeBytes || hex.EncodeToString(hasher.Sum(nil)) != object.ContentSHA256 {
		return fmt.Errorf("capsule object %s changed during export", object.ContentSHA256)
	}
	return nil
}

func verifyFile(path, expectedHash string, expectedSize int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != expectedSize {
		return fmt.Errorf("file size/type mismatch")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, file)
	if err != nil {
		return err
	}
	if written != expectedSize || hex.EncodeToString(hasher.Sum(nil)) != expectedHash {
		return fmt.Errorf("file content hash mismatch")
	}
	return nil
}

func writeSyncedFile(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
