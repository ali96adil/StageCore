package extension

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
)

const (
	OfflineBundleFormatV1        = "stagecore-extension-bundle-v1"
	OfflineBundleMetadataName    = "bundle.json"
	OfflineBundleManifestName    = "manifest.json"
	OfflineBundlePayloadName     = "payload.bin"
	OfflineBundleFileExtension   = ".scext"
	MaxOfflineBundleMetadataSize = 64 << 10
	MaxOfflineBundleManifestSize = 256 << 10
	MaxOfflineBundlePayloadSize  = 512 << 20
)

var (
	ErrOfflineBundleInvalid      = errors.New("invalid StageCore extension bundle")
	ErrOfflineBundleSource       = errors.New("offline extension bundle source is not allowed on this import path")
	ErrOfflineBundleIntegrity    = errors.New("offline extension bundle payload integrity mismatch")
	ErrOfflineBundleTooLarge     = errors.New("offline extension bundle exceeds size limits")
	ErrTrustedCatalogUnavailable = errors.New("trusted extension catalog is unavailable")
)

type OfflineBundleMetadata struct {
	Format             string `json:"format"`
	ProductID          string `json:"product_id"`
	Version            string `json:"version"`
	Platform           string `json:"platform"`
	Architecture       string `json:"architecture"`
	MinAPIVersion      int    `json:"min_api_version"`
	MaxAPIVersion      int    `json:"max_api_version"`
	OriginalFilename   string `json:"original_filename"`
	SigningStatus      string `json:"signing_status"`
	NotarizationStatus string `json:"notarization_status"`
	ReleaseChannel     string `json:"release_channel"`
	ReleaseNotes       string `json:"release_notes"`
	PayloadSHA256      string `json:"payload_sha256"`
	PayloadSizeBytes   int64  `json:"payload_size_bytes"`
}

type OfflineBundleImportResult struct {
	Package           Package `json:"package"`
	PayloadSHA256     string  `json:"payload_sha256"`
	PayloadSizeBytes  int64   `json:"payload_size_bytes"`
	TrustedOfficial   bool    `json:"trusted_official"`
	AlreadyRegistered bool    `json:"already_registered"`
}

type OfflineBundleImporter struct {
	library *Library
}

func NewOfflineBundleImporter(library *Library) (*OfflineBundleImporter, error) {
	if library == nil || library.store == nil || library.software == nil {
		return nil, fmt.Errorf("offline bundle importer requires extension library")
	}
	return &OfflineBundleImporter{library: library}, nil
}

// Import accepts operator-supplied bundles. It never grants OFFICIAL
// provenance and deliberately downgrades unverified signing/release metadata.
func (i *OfflineBundleImporter) Import(ctx context.Context, source io.Reader, actor string) (OfflineBundleImportResult, error) {
	return i.importBundle(ctx, source, actor, false)
}

// ImportTrustedOfficial is reserved for StageCore-owned catalog roots.
func (i *OfflineBundleImporter) ImportTrustedOfficial(ctx context.Context, source io.Reader, actor string) (OfflineBundleImportResult, error) {
	return i.importBundle(ctx, source, actor, true)
}

