package extension

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
)

var ErrOfficialSourceRequiresTrustedPath = errors.New("OFFICIAL extension source requires a trusted bundled registration path")

type Library struct {
	store    *store.Store
	software *software.Repository
}

func NewLibrary(stageStore *store.Store, softwareRepository *software.Repository) (*Library, error) {
	if stageStore == nil || softwareRepository == nil {
		return nil, fmt.Errorf("extension library requires store and software repository")
	}
	return &Library{store: stageStore, software: softwareRepository}, nil
}

func (l *Library) Register(ctx context.Context, packageID string, rawManifest []byte, actor string) (Package, error) {
	return l.register(ctx, packageID, rawManifest, actor, false)
}

// RegisterOfficial is reserved for a trusted StageCore-bundled catalog path.
// Operator/API imports must use Register so an arbitrary local manifest cannot
// self-assert OFFICIAL provenance.
func (l *Library) RegisterOfficial(ctx context.Context, packageID string, rawManifest []byte, actor string) (Package, error) {
	return l.register(ctx, packageID, rawManifest, actor, true)
}

func (l *Library) register(ctx context.Context, packageID string, rawManifest []byte, actor string, trustedOfficial bool) (Package, error) {
	if l == nil || l.store == nil || l.software == nil {
		return Package{}, fmt.Errorf("extension library is unavailable")
	}
	packageID = strings.TrimSpace(packageID)
	actor = strings.TrimSpace(actor)
	if packageID == "" || actor == "" {
		return Package{}, fmt.Errorf("package ID and actor are required")
	}
	activeType, err := l.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return Package{}, err
	}
	if activeType == domain.SessionShow {
		return Package{}, domain.ErrShowConfigurationLocked
	}

	manifest, canonical, err := ParseManifest(rawManifest)
	if err != nil {
		return Package{}, err
	}
	if manifest.Source == SourceOfficial && !trustedOfficial {
		return Package{}, ErrOfficialSourceRequiresTrustedPath
	}
	status, err := l.software.Get(ctx, packageID)
	if err != nil {
		return Package{}, err
	}
	if !status.Compatible {
		return Package{}, fmt.Errorf("software package is incompatible: %s", status.Reason)
	}
	pkg := status.Package
	if pkg.ProductID != manifest.ExtensionID {
		return Package{}, fmt.Errorf("extension_id %q does not match software product_id %q", manifest.ExtensionID, pkg.ProductID)
	}
	if pkg.Version != manifest.Version {
		return Package{}, fmt.Errorf("extension version %q does not match software package version %q", manifest.Version, pkg.Version)
	}
	if pkg.MinAPIVersion != manifest.Compatibility.APIMin || pkg.MaxAPIVersion != manifest.Compatibility.APIMax {
		return Package{}, fmt.Errorf("extension API compatibility must exactly match immutable software package metadata")
	}
	if !contains(manifest.Compatibility.Platforms, pkg.Platform) {
		return Package{}, fmt.Errorf("extension manifest does not include package platform %q", pkg.Platform)
	}
	if !contains(manifest.Compatibility.Architectures, pkg.Architecture) {
		return Package{}, fmt.Errorf("extension manifest does not include package architecture %q", pkg.Architecture)
	}
	if manifest.Source == SourceOfficial && !status.ProductionReady {
		return Package{}, fmt.Errorf("OFFICIAL extension package must be production-ready: %s", status.Reason)
	}

	hash := sha256.Sum256(canonical)
	record, err := l.store.RegisterExtensionPackage(ctx, store.RegisterExtensionPackageParams{
		PackageID: packageID, ExtensionID: manifest.ExtensionID, Version: manifest.Version,
		Kind: string(manifest.Kind), Source: string(manifest.Source), ManifestJSON: canonical,
		ManifestSHA256: hex.EncodeToString(hash[:]), RegisteredBy: actor,
	})
	if err != nil {
		return Package{}, err
	}
	return makePackage(record, manifest, status), nil
}

func (l *Library) Get(ctx context.Context, packageID string) (Package, error) {
	if l == nil || l.store == nil || l.software == nil {
		return Package{}, fmt.Errorf("extension library is unavailable")
	}
	record, err := l.store.GetExtensionPackage(ctx, strings.TrimSpace(packageID))
	if err != nil {
		return Package{}, err
	}
	return l.inflate(ctx, record)
}

func (l *Library) List(ctx context.Context, extensionID string) ([]Package, error) {
	if l == nil || l.store == nil || l.software == nil {
		return nil, fmt.Errorf("extension library is unavailable")
	}
	records, err := l.store.ListExtensionPackages(ctx, strings.TrimSpace(extensionID))
	if err != nil {
		return nil, err
	}
	items := make([]Package, 0, len(records))
	for _, record := range records {
		item, err := l.inflate(ctx, record)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (l *Library) inflate(ctx context.Context, record store.ExtensionPackage) (Package, error) {
	manifest, canonical, err := ParseManifest(record.ManifestJSON)
	if err != nil {
		return Package{}, fmt.Errorf("stored extension manifest %s is invalid: %w", record.PackageID, err)
	}
	hash := sha256.Sum256(canonical)
	if hex.EncodeToString(hash[:]) != record.ManifestSHA256 {
		return Package{}, fmt.Errorf("stored extension manifest hash mismatch for package %s", record.PackageID)
	}
	status, err := l.software.Get(ctx, record.PackageID)
	if err != nil {
		return Package{}, err
	}
	return makePackage(record, manifest, status), nil
}

func makePackage(record store.ExtensionPackage, manifest Manifest, status software.PackageStatus) Package {
	return Package{
		PackageID: record.PackageID,
		Manifest: manifest,
		ManifestSHA256: record.ManifestSHA256,
		Compatible: status.Compatible,
		ProductionReady: status.ProductionReady,
		CompatibilityReason: status.Reason,
		RegisteredBy: record.RegisteredBy,
		RegisteredAt: record.RegisteredAt,
	}
}
