package extension

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

const (
	ExtensionSetFormatV1        = "stagecore-extension-set-v1"
	ExtensionSetSchemaVersion   = 1
	MaxExtensionSetManifestSize = 1 << 20
)

var (
	ErrExtensionSetInvalid        = errors.New("invalid StageCore extension set manifest")
	ErrExtensionSetRestoreBlocked = errors.New("extension set restore is blocked")
)

type ExtensionSetEntry struct {
	ExtensionID      string `json:"extension_id"`
	Version          string `json:"version"`
	Kind             Kind   `json:"kind"`
	Source           Source `json:"source"`
	ManifestSHA256   string `json:"manifest_sha256"`
	PayloadSHA256    string `json:"payload_sha256"`
	PayloadSizeBytes int64  `json:"payload_size_bytes"`
	Platform         string `json:"platform"`
	Architecture     string `json:"architecture"`
}

type ExtensionSetManifest struct {
	Format        string              `json:"format"`
	SchemaVersion int                 `json:"schema_version"`
	Extensions    []ExtensionSetEntry `json:"extensions"`
}

type ExtensionSetRestoreStatus string

const (
	ExtensionSetRestoreReady   ExtensionSetRestoreStatus = "READY"
	ExtensionSetRestoreBlocked ExtensionSetRestoreStatus = "BLOCKED"
	ExtensionSetRestoreNoop    ExtensionSetRestoreStatus = "NOOP"
)

type ExtensionSetRestoreAction string

const (
	ExtensionSetRestoreInstall          ExtensionSetRestoreAction = "INSTALL"
	ExtensionSetRestoreAlreadyInstalled ExtensionSetRestoreAction = "ALREADY_INSTALLED"
)

