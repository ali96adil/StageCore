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
	"strings"
	"sync"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/storagehealth"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	installDirectoryMode = 0o750
	installedPayloadMode = 0o440
	installedPayloadName = "payload.pkg"
)

var (
	ErrDifferentPackageInstalled = errors.New("a different package version of this extension is already installed")
	ErrInstalledPayloadIntegrity = errors.New("installed extension payload failed integrity verification")
)

type Installation struct {
	InstallationID      string    `json:"installation_id"`
	PackageID           string    `json:"package_id"`
	ExtensionID         string    `json:"extension_id"`
	Version             string    `json:"version"`
	Kind                Kind      `json:"kind"`
	LifecycleState      string    `json:"lifecycle_state"`
	PayloadRelativePath string    `json:"payload_relative_path"`
	ContentSHA256       string    `json:"content_sha256"`
	SizeBytes           int64     `json:"size_bytes"`
	InstalledBy         string    `json:"installed_by"`
	InstalledAt         time.Time `json:"installed_at"`
}

type InstallerOption func(*Installer)

func WithInstallerCapacityPolicy(policy *storagehealth.Policy) InstallerOption {
	return func(installer *Installer) {
		if policy != nil {
			installer.capacity = policy
		}
	}
}

type Installer struct {
	library       *Library
	root          string
	stagingRoot   string
	installedRoot string
	capacity      *storagehealth.Policy
	mu            sync.Mutex
}

func NewInstaller(library *Library, root string, options ...InstallerOption) (*Installer, error) {
	if library == nil || library.store == nil || library.software == nil {
		return nil, fmt.Errorf("extension installer requires an extension library")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("extension install root is required")
	}
	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("resolve extension install root: %w", err)
	}
	installer := &Installer{
		library: library,
		root: absoluteRoot,
		stagingRoot: filepath.Join(absoluteRoot, "staging"),
		installedRoot: filepath.Join(absoluteRoot, "installed"),
		capacity: storagehealth.NewPolicy(0, 0),
	}
	for _, option := range options {
		option(installer)
	}
	for _, dir := range []string{installer.root, installer.stagingRoot, installer.installedRoot} {
		if err := ensureManagedDirectory(dir); err != nil {
			return nil, err
		}
	}
	if err := installer.cleanStaging(); err != nil {
		return nil, err
	}
	return installer, nil
}

func (i *Installer) Install(ctx context.Context, packageID, actor string) (Installation, error) {
	if i == nil || i.library == nil {
		return Installation{}, fmt.Errorf("extension installer is unavailable")
	}
	packageID = strings.TrimSpace(packageID)
	actor = strings.TrimSpace(actor)
	if packageID == "" || actor == "" {
		return Installation{}, fmt.Errorf("package ID and installation actor are required")
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	activeType, err := i.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return Installation{}, err
	}
	if activeType == domain.SessionShow {
		return Installation{}, domain.ErrShowConfigurationLocked
	}

	pkg, err := i.library.Get(ctx, packageID)
	if err != nil {
		return Installation{}, err
	}
	if !pkg.Compatible {
		return Installation{}, fmt.Errorf("extension package is incompatible: %s", pkg.CompatibilityReason)
	}

	existingForExtension, err := i.library.store.ListExtensionInstallations(ctx, pkg.Manifest.ExtensionID)
	if err != nil {
		return Installation{}, err
	}
	for _, existing := range existingForExtension {
		if existing.PackageID != packageID {
			return Installation{}, ErrDifferentPackageInstalled
		}
		return i.inflateAndVerify(ctx, existing)
	}

	status, err := i.library.software.Get(ctx, packageID)
	if err != nil {
		return Installation{}, err
	}
	relativePath := path.Join(pkg.Manifest.ExtensionID, pkg.Manifest.Version, packageID, installedPayloadName)
	finalPath, err := i.safeInstalledPath(pkg.Manifest.ExtensionID, pkg.Manifest.Version, packageID)
	if err != nil {
		return Installation{}, err
	}

	if _, err := os.Lstat(finalPath); err == nil {
		if err := verifyPayload(finalPath, status.Package.ContentHash, status.Package.SizeBytes); err != nil {
			return Installation{}, err
		}
		return i.persistInstalled(ctx, pkg, actor, relativePath, status.Package.ContentHash, status.Package.SizeBytes)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Installation{}, fmt.Errorf("inspect existing installed payload: %w", err)
	}

	if err := i.capacity.Admit(i.root, uint64(status.Package.SizeBytes)); err != nil {
		return Installation{}, err
	}

	source, openedStatus, err := i.library.software.OpenPackage(ctx, packageID)
	if err != nil {
		return Installation{}, err
	}
	defer source.Close()

	staged, err := os.CreateTemp(i.stagingRoot, packageID+"-*.part")
	if err != nil {
		return Installation{}, fmt.Errorf("create extension staging file: %w", err)
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
		return Installation{}, fmt.Errorf("stage extension package: %w", err)
	}
	contentHash := hex.EncodeToString(hasher.Sum(nil))
	if size != openedStatus.Package.SizeBytes || contentHash != openedStatus.Package.ContentHash {
		return Installation{}, fmt.Errorf("%w: staged package hash or size differs from immutable software metadata", ErrInstalledPayloadIntegrity)
	}
	if err := staged.Sync(); err != nil {
		return Installation{}, fmt.Errorf("sync extension staging file: %w", err)
	}
	if err := staged.Chmod(installedPayloadMode); err != nil {
		return Installation{}, fmt.Errorf("make staged extension payload non-writable and non-executable: %w", err)
	}
	if err := staged.Close(); err != nil {
		return Installation{}, fmt.Errorf("close extension staging file: %w", err)
	}

	if _, err := os.Lstat(finalPath); err == nil {
		return Installation{}, fmt.Errorf("installed payload appeared during staging")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Installation{}, fmt.Errorf("inspect final extension payload path: %w", err)
	}
	if err := os.Rename(stagedPath, finalPath); err != nil {
		return Installation{}, fmt.Errorf("atomically promote extension package: %w", err)
	}
	promoted = true
	if err := syncDirectory(filepath.Dir(finalPath)); err != nil {
		return Installation{}, fmt.Errorf("sync installed extension directory: %w", err)
	}
	if err := verifyPayload(finalPath, contentHash, size); err != nil {
		return Installation{}, err
	}
	return i.persistInstalled(ctx, pkg, actor, relativePath, contentHash, size)
}