func (i *OfflineBundleImporter) importBundle(ctx context.Context, source io.Reader, actor string, trustedOfficial bool) (OfflineBundleImportResult, error) {
	if i == nil || i.library == nil || i.library.store == nil || i.library.software == nil {
		return OfflineBundleImportResult{}, fmt.Errorf("offline bundle importer is unavailable")
	}
	actor = strings.TrimSpace(actor)
	if source == nil || actor == "" {
		return OfflineBundleImportResult{}, fmt.Errorf("bundle source and actor are required")
	}
	activeType, err := i.library.store.ActiveOperationalSessionType(ctx)
	if err != nil {
		return OfflineBundleImportResult{}, err
	}
	if activeType == domain.SessionShow {
		return OfflineBundleImportResult{}, domain.ErrShowConfigurationLocked
	}

	metadata, rawManifest, canonicalManifest, staged, err := stageOfflineBundle(source)
	if err != nil {
		return OfflineBundleImportResult{}, err
	}
	defer func() {
		name := staged.Name()
		_ = staged.Close()
		_ = os.Remove(name)
	}()

	manifest, _, err := ParseManifest(rawManifest)
	if err != nil {
		return OfflineBundleImportResult{}, fmt.Errorf("%w: manifest: %v", ErrOfflineBundleInvalid, err)
	}
	if trustedOfficial {
		if manifest.Source != SourceOfficial {
			return OfflineBundleImportResult{}, fmt.Errorf("%w: trusted catalog bundle must declare OFFICIAL source", ErrOfflineBundleSource)
		}
	} else if manifest.Source != SourceLocal && manifest.Source != SourceCommunity {
		return OfflineBundleImportResult{}, fmt.Errorf("%w: operator import accepts LOCAL or COMMUNITY only", ErrOfflineBundleSource)
	}
	if err := validateBundleBinding(metadata, manifest); err != nil {
		return OfflineBundleImportResult{}, err
	}

	manifestHash := sha256.Sum256(canonicalManifest)
	existing, found, err := i.findExisting(ctx, metadata, manifest, hex.EncodeToString(manifestHash[:]))
	if err != nil {
		return OfflineBundleImportResult{}, err
	}
	if found {
		return OfflineBundleImportResult{
			Package: existing, PayloadSHA256: metadata.PayloadSHA256, PayloadSizeBytes: metadata.PayloadSizeBytes,
			TrustedOfficial: trustedOfficial, AlreadyRegistered: true,
		}, nil
	}

	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return OfflineBundleImportResult{}, fmt.Errorf("rewind staged extension payload: %w", err)
	}
	signingStatus := metadata.SigningStatus
	notarizationStatus := metadata.NotarizationStatus
	releaseChannel := metadata.ReleaseChannel
	if !trustedOfficial {
		signingStatus = store.SoftwareSigningUnknown
		notarizationStatus = store.SoftwareNotarizationUnknown
		releaseChannel = store.SoftwareChannelDevelopment
	}
	expectedSize := metadata.PayloadSizeBytes
	softwarePackage, err := i.library.software.ImportPackage(ctx, software.ImportParams{
		ProductID: metadata.ProductID, Version: metadata.Version, Platform: metadata.Platform, Architecture: metadata.Architecture,
		MinAPIVersion: metadata.MinAPIVersion, MaxAPIVersion: metadata.MaxAPIVersion,
		OriginalFilename: metadata.OriginalFilename, SigningStatus: signingStatus, NotarizationStatus: notarizationStatus,
		ReleaseChannel: releaseChannel, ReleaseNotes: metadata.ReleaseNotes,
		ExpectedContentHash: metadata.PayloadSHA256, ExpectedSizeBytes: &expectedSize,
	}, staged)
	if err != nil {
		return OfflineBundleImportResult{}, err
	}

	var registered Package
	if trustedOfficial {
		registered, err = i.library.RegisterOfficial(ctx, softwarePackage.ID, rawManifest, actor)
	} else {
		registered, err = i.library.Register(ctx, softwarePackage.ID, rawManifest, actor)
	}
	if err != nil {
		return OfflineBundleImportResult{}, err
	}
	return OfflineBundleImportResult{
		Package: registered, PayloadSHA256: metadata.PayloadSHA256, PayloadSizeBytes: metadata.PayloadSizeBytes,
		TrustedOfficial: trustedOfficial,
	}, nil
}

