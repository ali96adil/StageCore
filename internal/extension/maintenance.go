package extension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	UpdateDirectionUpdate   = "UPDATE"
	UpdateDirectionRollback = "ROLLBACK"

	MaintenanceCleanupIncomplete = "OLD_PAYLOAD_CLEANUP_INCOMPLETE"
)

var (
	ErrUpdateDependenciesRequired = errors.New("extension update dependencies must be installed first")
	ErrUpdatePlanBlocked          = errors.New("extension update or rollback plan is blocked")
	ErrUpdateTargetIdentity       = errors.New("extension update target must preserve extension identity and kind")
	ErrUpdateTargetVersion        = errors.New("extension update target must use a different semantic version")
)

type UpdatePlan struct {
	InstallationID string               `json:"installation_id"`
	ExtensionID    string               `json:"extension_id"`
	Direction      string               `json:"direction,omitempty"`
	CurrentPackage string               `json:"current_package_id"`
	CurrentVersion string               `json:"current_version"`
	TargetPackage  string               `json:"target_package_id"`
	TargetVersion  string               `json:"target_version"`
	Status         InstallPlanStatus    `json:"status"`
	Steps          []InstallPlanStep    `json:"steps"`
	Blockers       []InstallPlanBlocker `json:"blockers"`
	Warnings       []InstallPlanWarning `json:"warnings"`
}

type UpdatePlanError struct {
	Cause error
	Plan  UpdatePlan
}