func (i *Installer) Get(ctx context.Context, installationID string) (Installation, error) {
	record, err := i.library.store.GetExtensionInstallation(ctx, strings.TrimSpace(installationID))
	if err != nil {
		return Installation{}, err
	}
	return i.inflateAndVerify(ctx, record)
}

func (i *Installer) List(ctx context.Context, extensionID string) ([]Installation, error) {
	records, err := i.library.store.ListExtensionInstallations(ctx, strings.TrimSpace(extensionID))
	if err != nil {
		return nil, err
	}
	items := make([]Installation, 0, len(records))
	for _, record := range records {
		item, err := i.inflateAndVerify(ctx, record)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (i *Installer) persistInstalled(ctx context.Context, pkg Package, actor, relativePath, contentHash string, size int64) (Installation, error) {
	record, err := i.library.store.RegisterExtensionInstallation(ctx, store.RegisterExtensionInstallationParams{
		PackageID: pkg.PackageID,
		LifecycleState: store.ExtensionInstallationInstalled,
		PayloadRelativePath: relativePath,
		ContentSHA256: contentHash,
		SizeBytes: size,
		InstalledBy: actor,
	})
	if err != nil {
		if existing, getErr := i.library.store.GetExtensionInstallationByPackageID(ctx, pkg.PackageID); getErr == nil {
			return i.inflateAndVerify(ctx, existing)
		}
		return Installation{}, err
	}
	return makeInstallation(record, pkg), nil
}

func (i *Installer) inflateAndVerify(ctx context.Context, record store.ExtensionInstallation) (Installation, error) {
	pkg, err := i.library.Get(ctx, record.PackageID)
	if err != nil {
		return Installation{}, err
	}
	absolutePath, err := i.absoluteInstalledPath(record.PayloadRelativePath)
	if err != nil {
		return Installation{}, err
	}
	if err := verifyPayload(absolutePath, record.ContentSHA256, record.SizeBytes); err != nil {
		return Installation{}, err
	}
	return makeInstallation(record, pkg), nil
}

func makeInstallation(record store.ExtensionInstallation, pkg Package) Installation {
	return Installation{
		InstallationID: record.InstallationID,
		PackageID: record.PackageID,
		ExtensionID: pkg.Manifest.ExtensionID,
		Version: pkg.Manifest.Version,
		Kind: pkg.Manifest.Kind,
		LifecycleState: record.LifecycleState,
		PayloadRelativePath: record.PayloadRelativePath,
		ContentSHA256: record.ContentSHA256,
		SizeBytes: record.SizeBytes,
		InstalledBy: record.InstalledBy,
		InstalledAt: record.InstalledAt,
	}
}

func (i *Installer) safeInstalledPath(extensionID, version, packageID string) (string, error) {
	parent := i.installedRoot
	if err := inspectManagedDirectory(parent); err != nil {
		return "", err
	}
	for _, segment := range []string{extensionID, version, packageID} {
		if !safePathSegment(segment) {
			return "", fmt.Errorf("unsafe installed extension path segment")
		}
		parent = filepath.Join(parent, segment)
		if err := ensureManagedDirectory(parent); err != nil {
			return "", err
		}
	}
	return filepath.Join(parent, installedPayloadName), nil
}

func (i *Installer) absoluteInstalledPath(relative string) (string, error) {
	relative = path.Clean(strings.TrimSpace(relative))
	if relative == "." || path.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("%w: stored installed payload path is unsafe", ErrInstalledPayloadIntegrity)
	}
	parts := strings.Split(relative, "/")
	if len(parts) < 2 || parts[len(parts)-1] != installedPayloadName {
		return "", fmt.Errorf("%w: stored installed payload path shape is invalid", ErrInstalledPayloadIntegrity)
	}
	current := i.installedRoot
	if err := inspectManagedDirectory(current); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInstalledPayloadIntegrity, err)
	}
	for _, segment := range parts[:len(parts)-1] {
		if !safePathSegment(segment) {
			return "", fmt.Errorf("%w: stored installed payload path segment is unsafe", ErrInstalledPayloadIntegrity)
		}
		current = filepath.Join(current, segment)
		if err := inspectManagedDirectory(current); err != nil {
			return "", fmt.Errorf("%w: %v", ErrInstalledPayloadIntegrity, err)
		}
	}
	absolute := filepath.Join(current, installedPayloadName)
	resolvedRelative, err := filepath.Rel(i.installedRoot, absolute)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: installed payload escaped managed root", ErrInstalledPayloadIntegrity)
	}
	return absolute, nil
}