func stageOfflineBundle(source io.Reader) (OfflineBundleMetadata, []byte, []byte, *os.File, error) {
	tr := tar.NewReader(source)
	metadataRaw, err := readStrictTarEntry(tr, OfflineBundleMetadataName, MaxOfflineBundleMetadataSize)
	if err != nil {
		return OfflineBundleMetadata{}, nil, nil, nil, err
	}
	var metadata OfflineBundleMetadata
	if err := decodeStrictBundleJSON(metadataRaw, &metadata); err != nil {
		return OfflineBundleMetadata{}, nil, nil, nil, fmt.Errorf("%w: bundle metadata: %v", ErrOfflineBundleInvalid, err)
	}
	metadata = normalizeBundleMetadata(metadata)
	if err := validateBundleMetadata(metadata); err != nil {
		return OfflineBundleMetadata{}, nil, nil, nil, err
	}

	manifestRaw, err := readStrictTarEntry(tr, OfflineBundleManifestName, MaxOfflineBundleManifestSize)
	if err != nil {
		return OfflineBundleMetadata{}, nil, nil, nil, err
	}
	_, canonicalManifest, err := ParseManifest(manifestRaw)
	if err != nil {
		return OfflineBundleMetadata{}, nil, nil, nil, fmt.Errorf("%w: manifest: %v", ErrOfflineBundleInvalid, err)
	}

	header, err := tr.Next()
	if err != nil {
		return OfflineBundleMetadata{}, nil, nil, nil, fmt.Errorf("%w: missing %s: %v", ErrOfflineBundleInvalid, OfflineBundlePayloadName, err)
	}
	if !strictRegularHeader(header, OfflineBundlePayloadName) {
		return OfflineBundleMetadata{}, nil, nil, nil, fmt.Errorf("%w: expected regular %s entry", ErrOfflineBundleInvalid, OfflineBundlePayloadName)
	}
	if header.Size < 0 || header.Size > MaxOfflineBundlePayloadSize {
		return OfflineBundleMetadata{}, nil, nil, nil, ErrOfflineBundleTooLarge
	}
	if header.Size != metadata.PayloadSizeBytes {
		return OfflineBundleMetadata{}, nil, nil, nil, fmt.Errorf("%w: payload header size does not match bundle metadata", ErrOfflineBundleIntegrity)
	}

	staged, err := os.CreateTemp("", "stagecore-extension-import-*.payload")
	if err != nil {
		return OfflineBundleMetadata{}, nil, nil, nil, fmt.Errorf("create extension import staging file: %w", err)
	}
	cleanup := func(cause error) (OfflineBundleMetadata, []byte, []byte, *os.File, error) {
		name := staged.Name()
		_ = staged.Close()
		_ = os.Remove(name)
		return OfflineBundleMetadata{}, nil, nil, nil, cause
	}
	if err := staged.Chmod(0o600); err != nil {
		return cleanup(err)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(staged, hash), tr)
	if err != nil {
		return cleanup(fmt.Errorf("stage extension payload: %w", err))
	}
	if written != metadata.PayloadSizeBytes || hex.EncodeToString(hash.Sum(nil)) != metadata.PayloadSHA256 {
		return cleanup(ErrOfflineBundleIntegrity)
	}
	if next, err := tr.Next(); !errors.Is(err, io.EOF) || next != nil {
		if err == nil {
			err = fmt.Errorf("unexpected extra tar entry %q", next.Name)
		}
		return cleanup(fmt.Errorf("%w: %v", ErrOfflineBundleInvalid, err))
	}
	return metadata, manifestRaw, canonicalManifest, staged, nil
}

func readStrictTarEntry(tr *tar.Reader, expectedName string, maxSize int64) ([]byte, error) {
	header, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("%w: missing %s: %v", ErrOfflineBundleInvalid, expectedName, err)
	}
	if !strictRegularHeader(header, expectedName) {
		return nil, fmt.Errorf("%w: expected regular %s entry", ErrOfflineBundleInvalid, expectedName)
	}
	if header.Size < 0 || header.Size > maxSize {
		return nil, ErrOfflineBundleTooLarge
	}
	content, err := io.ReadAll(io.LimitReader(tr, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", expectedName, err)
	}
	if int64(len(content)) != header.Size || int64(len(content)) > maxSize {
		return nil, fmt.Errorf("%w: invalid %s size", ErrOfflineBundleInvalid, expectedName)
	}
	return content, nil
}

func strictRegularHeader(header *tar.Header, expectedName string) bool {
	return header != nil && header.Name == expectedName && (header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA)
}

func decodeStrictBundleJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func normalizeBundleMetadata(metadata OfflineBundleMetadata) OfflineBundleMetadata {
	metadata.Format = strings.TrimSpace(metadata.Format)
	metadata.ProductID = strings.TrimSpace(metadata.ProductID)
	metadata.Version = strings.TrimSpace(metadata.Version)
	metadata.Platform = strings.ToLower(strings.TrimSpace(metadata.Platform))
	metadata.Architecture = strings.ToLower(strings.TrimSpace(metadata.Architecture))
	metadata.OriginalFilename = strings.TrimSpace(metadata.OriginalFilename)
	metadata.SigningStatus = strings.ToUpper(strings.TrimSpace(metadata.SigningStatus))
	metadata.NotarizationStatus = strings.ToUpper(strings.TrimSpace(metadata.NotarizationStatus))
	metadata.ReleaseChannel = strings.ToLower(strings.TrimSpace(metadata.ReleaseChannel))
	metadata.PayloadSHA256 = strings.ToLower(strings.TrimSpace(metadata.PayloadSHA256))
	return metadata
}

func validateBundleMetadata(metadata OfflineBundleMetadata) error {
	if metadata.Format != OfflineBundleFormatV1 || metadata.ProductID == "" || metadata.Version == "" || metadata.Platform == "" || metadata.Architecture == "" || metadata.OriginalFilename == "" {
		return fmt.Errorf("%w: required bundle identity metadata is missing", ErrOfflineBundleInvalid)
	}
	if filepath.Base(metadata.OriginalFilename) != metadata.OriginalFilename {
		return fmt.Errorf("%w: original_filename must be a base filename", ErrOfflineBundleInvalid)
	}
	if metadata.MinAPIVersion < 0 || metadata.MaxAPIVersion < metadata.MinAPIVersion {
		return fmt.Errorf("%w: invalid API compatibility range", ErrOfflineBundleInvalid)
	}
	if metadata.PayloadSizeBytes < 0 || metadata.PayloadSizeBytes > MaxOfflineBundlePayloadSize {
		return ErrOfflineBundleTooLarge
	}
	decoded, err := hex.DecodeString(metadata.PayloadSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("%w: payload_sha256 must be a 64-character SHA-256 hex digest", ErrOfflineBundleInvalid)
	}
	if !oneOf(metadata.SigningStatus, store.SoftwareSigningUnknown, store.SoftwareSigningUnsigned, store.SoftwareSigningSigned) {
		return fmt.Errorf("%w: invalid signing_status", ErrOfflineBundleInvalid)
	}
	if !oneOf(metadata.NotarizationStatus, store.SoftwareNotarizationUnknown, store.SoftwareNotarizationNotApplicable, store.SoftwareNotarizationNotNotarized, store.SoftwareNotarizationNotarized) {
		return fmt.Errorf("%w: invalid notarization_status", ErrOfflineBundleInvalid)
	}
	if !oneOf(metadata.ReleaseChannel, store.SoftwareChannelDevelopment, store.SoftwareChannelRelease) {
		return fmt.Errorf("%w: invalid release_channel", ErrOfflineBundleInvalid)
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateBundleBinding(metadata OfflineBundleMetadata, manifest Manifest) error {
	if metadata.ProductID != manifest.ExtensionID || metadata.Version != manifest.Version {
		return fmt.Errorf("%w: bundle identity does not match extension manifest", ErrOfflineBundleInvalid)
	}
	if metadata.MinAPIVersion != manifest.Compatibility.APIMin || metadata.MaxAPIVersion != manifest.Compatibility.APIMax {
		return fmt.Errorf("%w: bundle API range does not match extension manifest", ErrOfflineBundleInvalid)
	}
	if !contains(manifest.Compatibility.Platforms, metadata.Platform) || !contains(manifest.Compatibility.Architectures, metadata.Architecture) {
		return fmt.Errorf("%w: bundle platform or architecture is not declared by extension manifest", ErrOfflineBundleInvalid)
	}
	return nil
}

func (i *OfflineBundleImporter) findExisting(ctx context.Context, metadata OfflineBundleMetadata, manifest Manifest, manifestHash string) (Package, bool, error) {
	packages, err := i.library.List(ctx, manifest.ExtensionID)
	if err != nil {
		return Package{}, false, err
	}
	for _, candidate := range packages {
		if candidate.Manifest.Version != manifest.Version || candidate.Manifest.Source != manifest.Source || candidate.ManifestSHA256 != manifestHash {
			continue
		}
		status, err := i.library.software.Get(ctx, candidate.PackageID)
		if err != nil {
			return Package{}, false, err
		}
		pkg := status.Package
		if pkg.ContentHash == metadata.PayloadSHA256 && pkg.SizeBytes == metadata.PayloadSizeBytes && pkg.Platform == metadata.Platform && pkg.Architecture == metadata.Architecture {
			return candidate, true, nil
		}
	}
	return Package{}, false, nil
}

type TrustedCatalogSyncResult struct {
	Root     string                      `json:"-"`
	Imported []TrustedCatalogImportEntry `json:"imported"`
}

type TrustedCatalogImportEntry struct {
	Filename          string `json:"filename"`
	PackageID         string `json:"package_id"`
	ExtensionID       string `json:"extension_id"`
	Version           string `json:"version"`
	AlreadyRegistered bool   `json:"already_registered"`
}

type TrustedCatalog struct {
	importer *OfflineBundleImporter
	root     string
}

func NewTrustedCatalog(importer *OfflineBundleImporter, root string) (*TrustedCatalog, error) {
	if importer == nil {
		return nil, fmt.Errorf("trusted catalog requires offline bundle importer")
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("trusted extension catalog root must be an absolute path")
	}
	return &TrustedCatalog{importer: importer, root: root}, nil
}

func (c *TrustedCatalog) Sync(ctx context.Context, actor string) (TrustedCatalogSyncResult, error) {
	if c == nil || c.importer == nil {
		return TrustedCatalogSyncResult{}, ErrTrustedCatalogUnavailable
	}
	entries, err := os.ReadDir(c.root)
	if errors.Is(err, os.ErrNotExist) {
		return TrustedCatalogSyncResult{Root: c.root, Imported: []TrustedCatalogImportEntry{}}, nil
	}
	if err != nil {
		return TrustedCatalogSyncResult{}, fmt.Errorf("%w: %v", ErrTrustedCatalogUnavailable, err)
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name() < entries[b].Name() })
	result := TrustedCatalogSyncResult{Root: c.root, Imported: []TrustedCatalogImportEntry{}}
	for _, entry := range entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name()), OfflineBundleFileExtension) {
			continue
		}
		fullPath := filepath.Join(c.root, entry.Name())
		info, err := os.Lstat(fullPath)
		if err != nil {
			return TrustedCatalogSyncResult{}, fmt.Errorf("trusted catalog entry %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return TrustedCatalogSyncResult{}, fmt.Errorf("trusted catalog entry %s is not a regular file", entry.Name())
		}
		file, err := os.Open(fullPath)
		if err != nil {
			return TrustedCatalogSyncResult{}, fmt.Errorf("open trusted catalog entry %s: %w", entry.Name(), err)
		}
		imported, importErr := c.importer.ImportTrustedOfficial(ctx, file, actor)
		closeErr := file.Close()
		if importErr != nil {
			return TrustedCatalogSyncResult{}, fmt.Errorf("import trusted catalog entry %s: %w", entry.Name(), importErr)
		}
		if closeErr != nil {
			return TrustedCatalogSyncResult{}, fmt.Errorf("close trusted catalog entry %s: %w", entry.Name(), closeErr)
		}
		result.Imported = append(result.Imported, TrustedCatalogImportEntry{
			Filename: entry.Name(), PackageID: imported.Package.PackageID, ExtensionID: imported.Package.Manifest.ExtensionID,
			Version: imported.Package.Manifest.Version, AlreadyRegistered: imported.AlreadyRegistered,
		})
	}
	return result, nil
}