type ExtensionSetRestoreBlocker struct {
	Code        string `json:"code"`
	ExtensionID string `json:"extension_id,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

type ExtensionSetRestoreStep struct {
	Order       int                       `json:"order"`
	ExtensionID string                    `json:"extension_id"`
	Version     string                    `json:"version"`
	PackageID   string                    `json:"package_id"`
	Action      ExtensionSetRestoreAction `json:"action"`
}

type ExtensionSetRestorePlan struct {
	Status                    ExtensionSetRestoreStatus    `json:"status"`
	Steps                     []ExtensionSetRestoreStep    `json:"steps"`
	Blockers                  []ExtensionSetRestoreBlocker `json:"blockers"`
	PermissionReviewsRestored bool                         `json:"permission_reviews_restored"`
	RuntimeIntentRestored     bool                         `json:"runtime_intent_restored"`
	NewInstallDesiredState    string                       `json:"new_install_desired_state"`
}

type ExtensionSetRestoreResult struct {
	Plan      ExtensionSetRestorePlan `json:"plan"`
	Installed []Installation          `json:"installed"`
}

type ExtensionSetService struct {
	installer *Installer
}

func NewExtensionSetService(installer *Installer) (*ExtensionSetService, error) {
	if installer == nil || installer.library == nil || installer.library.store == nil || installer.library.software == nil {
		return nil, fmt.Errorf("extension set service requires extension installer")
	}
	return &ExtensionSetService{installer: installer}, nil
}

// Export is a portable content-bound inventory. It intentionally omits local
// database IDs, permission approvals, observed runtime state and enable intent.
func (s *ExtensionSetService) Export(ctx context.Context) (ExtensionSetManifest, []byte, error) {
	if s == nil || s.installer == nil {
		return ExtensionSetManifest{}, nil, fmt.Errorf("extension set service is unavailable")
	}
	installations, err := s.installer.List(ctx, "")
	if err != nil {
		return ExtensionSetManifest{}, nil, err
	}
	entries := make([]ExtensionSetEntry, 0, len(installations))
	for _, installation := range installations {
		pkg, err := s.installer.library.Get(ctx, installation.PackageID)
		if err != nil {
			return ExtensionSetManifest{}, nil, err
		}
		status, err := s.installer.library.software.Get(ctx, installation.PackageID)
		if err != nil {
			return ExtensionSetManifest{}, nil, err
		}
		softwarePackage := status.Package
		if installation.ContentSHA256 != softwarePackage.ContentHash || installation.SizeBytes != softwarePackage.SizeBytes {
			return ExtensionSetManifest{}, nil, fmt.Errorf("%w: installed payload metadata differs from immutable package metadata for %s", ErrInstalledPayloadIntegrity, installation.ExtensionID)
		}
		entries = append(entries, ExtensionSetEntry{
			ExtensionID: installation.ExtensionID,
			Version: installation.Version,
			Kind: pkg.Manifest.Kind,
			Source: pkg.Manifest.Source,
			ManifestSHA256: pkg.ManifestSHA256,
			PayloadSHA256: installation.ContentSHA256,
			PayloadSizeBytes: installation.SizeBytes,
			Platform: softwarePackage.Platform,
			Architecture: softwarePackage.Architecture,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ExtensionID < entries[j].ExtensionID })
	manifest := ExtensionSetManifest{
		Format: ExtensionSetFormatV1,
		SchemaVersion: ExtensionSetSchemaVersion,
		Extensions: entries,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ExtensionSetManifest{}, nil, fmt.Errorf("encode extension set manifest: %w", err)
	}
	return manifest, append(raw, '\n'), nil
}

func ParseExtensionSetManifest(raw []byte) (ExtensionSetManifest, error) {
	if len(raw) > MaxExtensionSetManifestSize {
		return ExtensionSetManifest{}, fmt.Errorf("%w: manifest exceeds %d bytes", ErrExtensionSetInvalid, MaxExtensionSetManifestSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest ExtensionSetManifest
	if err := decoder.Decode(&manifest); err != nil {
		return ExtensionSetManifest{}, fmt.Errorf("%w: %v", ErrExtensionSetInvalid, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return ExtensionSetManifest{}, fmt.Errorf("%w: multiple JSON values", ErrExtensionSetInvalid)
		}
		return ExtensionSetManifest{}, fmt.Errorf("%w: trailing data: %v", ErrExtensionSetInvalid, err)
	}
	manifest.Format = strings.TrimSpace(manifest.Format)
	if manifest.Format != ExtensionSetFormatV1 || manifest.SchemaVersion != ExtensionSetSchemaVersion {
		return ExtensionSetManifest{}, fmt.Errorf("%w: unsupported format or schema version", ErrExtensionSetInvalid)
	}
	seen := make(map[string]struct{}, len(manifest.Extensions))
	for index := range manifest.Extensions {
		entry := &manifest.Extensions[index]
		entry.ExtensionID = strings.TrimSpace(entry.ExtensionID)
		entry.Version = strings.TrimSpace(entry.Version)
		entry.ManifestSHA256 = strings.ToLower(strings.TrimSpace(entry.ManifestSHA256))
		entry.PayloadSHA256 = strings.ToLower(strings.TrimSpace(entry.PayloadSHA256))
		entry.Platform = strings.ToLower(strings.TrimSpace(entry.Platform))
		entry.Architecture = strings.ToLower(strings.TrimSpace(entry.Architecture))
		if !extensionIDPattern.MatchString(entry.ExtensionID) || !versionPattern.MatchString(entry.Version) {
			return ExtensionSetManifest{}, fmt.Errorf("%w: invalid extension identity", ErrExtensionSetInvalid)
		}
		switch entry.Kind {
		case KindPlugin, KindAddon:
		default:
			return ExtensionSetManifest{}, fmt.Errorf("%w: invalid kind for %s", ErrExtensionSetInvalid, entry.ExtensionID)
		}
		switch entry.Source {
		case SourceOfficial, SourceLocal, SourceCommunity:
		default:
			return ExtensionSetManifest{}, fmt.Errorf("%w: invalid source for %s", ErrExtensionSetInvalid, entry.ExtensionID)
		}
		if !validSHA256Hex(entry.ManifestSHA256) || !validSHA256Hex(entry.PayloadSHA256) || entry.PayloadSizeBytes < 0 {
			return ExtensionSetManifest{}, fmt.Errorf("%w: invalid content identity for %s", ErrExtensionSetInvalid, entry.ExtensionID)
		}
		if !tokenPattern.MatchString(entry.Platform) || !tokenPattern.MatchString(entry.Architecture) {
			return ExtensionSetManifest{}, fmt.Errorf("%w: invalid platform or architecture for %s", ErrExtensionSetInvalid, entry.ExtensionID)
		}
		if _, duplicate := seen[entry.ExtensionID]; duplicate {
			return ExtensionSetManifest{}, fmt.Errorf("%w: duplicate extension_id %s", ErrExtensionSetInvalid, entry.ExtensionID)
		}
		seen[entry.ExtensionID] = struct{}{}
	}
	sort.Slice(manifest.Extensions, func(i, j int) bool { return manifest.Extensions[i].ExtensionID < manifest.Extensions[j].ExtensionID })
	return manifest, nil
}

func validSHA256Hex(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func (s *ExtensionSetService) PlanRestore(ctx context.Context, raw []byte) (ExtensionSetRestorePlan, error) {
	manifest, err := ParseExtensionSetManifest(raw)
	if err != nil {
		return ExtensionSetRestorePlan{}, err
	}
	plan := newExtensionSetRestorePlan()
	if len(manifest.Extensions) == 0 {
		plan.Status = ExtensionSetRestoreNoop
		return plan, nil
	}

	installed, err := s.installer.List(ctx, "")
	if err != nil {
		return ExtensionSetRestorePlan{}, err
	}
	installedByExtension := make(map[string]Installation, len(installed))
	for _, installation := range installed {
		installedByExtension[installation.ExtensionID] = installation
	}

	entries := make(map[string]ExtensionSetEntry, len(manifest.Extensions))
	resolved := make(map[string]Package, len(manifest.Extensions))
	actions := make(map[string]ExtensionSetRestoreAction, len(manifest.Extensions))
	for _, entry := range manifest.Extensions {
		entries[entry.ExtensionID] = entry
		pkg, found, incompatible, err := s.resolveExactPackage(ctx, entry)
		if err != nil {
			return ExtensionSetRestorePlan{}, err
		}
		if !found {
			code := "EXACT_PACKAGE_NOT_AVAILABLE"
			if incompatible {
				code = "EXACT_PACKAGE_INCOMPATIBLE"
			}
			plan.Blockers = append(plan.Blockers, ExtensionSetRestoreBlocker{Code: code, ExtensionID: entry.ExtensionID})
			continue
		}
		resolved[entry.ExtensionID] = pkg
		if existing, ok := installedByExtension[entry.ExtensionID]; ok {
			matches, err := s.installationMatchesEntry(ctx, existing, entry)
			if err != nil {
				return ExtensionSetRestorePlan{}, err
			}
			if !matches {
				plan.Blockers = append(plan.Blockers, ExtensionSetRestoreBlocker{
					Code: "DIFFERENT_ARTIFACT_INSTALLED",
					ExtensionID: entry.ExtensionID,
					Detail: existing.Version,
				})
				continue
			}
			installedPackage, err := s.installer.library.Get(ctx, existing.PackageID)
			if err != nil {
				return ExtensionSetRestorePlan{}, err
			}
			resolved[entry.ExtensionID] = installedPackage
			actions[entry.ExtensionID] = ExtensionSetRestoreAlreadyInstalled
		} else {
			actions[entry.ExtensionID] = ExtensionSetRestoreInstall
		}
	}

	if len(plan.Blockers) == 0 {
		for _, entry := range manifest.Extensions {
			pkg := resolved[entry.ExtensionID]
			for _, dependency := range pkg.Manifest.Dependencies {
				if dependency.Optional {
					continue
				}
				dependencyEntry, ok := entries[dependency.ExtensionID]
				if !ok {
					plan.Blockers = append(plan.Blockers, ExtensionSetRestoreBlocker{
						Code: "DEPENDENCY_MISSING_FROM_SET",
						ExtensionID: dependency.ExtensionID,
						Detail: "required by " + entry.ExtensionID,
					})
					continue
				}
				if !versionWithinDependencyRange(dependencyEntry.Version, dependency) {
					plan.Blockers = append(plan.Blockers, ExtensionSetRestoreBlocker{
						Code: "DEPENDENCY_VERSION_MISMATCH",
						ExtensionID: dependency.ExtensionID,
						Detail: "required by " + entry.ExtensionID,
					})
				}
			}
		}
	}

	order, cycle := extensionSetTopologicalOrder(manifest.Extensions, resolved)
	if cycle && len(plan.Blockers) == 0 {
		plan.Blockers = append(plan.Blockers, ExtensionSetRestoreBlocker{Code: "DEPENDENCY_CYCLE"})
	}
	if len(plan.Blockers) != 0 {
		sort.Slice(plan.Blockers, func(i, j int) bool {
			if plan.Blockers[i].ExtensionID != plan.Blockers[j].ExtensionID {
				return plan.Blockers[i].ExtensionID < plan.Blockers[j].ExtensionID
			}
			return plan.Blockers[i].Code < plan.Blockers[j].Code
		})
		plan.Status = ExtensionSetRestoreBlocked
		return plan, nil
	}

	installCount := 0
	for index, extensionID := range order {
		pkg := resolved[extensionID]
		action := actions[extensionID]
		if action == ExtensionSetRestoreInstall {
			installCount++
		}
		plan.Steps = append(plan.Steps, ExtensionSetRestoreStep{
			Order: index + 1,
			ExtensionID: extensionID,
			Version: entries[extensionID].Version,
			PackageID: pkg.PackageID,
			Action: action,
		})
	}
	if installCount == 0 {
		plan.Status = ExtensionSetRestoreNoop
	} else {
		plan.Status = ExtensionSetRestoreReady
	}
	return plan, nil
}

func newExtensionSetRestorePlan() ExtensionSetRestorePlan {
	return ExtensionSetRestorePlan{
		Status: ExtensionSetRestoreBlocked,
		Steps: []ExtensionSetRestoreStep{},
		Blockers: []ExtensionSetRestoreBlocker{},
		PermissionReviewsRestored: false,
		RuntimeIntentRestored: false,
		NewInstallDesiredState: "DISABLED",
	}
}

func (s *ExtensionSetService) Restore(ctx context.Context, raw []byte, actor string) (ExtensionSetRestoreResult, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return ExtensionSetRestoreResult{}, fmt.Errorf("restore actor is required")
	}
	activeType, err := s.installer.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return ExtensionSetRestoreResult{}, err
	}
	if activeType == domain.SessionShow {
		return ExtensionSetRestoreResult{}, domain.ErrShowConfigurationLocked
	}
	plan, err := s.PlanRestore(ctx, raw)
	if err != nil {
		return ExtensionSetRestoreResult{}, err
	}
	result := ExtensionSetRestoreResult{Plan: plan, Installed: []Installation{}}
	if plan.Status == ExtensionSetRestoreBlocked {
		return result, ErrExtensionSetRestoreBlocked
	}
	for _, step := range plan.Steps {
		if step.Action == ExtensionSetRestoreAlreadyInstalled {
			continue
		}
		installation, err := s.installer.InstallPlanned(ctx, step.PackageID, actor)
		if err != nil {
			return result, fmt.Errorf("restore %s: %w", step.ExtensionID, err)
		}
		result.Installed = append(result.Installed, installation)
	}
	return result, nil
}

func (s *ExtensionSetService) resolveExactPackage(ctx context.Context, entry ExtensionSetEntry) (Package, bool, bool, error) {
	candidates, err := s.installer.library.List(ctx, entry.ExtensionID)
	if err != nil {
		return Package{}, false, false, err
	}
	incompatible := false
	for _, candidate := range candidates {
		if candidate.Manifest.Version != entry.Version || candidate.Manifest.Kind != entry.Kind || candidate.Manifest.Source != entry.Source || candidate.ManifestSHA256 != entry.ManifestSHA256 {
			continue
		}
		status, err := s.installer.library.software.Get(ctx, candidate.PackageID)
		if err != nil {
			return Package{}, false, false, err
		}
		softwarePackage := status.Package
		if softwarePackage.ContentHash != entry.PayloadSHA256 || softwarePackage.SizeBytes != entry.PayloadSizeBytes || softwarePackage.Platform != entry.Platform || softwarePackage.Architecture != entry.Architecture {
			continue
		}
		if !candidate.Compatible {
			incompatible = true
			continue
		}
		return candidate, true, incompatible, nil
	}
	return Package{}, false, incompatible, nil
}

func (s *ExtensionSetService) installationMatchesEntry(ctx context.Context, installation Installation, entry ExtensionSetEntry) (bool, error) {
	if installation.ExtensionID != entry.ExtensionID || installation.Version != entry.Version || installation.ContentSHA256 != entry.PayloadSHA256 || installation.SizeBytes != entry.PayloadSizeBytes {
		return false, nil
	}
	pkg, err := s.installer.library.Get(ctx, installation.PackageID)
	if err != nil {
		return false, err
	}
	if pkg.Manifest.Kind != entry.Kind || pkg.Manifest.Source != entry.Source || pkg.ManifestSHA256 != entry.ManifestSHA256 {
		return false, nil
	}
	status, err := s.installer.library.software.Get(ctx, installation.PackageID)
	if err != nil {
		return false, err
	}
	return status.Package.Platform == entry.Platform && status.Package.Architecture == entry.Architecture, nil
}

func versionWithinDependencyRange(version string, dependency Dependency) bool {
	if dependency.MinVersion != "" && compareSemanticVersions(version, dependency.MinVersion) < 0 {
		return false
	}
	if dependency.MaxVersion != "" && compareSemanticVersions(version, dependency.MaxVersion) > 0 {
		return false
	}
	return true
}

func extensionSetTopologicalOrder(entries []ExtensionSetEntry, resolved map[string]Package) ([]string, bool) {
	indegree := make(map[string]int, len(entries))
	edges := make(map[string][]string, len(entries))
	for _, entry := range entries {
		indegree[entry.ExtensionID] = 0
	}
	for _, entry := range entries {
		pkg, ok := resolved[entry.ExtensionID]
		if !ok {
			continue
		}
		for _, dependency := range pkg.Manifest.Dependencies {
			if dependency.Optional {
				continue
			}
			if _, exists := indegree[dependency.ExtensionID]; !exists {
				continue
			}
			edges[dependency.ExtensionID] = append(edges[dependency.ExtensionID], entry.ExtensionID)
			indegree[entry.ExtensionID]++
		}
	}
	ready := make([]string, 0)
	for extensionID, degree := range indegree {
		if degree == 0 {
			ready = append(ready, extensionID)
		}
	}
	sort.Strings(ready)
	order := make([]string, 0, len(entries))
	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
		order = append(order, current)
		dependents := append([]string(nil), edges[current]...)
		sort.Strings(dependents)
		for _, dependent := range dependents {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				ready = append(ready, dependent)
				sort.Strings(ready)
			}
		}
	}
	return order, len(order) != len(entries)
}
