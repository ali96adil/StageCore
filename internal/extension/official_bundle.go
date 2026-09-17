package extension

import (
	"bytes"
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

var ErrOfficialBundleConflict = errors.New("bundled OFFICIAL extension conflicts with existing immutable package metadata")

// BundledOfficialPackage describes one StageCore-shipped extension payload.
// Manifest and Payload are immutable build inputs; bootstrap imports Payload
// through the normal Software Repository/Vault and registers Manifest through
// the trusted OFFICIAL Extension Library path.
type BundledOfficialPackage struct {
	Manifest         []byte
	Payload          []byte
	Platform         string
	Architecture     string
	OriginalFilename string
	ReleaseNotes     string
}

// BootstrapOfficial makes a bundled OFFICIAL extension discoverable without
// introducing a second package system. It is idempotent across Hub restarts:
// an exact Software Repository + Extension Library record is reused, while a
// same-version package whose immutable metadata differs is rejected.
//
// No write occurs while SHOW configuration is locked. An already-complete
// bootstrap remains readable during SHOW because that path is mutation-free.
func (l *Library) BootstrapOfficial(ctx context.Context, bundle BundledOfficialPackage, actor string) (Package, error) {
	if l == nil || l.store == nil || l.software == nil {
		return Package{}, fmt.Errorf("extension library is unavailable")
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return Package{}, fmt.Errorf("official extension bootstrap actor is required")
	}
	if len(bundle.Payload) == 0 {
		return Package{}, fmt.Errorf("official extension bundle payload is required")
	}

	manifest, canonicalManifest, err := ParseManifest(bundle.Manifest)
	if err != nil {
		return Package{}, err
	}
	if manifest.Source != SourceOfficial {
		return Package{}, fmt.Errorf("official extension bootstrap requires source OFFICIAL")
	}

	platform := strings.ToLower(strings.TrimSpace(bundle.Platform))
	architecture := strings.ToLower(strings.TrimSpace(bundle.Architecture))
	if platform == "" || architecture == "" {
		return Package{}, fmt.Errorf("official extension bundle platform and architecture are required")
	}
	if !contains(manifest.Compatibility.Platforms, platform) {
		return Package{}, fmt.Errorf("official extension manifest does not support package platform %q", platform)
	}
	if !contains(manifest.Compatibility.Architectures, architecture) {
		return Package{}, fmt.Errorf("official extension manifest does not support package architecture %q", architecture)
	}

	payloadHashBytes := sha256.Sum256(bundle.Payload)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])
	payloadSize := int64(len(bundle.Payload))
	manifestHashBytes := sha256.Sum256(canonicalManifest)
	manifestHash := hex.EncodeToString(manifestHashBytes[:])

	statuses, err := l.software.List(ctx, manifest.ExtensionID, platform, architecture)
	if err != nil {
		return Package{}, err
	}
	exact := make([]software.PackageStatus, 0, 1)
	for _, status := range statuses {
		pkg := status.Package
		if pkg.Version != manifest.Version {
			continue
		}
		if !officialSoftwareMetadataMatches(pkg, manifest, payloadHash, payloadSize) {
			return Package{}, fmt.Errorf("%w: %s %s for %s/%s", ErrOfficialBundleConflict, manifest.ExtensionID, manifest.Version, platform, architecture)
		}
		exact = append(exact, status)
	}

	// Prefer a package that is already registered in the Extension Library.
	// This makes repeated Hub starts a pure read even if an earlier release
	// accidentally left duplicate exact Software Repository rows.
	for _, status := range exact {
		registered, err := l.Get(ctx, status.Package.ID)
		if err == nil {
			if registered.Manifest.Source != SourceOfficial || registered.ManifestSHA256 != manifestHash {
				return Package{}, fmt.Errorf("%w: stored extension manifest differs for package %s", ErrOfficialBundleConflict, status.Package.ID)
			}
			return registered, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return Package{}, err
		}
	}

	if err := l.requireBootstrapMutable(ctx); err != nil {
		return Package{}, err
	}

	if len(exact) != 0 {
		return l.RegisterOfficial(ctx, exact[0].Package.ID, canonicalManifest, actor)
	}

	filename := strings.TrimSpace(bundle.OriginalFilename)
	if filename == "" {
		filename = manifest.ExtensionID + "-" + manifest.Version + ".addon"
	}
	softwarePackage, err := l.software.ImportPackage(ctx, software.ImportParams{
		ProductID:            manifest.ExtensionID,
		Version:              manifest.Version,
		Platform:             platform,
		Architecture:         architecture,
		MinAPIVersion:        manifest.Compatibility.APIMin,
		MaxAPIVersion:        manifest.Compatibility.APIMax,
		OriginalFilename:     filename,
		SigningStatus:        store.SoftwareSigningSigned,
		NotarizationStatus:   store.SoftwareNotarizationNotApplicable,
		ReleaseChannel:       store.SoftwareChannelRelease,
		ReleaseNotes:         strings.TrimSpace(bundle.ReleaseNotes),
		ExpectedContentHash:  payloadHash,
		ExpectedSizeBytes:    &payloadSize,
	}, bytes.NewReader(bundle.Payload))
	if err != nil {
		return Package{}, fmt.Errorf("import bundled official extension payload: %w", err)
	}
	return l.RegisterOfficial(ctx, softwarePackage.ID, canonicalManifest, actor)
}

func (l *Library) requireBootstrapMutable(ctx context.Context) error {
	activeType, err := l.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return err
	}
	if activeType == domain.SessionShow {
		return domain.ErrShowConfigurationLocked
	}
	return nil
}

func officialSoftwareMetadataMatches(pkg store.SoftwarePackage, manifest Manifest, payloadHash string, payloadSize int64) bool {
	return pkg.ProductID == manifest.ExtensionID &&
		pkg.Version == manifest.Version &&
		pkg.MinAPIVersion == manifest.Compatibility.APIMin &&
		pkg.MaxAPIVersion == manifest.Compatibility.APIMax &&
		pkg.ContentHash == payloadHash &&
		pkg.SizeBytes == payloadSize &&
		pkg.SigningStatus == store.SoftwareSigningSigned &&
		pkg.NotarizationStatus == store.SoftwareNotarizationNotApplicable &&
		pkg.ReleaseChannel == store.SoftwareChannelRelease
}