func (e *UpdatePlanError) Error() string {
	if e == nil || e.Cause == nil {
		return "extension update plan is not ready"
	}
	return e.Cause.Error()
}
func (e *UpdatePlanError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type UpdateResult struct {
	InstallationID string `json:"installation_id"`
	ExtensionID    string `json:"extension_id"`
	Direction      string `json:"direction"`
	FromPackageID  string `json:"from_package_id"`
	FromVersion    string `json:"from_version"`
	ToPackageID    string `json:"to_package_id"`
	ToVersion      string `json:"to_version"`
	CleanupWarning string `json:"cleanup_warning,omitempty"`
}

type RepairResult struct {
	InstallationID string `json:"installation_id"`
	PackageID      string `json:"package_id"`
	ExtensionID    string `json:"extension_id"`
	Version        string `json:"version"`
	AlreadyHealthy bool   `json:"already_healthy"`
	PayloadRepaired bool   `json:"payload_repaired"`
}

func (i *Installer) PlanUpdate(ctx context.Context, installationID, targetPackageID string) (UpdatePlan, error) {
	if i == nil || i.library == nil || i.library.store == nil {
		return UpdatePlan{}, fmt.Errorf("extension installer is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	targetPackageID = strings.TrimSpace(targetPackageID)
	if installationID == "" || targetPackageID == "" {
		return UpdatePlan{}, fmt.Errorf("installation ID and target package ID are required")
	}
	current, err := i.Get(ctx, installationID)
	if err != nil {
		return UpdatePlan{}, err
	}
	target, err := i.library.Get(ctx, targetPackageID)
	if err != nil {
		return UpdatePlan{}, err
	}
	plan := UpdatePlan{
		InstallationID: installationID,
		ExtensionID: current.ExtensionID,
		CurrentPackage: current.PackageID,
		CurrentVersion: current.Version,
		TargetPackage: target.PackageID,
		TargetVersion: target.Manifest.Version,
		Status: InstallPlanBlocked,
		Steps: []InstallPlanStep{},
		Blockers: []InstallPlanBlocker{},
		Warnings: []InstallPlanWarning{},
	}
	if target.Manifest.ExtensionID != current.ExtensionID || target.Manifest.Kind != current.Kind {
		plan.Blockers = append(plan.Blockers, InstallPlanBlocker{Code: "UPDATE_TARGET_IDENTITY_MISMATCH", ExtensionID: target.Manifest.ExtensionID, Detail: ErrUpdateTargetIdentity.Error()})
		return plan, nil
	}
	comparison := compareSemanticVersions(target.Manifest.Version, current.Version)
	if comparison == 0 {
		plan.Blockers = append(plan.Blockers, InstallPlanBlocker{Code: "UPDATE_TARGET_VERSION_UNCHANGED", ExtensionID: current.ExtensionID, InstalledVersion: current.Version, Detail: ErrUpdateTargetVersion.Error()})
		return plan, nil
	}
	if comparison > 0 {
		plan.Direction = UpdateDirectionUpdate
	} else {
		plan.Direction = UpdateDirectionRollback
	}
	if !target.Compatible {
		plan.Blockers = append(plan.Blockers, InstallPlanBlocker{Code: "UPDATE_TARGET_INCOMPATIBLE", ExtensionID: current.ExtensionID, Detail: target.CompatibilityReason})
		return plan, nil
	}
	dependentBlockers, err := i.replacementDependentBlockers(ctx, installationID, current.ExtensionID, target.Manifest.Version)
	if err != nil {
		return UpdatePlan{}, err
	}
	if len(dependentBlockers) != 0 {
		plan.Blockers = append(plan.Blockers, dependentBlockers...)
		return plan, nil
	}

	solver := &dependencySolver{
		installer: i,
		installedCache: map[string]installedDependency{
			current.ExtensionID: {pkg: target, found: true},
		},
		candidateCache: make(map[string][]Package),
	}
	state := newDependencySolveState()
	state.selected[current.ExtensionID] = target
	state.installed[current.ExtensionID] = true
	solved, blocker, err := solver.solve(ctx, state)
	if err != nil {
		return UpdatePlan{}, err
	}
	if blocker != nil {
		plan.Blockers = append(plan.Blockers, *blocker)
		return plan, nil
	}
	if cycle := detectDependencyCycle(current.ExtensionID, solved.edges); len(cycle) != 0 {
		plan.Blockers = append(plan.Blockers, InstallPlanBlocker{Code: "DEPENDENCY_CYCLE", ExtensionID: cycle[0], Detail: strings.Join(cycle, " -> ")})
		return plan, nil
	}
	for _, extensionID := range dependencyInstallOrder(current.ExtensionID, solved.edges) {
		if extensionID == current.ExtensionID || solved.installed[extensionID] {
			continue
		}
		pkg := solved.selected[extensionID]
		plan.Steps = append(plan.Steps, InstallPlanStep{
			Order: len(plan.Steps) + 1,
			PackageID: pkg.PackageID,
			ExtensionID: pkg.Manifest.ExtensionID,
			Version: pkg.Manifest.Version,
			Kind: pkg.Manifest.Kind,
			Source: pkg.Manifest.Source,
		})
	}
	warnings, err := solver.optionalWarnings(ctx, solved)
	if err != nil {
		return UpdatePlan{}, err
	}
	plan.Warnings = warnings
	if len(plan.Steps) != 0 {
		plan.Status = InstallPlanRequiresDependencies
	} else {
		plan.Status = InstallPlanReady
	}
	return plan, nil
}

func (i *Installer) replacementDependentBlockers(ctx context.Context, removingInstallationID, extensionID, targetVersion string) ([]InstallPlanBlocker, error) {
	records, err := i.library.store.ListExtensionInstallations(ctx, "")
	if err != nil {
		return nil, err
	}
	blockers := make([]InstallPlanBlocker, 0)
	for _, record := range records {
		if record.InstallationID == removingInstallationID {
			continue
		}
		pkg, err := i.library.Get(ctx, record.PackageID)
		if err != nil {
			return nil, err
		}
		for _, dependency := range pkg.Manifest.Dependencies {
			if dependency.Optional || dependency.ExtensionID != extensionID || versionInRange(targetVersion, dependency.MinVersion, dependency.MaxVersion) {
				continue
			}
			blockers = append(blockers, InstallPlanBlocker{
				Code: "INSTALLED_DEPENDENT_VERSION_CONFLICT",
				ExtensionID: extensionID,
				RequiredBy: pkg.Manifest.ExtensionID,
				MinVersion: dependency.MinVersion,
				MaxVersion: dependency.MaxVersion,
				InstalledVersion: targetVersion,
				Detail: "target version would violate an installed dependent requirement",
			})
		}
	}
	sort.Slice(blockers, func(a, b int) bool {
		if blockers[a].RequiredBy == blockers[b].RequiredBy {
			return blockers[a].MinVersion < blockers[b].MinVersion
		}
		return blockers[a].RequiredBy < blockers[b].RequiredBy
	})
	return blockers, nil
}

func (s *RuntimeSupervisor) UpdateInstallation(ctx context.Context, installationID, targetPackageID, actor string) (UpdateResult, error) {
	if s == nil || s.installer == nil {
		return UpdateResult{}, fmt.Errorf("extension runtime supervisor is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	targetPackageID = strings.TrimSpace(targetPackageID)
	actor = strings.TrimSpace(actor)
	if installationID == "" || targetPackageID == "" || actor == "" {
		return UpdateResult{}, fmt.Errorf("installation ID, target package ID and actor are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectShow(ctx); err != nil {
		return UpdateResult{}, err
	}
	if err := s.requireStopped(ctx, installationID); err != nil {
		return UpdateResult{}, err
	}
	return s.installer.replaceDisabled(ctx, installationID, targetPackageID, actor)
}

func (s *RuntimeSupervisor) RepairInstallation(ctx context.Context, installationID, actor string) (RepairResult, error) {
	if s == nil || s.installer == nil {
		return RepairResult{}, fmt.Errorf("extension runtime supervisor is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	actor = strings.TrimSpace(actor)
	if installationID == "" || actor == "" {
		return RepairResult{}, fmt.Errorf("installation ID and actor are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rejectShow(ctx); err != nil {
		return RepairResult{}, err
	}
	if err := s.requireStopped(ctx, installationID); err != nil {
		return RepairResult{}, err
	}
	return s.installer.repairDisabled(ctx, installationID)
}

func (s *RuntimeSupervisor) requireStopped(ctx context.Context, installationID string) error {
	lifecycle, err := s.installer.library.store.GetExtensionRuntimeLifecycle(ctx, installationID)
	if err != nil {
		return err
	}
	if s.processes[installationID] != nil || lifecycle.DesiredState != store.ExtensionRuntimeDesiredDisabled || lifecycle.ObservedState != store.ExtensionRuntimeObservedStopped {
		return ErrExtensionRuntimeMustBeDisabled
	}
	return nil
}

func (i *Installer) replaceDisabled(ctx context.Context, installationID, targetPackageID, actor string) (UpdateResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	activeType, err := i.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return UpdateResult{}, err
	}
	if activeType == domain.SessionShow {
		return UpdateResult{}, domain.ErrShowConfigurationLocked
	}
	plan, err := i.PlanUpdate(ctx, installationID, targetPackageID)
	if err != nil {
		return UpdateResult{}, err
	}
	switch plan.Status {
	case InstallPlanReady:
	case InstallPlanRequiresDependencies:
		return UpdateResult{}, &UpdatePlanError{Cause: ErrUpdateDependenciesRequired, Plan: plan}
	default:
		return UpdateResult{}, &UpdatePlanError{Cause: ErrUpdatePlanBlocked, Plan: plan}
	}
	currentRecord, err := i.library.store.GetExtensionInstallation(ctx, installationID)
	if err != nil {
		return UpdateResult{}, err
	}
	currentPkg, err := i.library.Get(ctx, currentRecord.PackageID)
	if err != nil {
		return UpdateResult{}, err
	}
	targetPkg, err := i.library.Get(ctx, targetPackageID)
	if err != nil {
		return UpdateResult{}, err
	}
	oldPath, err := i.absoluteInstalledPath(currentRecord.PayloadRelativePath)
	if err != nil {
		return UpdateResult{}, err
	}
	relativePath := path.Join(targetPkg.Manifest.ExtensionID, targetPkg.Manifest.Version, targetPkg.PackageID, installedPayloadName)
	finalPath, err := i.safeInstalledPath(targetPkg.Manifest.ExtensionID, targetPkg.Manifest.Version, targetPkg.PackageID)
	if err != nil {
		return UpdateResult{}, err
	}
	hash, size, created, err := i.materializePackage(ctx, targetPkg.PackageID, finalPath, "update")
	if err != nil {
		return UpdateResult{}, err
	}
	cleanupCandidate := func() {
		if created {
			_ = os.Remove(finalPath)
			i.removeEmptyInstalledParents(filepath.Dir(finalPath))
		}
	}
	replaced, err := i.library.store.ReplaceExtensionInstallation(ctx, installationID, store.RegisterExtensionInstallationParams{
		PackageID: targetPkg.PackageID,
		LifecycleState: store.ExtensionInstallationInstalled,
		PayloadRelativePath: relativePath,
		ContentSHA256: hash,
		SizeBytes: size,
		InstalledBy: actor,
	})
	if err != nil {
		cleanupCandidate()
		return UpdateResult{}, err
	}
	result := UpdateResult{
		InstallationID: installationID,
		ExtensionID: targetPkg.Manifest.ExtensionID,
		Direction: plan.Direction,
		FromPackageID: currentPkg.PackageID,
		FromVersion: currentPkg.Manifest.Version,
		ToPackageID: targetPkg.PackageID,
		ToVersion: targetPkg.Manifest.Version,
	}
	if oldPath != finalPath {
		if err := os.Remove(oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.CleanupWarning = MaintenanceCleanupIncomplete
		} else {
			_ = syncDirectory(filepath.Dir(oldPath))
			i.removeEmptyInstalledParents(filepath.Dir(oldPath))
		}
	}
	if _, err := i.inflateAndVerify(ctx, replaced); err != nil {
		return result, err
	}
	return result, nil
}

func (i *Installer) repairDisabled(ctx context.Context, installationID string) (RepairResult, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	activeType, err := i.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return RepairResult{}, err
	}
	if activeType == domain.SessionShow {
		return RepairResult{}, domain.ErrShowConfigurationLocked
	}
	record, err := i.library.store.GetExtensionInstallation(ctx, installationID)
	if err != nil {
		return RepairResult{}, err
	}
	pkg, err := i.library.Get(ctx, record.PackageID)
	if err != nil {
		return RepairResult{}, err
	}
	expectedRelative := path.Join(pkg.Manifest.ExtensionID, pkg.Manifest.Version, pkg.PackageID, installedPayloadName)
	if path.Clean(record.PayloadRelativePath) != expectedRelative {
		return RepairResult{}, fmt.Errorf("%w: installed payload path does not match immutable package identity", ErrInstalledPayloadIntegrity)
	}
	finalPath, err := i.safeInstalledPath(pkg.Manifest.ExtensionID, pkg.Manifest.Version, pkg.PackageID)
	if err != nil {
		return RepairResult{}, err
	}
	result := RepairResult{InstallationID: installationID, PackageID: pkg.PackageID, ExtensionID: pkg.Manifest.ExtensionID, Version: pkg.Manifest.Version}
	if err := verifyPayload(finalPath, record.ContentSHA256, record.SizeBytes); err == nil {
		result.AlreadyHealthy = true
		return result, nil
	}
	status, err := i.library.software.Get(ctx, pkg.PackageID)
	if err != nil {
		return RepairResult{}, err
	}
	if status.Package.ContentHash != record.ContentSHA256 || status.Package.SizeBytes != record.SizeBytes {
		return RepairResult{}, fmt.Errorf("%w: installation metadata differs from immutable software package", ErrInstalledPayloadIntegrity)
	}
	if info, statErr := os.Lstat(finalPath); statErr == nil {
		if info.IsDir() || (!info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0) {
			return RepairResult{}, fmt.Errorf("%w: installed payload has unsafe file type", ErrInstalledPayloadIntegrity)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return RepairResult{}, fmt.Errorf("inspect installed payload for repair: %w", statErr)
	}
	hash, size, _, err := i.materializePackageReplacing(ctx, pkg.PackageID, finalPath, "repair")
	if err != nil {
		return RepairResult{}, err
	}
	if hash != record.ContentSHA256 || size != record.SizeBytes {
		return RepairResult{}, fmt.Errorf("%w: repaired payload differs from installation metadata", ErrInstalledPayloadIntegrity)
	}
	result.PayloadRepaired = true
	return result, nil
}

func (i *Installer) materializePackage(ctx context.Context, packageID, finalPath, purpose string) (string, int64, bool, error) {
	status, err := i.library.software.Get(ctx, packageID)
	if err != nil {
		return "", 0, false, err
	}
	if info, statErr := os.Lstat(finalPath); statErr == nil {
		if err := verifyPayload(finalPath, status.Package.ContentHash, status.Package.SizeBytes); err != nil {
			return "", 0, false, err
		}
		if !info.Mode().IsRegular() {
			return "", 0, false, ErrInstalledPayloadIntegrity
		}
		return status.Package.ContentHash, status.Package.SizeBytes, false, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", 0, false, statErr
	}
	if err := i.capacity.Admit(i.root, uint64(status.Package.SizeBytes)); err != nil {
		return "", 0, false, err
	}
	return i.copyImmutablePackage(ctx, packageID, finalPath, purpose, false)
}

func (i *Installer) materializePackageReplacing(ctx context.Context, packageID, finalPath, purpose string) (string, int64, bool, error) {
	status, err := i.library.software.Get(ctx, packageID)
	if err != nil {
		return "", 0, false, err
	}
	if err := i.capacity.Admit(i.root, uint64(status.Package.SizeBytes)); err != nil {
		return "", 0, false, err
	}
	return i.copyImmutablePackage(ctx, packageID, finalPath, purpose, true)
}

func (i *Installer) copyImmutablePackage(ctx context.Context, packageID, finalPath, purpose string, replace bool) (string, int64, bool, error) {
	source, openedStatus, err := i.library.software.OpenPackage(ctx, packageID)
	if err != nil {
		return "", 0, false, err
	}
	defer source.Close()
	staged, err := os.CreateTemp(i.stagingRoot, packageID+"-"+purpose+"-*.part")
	if err != nil {
		return "", 0, false, fmt.Errorf("create extension maintenance staging file: %w", err)
	}
	stagedPath := staged.Name()
	promoted := false
	defer func() {
		_ = staged.Close()
		if !promoted {
			_ = os.Remove(stagedPath)
		}
	}()
	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(staged, hasher), source)
	if err != nil {
		return "", 0, false, fmt.Errorf("stage extension maintenance package: %w", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	if size != openedStatus.Package.SizeBytes || hash != openedStatus.Package.ContentHash {
		return "", 0, false, fmt.Errorf("%w: staged maintenance package differs from immutable metadata", ErrInstalledPayloadIntegrity)
	}
	if err := staged.Sync(); err != nil {
		return "", 0, false, err
	}
	if err := staged.Chmod(installedPayloadMode); err != nil {
		return "", 0, false, err
	}
	if err := staged.Close(); err != nil {
		return "", 0, false, err
	}
	if !replace {
		if _, err := os.Lstat(finalPath); err == nil {
			return "", 0, false, fmt.Errorf("extension maintenance target appeared during staging")
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", 0, false, err
		}
	}
	if err := os.Rename(stagedPath, finalPath); err != nil {
		return "", 0, false, fmt.Errorf("atomically promote extension maintenance payload: %w", err)
	}
	promoted = true
	if err := syncDirectory(filepath.Dir(finalPath)); err != nil {
		return "", 0, true, err
	}
	if err := verifyPayload(finalPath, hash, size); err != nil {
		return "", 0, true, err
	}
	return hash, size, true, nil
}
