package extension

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
)

// BundledOfficialSpec describes immutable extension bytes shipped inside the
// StageCore product. It deliberately reuses the normal Vault, Software
// Repository and Extension Library instead of creating a second package path.
type BundledOfficialSpec struct {
	Manifest         []byte
	Payload          []byte
	Platform         string
	Architecture     string
	OriginalFilename string
	ReleaseNotes     string
}

type BundledOfficialResult struct {
	Package           Package
	Supported         bool
	AlreadyRegistered bool
}

// BootstrapBundledOfficial makes a trusted StageCore-bundled OFFICIAL package
// discoverable through the normal Extension Library. It does not install,
// enable or activate the extension.
func (l *Library) BootstrapBundledOfficial(ctx context.Context, spec BundledOfficialSpec, actor string) (BundledOfficialResult, error) {
	if l == nil || l.store == nil || l.software == nil {
		return BundledOfficialResult{}, fmt.Errorf("extension library is unavailable")
	}
	actor = strings.TrimSpace(actor)
	platform := strings.ToLower(strings.TrimSpace(spec.Platform))
	architecture := strings.ToLower(strings.TrimSpace(spec.Architecture))
	if actor == "" || platform == "" || architecture == "" {
		return BundledOfficialResult{}, fmt.Errorf("bundled extension actor, platform and architecture are required")
	}
	if len(spec.Manifest) == 0 || len(spec.Payload) == 0 {
		return BundledOfficialResult{}, fmt.Errorf("bundled extension manifest and payload are required")
	}

	manifest, canonicalManifest, err := ParseManifest(spec.Manifest)
	if err != nil {
		return BundledOfficialResult{}, err
	}
	if manifest.Source != SourceOfficial {
		return BundledOfficialResult{}, fmt.Errorf("bundled extension %q must declare OFFICIAL source", manifest.ExtensionID)
	}
	if !contains(manifest.Compatibility.Platforms, platform) || !contains(manifest.Compatibility.Architectures, architecture) {
		return BundledOfficialResult{Supported: false}, nil
	}

	manifestDigest := sha256.Sum256(canonicalManifest)
	manifestHash := hex.EncodeToString(manifestDigest[:])
	payloadDigest := sha256.Sum256(spec.Payload)
	payloadHash := hex.EncodeToString(payloadDigest[:])
	payloadSize := int64(len(spec.Payload))

	existing, err := l.List(ctx, manifest.ExtensionID)
	if err != nil {
		return BundledOfficialResult{}, err
	}
	for _, candidate := range existing {
		if candidate.Manifest.Version != manifest.Version || candidate.Manifest.Source != SourceOfficial || candidate.ManifestSHA256 != manifestHash {
			continue
		}
		status, err := l.software.Get(ctx, candidate.PackageID)
		if err != nil {
			return BundledOfficialResult{}, err
		}
		pkg := status.Package
		if pkg.ContentHash == payloadHash && pkg.SizeBytes == payloadSize && pkg.Platform == platform && pkg.Architecture == architecture {
			return BundledOfficialResult{Package: candidate, Supported: true, AlreadyRegistered: true}, nil
		}
	}

	activeType, err := l.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return BundledOfficialResult{}, err
	}
	if activeType == domain.SessionShow {
		return BundledOfficialResult{}, domain.ErrShowConfigurationLocked
	}

	filename := strings.TrimSpace(spec.OriginalFilename)
	if filename == "" {
		filename = manifest.ExtensionID + ".addon"
	}
	softwarePackage, err := l.software.ImportPackage(ctx, software.ImportParams{
		ProductID:           manifest.ExtensionID,
		Version:             manifest.Version,
		Platform:            platform,
		Architecture:        architecture,
		MinAPIVersion:       manifest.Compatibility.APIMin,
		MaxAPIVersion:       manifest.Compatibility.APIMax,
		OriginalFilename:    filename,
		SigningStatus:       store.SoftwareSigningSigned,
		NotarizationStatus:  store.SoftwareNotarizationNotApplicable,
		ReleaseChannel:      store.SoftwareChannelRelease,
		ReleaseNotes:        strings.TrimSpace(spec.ReleaseNotes),
		ExpectedContentHash: payloadHash,
		ExpectedSizeBytes:   &payloadSize,
	}, bytes.NewReader(spec.Payload))
	if err != nil {
		return BundledOfficialResult{}, err
	}

	registered, err := l.RegisterOfficial(ctx, softwarePackage.ID, spec.Manifest, actor)
	if err != nil {
		return BundledOfficialResult{}, err
	}
	return BundledOfficialResult{Package: registered, Supported: true}, nil
}
