package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

var (
	ErrExtensionRequiredByInstalled = errors.New("extension is required by another installed extension")
	ErrExtensionRuntimeMustBeDisabled = errors.New("extension runtime must be disabled and stopped before uninstall")
)

type UninstallDependencyBlocker struct {
	ExtensionID       string `json:"extension_id"`
	RequiredBy        string `json:"required_by"`
	RequiredByVersion string `json:"required_by_version"`
	MinVersion        string `json:"min_version,omitempty"`
	MaxVersion        string `json:"max_version,omitempty"`
}

type UninstallDependencyError struct {
	Blockers []UninstallDependencyBlocker `json:"blockers"`
}

func (e *UninstallDependencyError) Error() string { return ErrExtensionRequiredByInstalled.Error() }
func (e *UninstallDependencyError) Unwrap() error { return ErrExtensionRequiredByInstalled }

type UninstallResult struct {
	InstallationID       string `json:"installation_id"`
	PackageID            string `json:"package_id"`
	ExtensionID          string `json:"extension_id"`
	Version              string `json:"version"`
	PayloadRemoved       bool   `json:"payload_removed"`
	PayloadAlreadyAbsent bool   `json:"payload_already_absent,omitempty"`
	CleanupWarning       string `json:"cleanup_warning,omitempty"`
}

// Uninstall serializes against Enable/Disable with the supervisor mutex, then
// against Install with the installer mutex. It deliberately refuses to stop a
// running extension implicitly: the operator must first establish a durable
// DISABLED/STOPPED runtime state.
func (s *RuntimeSupervisor) Uninstall(ctx context.Context, installationID, actor string) (UninstallResult, error) {
	if s == nil || s.installer == nil {
		return UninstallResult{}, fmt.Errorf("extension runtime supervisor is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	actor = strings.TrimSpace(actor)
	if installationID == "" || actor == "" {
		return UninstallResult{}, fmt.Errorf("installation ID and actor are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.rejectShow(ctx); err != nil {
		return UninstallResult{}, err
	}
	lifecycle, err := s.installer.library.store.GetExtensionRuntimeLifecycle(ctx, installationID)
	if err != nil {
		return UninstallResult{}, err
	}
	if s.processes[installationID] != nil || lifecycle.DesiredState != store.ExtensionRuntimeDesiredDisabled || lifecycle.ObservedState != store.ExtensionRuntimeObservedStopped {
		return UninstallResult{}, ErrExtensionRuntimeMustBeDisabled
	}
	return s.installer.uninstallDisabled(ctx, installationID)
}

// uninstallDisabled is intentionally package-private: callers must establish
// the runtime precondition through RuntimeSupervisor.Uninstall.
func (i *Installer) uninstallDisabled(ctx context.Context, installationID string) (UninstallResult, error) {
	if i == nil || i.library == nil || i.library.store == nil {
		return UninstallResult{}, fmt.Errorf("extension installer is unavailable")
	}
	installationID = strings.TrimSpace(installationID)
	if installationID == "" {
		return UninstallResult{}, fmt.Errorf("installation ID is required")
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	activeType, err := i.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return UninstallResult{}, err
	}
	if activeType == domain.SessionShow {
		return UninstallResult{}, domain.ErrShowConfigurationLocked
	}

	record, err := i.library.store.GetExtensionInstallation(ctx, installationID)
	if err != nil {
		return UninstallResult{}, err
	}
	pkg, err := i.library.Get(ctx, record.PackageID)
	if err != nil {
		return UninstallResult{}, err
	}
	blockers, err := i.reverseDependencyBlockers(ctx, installationID, pkg.Manifest.ExtensionID)
	if err != nil {
		return UninstallResult{}, err
	}
	if len(blockers) != 0 {
		return UninstallResult{}, &UninstallDependencyError{Blockers: blockers}
	}

	absolutePath, err := i.absoluteInstalledPath(record.PayloadRelativePath)
	if err != nil {
		return UninstallResult{}, err
	}
	payloadAbsent := false
	if info, statErr := os.Lstat(absolutePath); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return UninstallResult{}, fmt.Errorf("inspect installed payload before uninstall: %w", statErr)
		}
		payloadAbsent = true
	} else if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return UninstallResult{}, fmt.Errorf("%w: installed payload has unsafe file type", ErrInstalledPayloadIntegrity)
	}

	if err := i.library.store.DeleteExtensionInstallation(ctx, installationID); err != nil {
		return UninstallResult{}, err
	}

	result := UninstallResult{
		InstallationID: installationID,
		PackageID: record.PackageID,
		ExtensionID: pkg.Manifest.ExtensionID,
		Version: pkg.Manifest.Version,
		PayloadAlreadyAbsent: payloadAbsent,
	}
	if payloadAbsent {
		result.PayloadRemoved = true
		return result, nil
	}
	if err := os.Remove(absolutePath); err != nil {
		// Durable state is already safely removed. Keep this a successful
		// uninstall with an explicit cleanup warning; the inert immutable file
		// cannot be enabled without an installation record and can be reused by
		// a later install if it still verifies.
		result.CleanupWarning = "PAYLOAD_CLEANUP_INCOMPLETE"
		return result, nil
	}
	result.PayloadRemoved = true
	_ = syncDirectory(filepath.Dir(absolutePath))
	i.removeEmptyInstalledParents(filepath.Dir(absolutePath))
	return result, nil
}

func (i *Installer) reverseDependencyBlockers(ctx context.Context, removingInstallationID, extensionID string) ([]UninstallDependencyBlocker, error) {
	installations, err := i.library.store.ListExtensionInstallations(ctx, "")
	if err != nil {
		return nil, err
	}
	blockers := make([]UninstallDependencyBlocker, 0)
	for _, installed := range installations {
		if installed.InstallationID == removingInstallationID {
			continue
		}
		pkg, err := i.library.Get(ctx, installed.PackageID)
		if err != nil {
			return nil, err
		}
		for _, dependency := range pkg.Manifest.Dependencies {
			if dependency.Optional || dependency.ExtensionID != extensionID {
				continue
			}
			blockers = append(blockers, UninstallDependencyBlocker{
				ExtensionID: extensionID,
				RequiredBy: pkg.Manifest.ExtensionID,
				RequiredByVersion: pkg.Manifest.Version,
				MinVersion: dependency.MinVersion,
				MaxVersion: dependency.MaxVersion,
			})
		}
	}
	sort.Slice(blockers, func(a, b int) bool {
		if blockers[a].RequiredBy == blockers[b].RequiredBy {
			return blockers[a].RequiredByVersion < blockers[b].RequiredByVersion
		}
		return blockers[a].RequiredBy < blockers[b].RequiredBy
	})
	return blockers, nil
}

func (i *Installer) removeEmptyInstalledParents(start string) {
	current := filepath.Clean(start)
	root := filepath.Clean(i.installedRoot)
	for current != root && current != "." && current != string(filepath.Separator) {
		if err := os.Remove(current); err != nil {
			return
		}
		current = filepath.Dir(current)
	}
	_ = syncDirectory(root)
}