func (i *Installer) cleanStaging() error {
	if err := inspectManagedDirectory(i.stagingRoot); err != nil {
		return err
	}
	entries, err := os.ReadDir(i.stagingRoot)
	if err != nil {
		return fmt.Errorf("read extension staging directory: %w", err)
	}
	for _, entry := range entries {
		fullPath := filepath.Join(i.stagingRoot, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			return fmt.Errorf("inspect stale extension staging entry: %w", err)
		}
		if !info.Mode().IsRegular() || !strings.HasSuffix(entry.Name(), ".part") {
			return fmt.Errorf("unexpected entry in managed extension staging directory: %s", entry.Name())
		}
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("remove stale extension staging file: %w", err)
		}
	}
	return nil
}

func safePathSegment(segment string) bool {
	return segment != "" && filepath.Base(segment) == segment && !strings.ContainsAny(segment, `/\\`)
}

func ensureManagedDirectory(dir string) error {
	if err := os.Mkdir(dir, installDirectoryMode); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create managed extension directory %q: %w", dir, err)
	}
	return inspectManagedDirectory(dir)
}

func inspectManagedDirectory(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect managed extension directory %q: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("managed extension path %q must be a real directory, not a symlink", dir)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("managed extension directory %q must not be group/world writable", dir)
	}
	return nil
}

func verifyPayload(filename, expectedHash string, expectedSize int64) error {
	initialInfo, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInstalledPayloadIntegrity, err)
	}
	if err := validatePayloadInfo(initialInfo, expectedSize); err != nil {
		return err
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("%w: open payload: %v", ErrInstalledPayloadIntegrity, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(initialInfo, openedInfo) {
		return fmt.Errorf("%w: installed payload changed while opening", ErrInstalledPayloadIntegrity)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("%w: hash payload: %v", ErrInstalledPayloadIntegrity, err)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != strings.ToLower(strings.TrimSpace(expectedHash)) {
		return fmt.Errorf("%w: installed payload SHA-256 mismatch", ErrInstalledPayloadIntegrity)
	}
	finalInfo, err := os.Lstat(filename)
	if err != nil || !os.SameFile(openedInfo, finalInfo) {
		return fmt.Errorf("%w: installed payload changed during verification", ErrInstalledPayloadIntegrity)
	}
	return validatePayloadInfo(finalInfo, expectedSize)
}

func validatePayloadInfo(info os.FileInfo, expectedSize int64) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: installed payload is not a regular file", ErrInstalledPayloadIntegrity)
	}
	if info.Mode().Perm()&0o333 != 0 {
		return fmt.Errorf("%w: installed payload must remain non-writable and non-executable before activation", ErrInstalledPayloadIntegrity)
	}
	if info.Size() != expectedSize {
		return fmt.Errorf("%w: installed payload size mismatch", ErrInstalledPayloadIntegrity)
	}
	return nil
}

func syncDirectory(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
