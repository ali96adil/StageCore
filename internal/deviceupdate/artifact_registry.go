package deviceupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrArtifactNotFound = errors.New("firmware artifact not found")
	ErrArtifactInvalid  = errors.New("firmware artifact invalid")
	ErrArtifactConflict = errors.New("firmware artifact already exists")
	ErrArtifactCorrupt  = errors.New("firmware artifact integrity check failed")
)

const (
	artifactFilename = "firmware.bin"
	metadataFilename = "metadata.json"
)

type ArtifactMetadata struct {
	ArtifactID       string    `json:"artifact_id"`
	DeviceID         string    `json:"device_id"`
	ProfileID        string    `json:"profile_id"`
	Version          string    `json:"version"`
	SourceRevision   string    `json:"source_revision"`
	Qualification    string    `json:"qualification"`
	SizeBytes        int64     `json:"size_bytes"`
	SHA256           string    `json:"sha256"`
	OriginalFilename string    `json:"original_filename,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type ArtifactRegistry struct {
	root string
}

func NewArtifactRegistry(root string) (*ArtifactRegistry, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("%w: artifact registry root is required", ErrArtifactInvalid)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve artifact registry root: %v", ErrArtifactInvalid, err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create artifact registry root: %w", err)
	}
	return &ArtifactRegistry{root: absolute}, nil
}

func (r *ArtifactRegistry) Root() string {
	if r == nil {
		return ""
	}
	return r.root
}

func ArtifactPath(artifactID string) string {
	return ArtifactPathPrefix + strings.TrimSpace(artifactID) + "/" + artifactFilename
}

func (r *ArtifactRegistry) ImportQualified(metadata ArtifactMetadata, source io.Reader) (ArtifactMetadata, error) {
	if r == nil || r.root == "" || source == nil {
		return ArtifactMetadata{}, fmt.Errorf("%w: artifact registry and source are required", ErrArtifactInvalid)
	}
	if err := validateArtifactMetadata(metadata); err != nil {
		return ArtifactMetadata{}, err
	}

	finalDir := filepath.Join(r.root, metadata.ArtifactID)
	if _, err := os.Lstat(finalDir); err == nil {
		return ArtifactMetadata{}, ErrArtifactConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return ArtifactMetadata{}, err
	}

	tempDir, err := os.MkdirTemp(r.root, ".incoming-*")
	if err != nil {
		return ArtifactMetadata{}, fmt.Errorf("create artifact staging directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := filepath.Join(tempDir, artifactFilename)
	file, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return ArtifactMetadata{}, fmt.Errorf("create staged firmware artifact: %w", err)
	}

	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(source, MaxArtifactBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return ArtifactMetadata{}, fmt.Errorf("write staged firmware artifact: %w", copyErr)
	}
	if closeErr != nil {
		return ArtifactMetadata{}, fmt.Errorf("close staged firmware artifact: %w", closeErr)
	}
	if written <= 0 || written > MaxArtifactBytes {
		return ArtifactMetadata{}, fmt.Errorf("%w: artifact payload size is outside the allowed range", ErrArtifactInvalid)
	}
	if written != metadata.SizeBytes {
		return ArtifactMetadata{}, fmt.Errorf("%w: artifact payload size does not match metadata", ErrArtifactInvalid)
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != metadata.SHA256 {
		return ArtifactMetadata{}, fmt.Errorf("%w: artifact payload SHA-256 does not match metadata", ErrArtifactInvalid)
	}

	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return ArtifactMetadata{}, fmt.Errorf("encode artifact metadata: %w", err)
	}
	metadataBytes = append(metadataBytes, '
')
	if err := os.WriteFile(filepath.Join(tempDir, metadataFilename), metadataBytes, 0o600); err != nil {
		return ArtifactMetadata{}, fmt.Errorf("write artifact metadata: %w", err)
	}

	if err := os.Rename(tempDir, finalDir); err != nil {
		if _, statErr := os.Lstat(finalDir); statErr == nil {
			return ArtifactMetadata{}, ErrArtifactConflict
		}
		return ArtifactMetadata{}, fmt.Errorf("publish firmware artifact: %w", err)
	}
	return metadata, nil
}

func (r *ArtifactRegistry) OpenForDevice(artifactID, deviceID string) (*os.File, ArtifactMetadata, error) {
	if r == nil || r.root == "" {
		return nil, ArtifactMetadata{}, ErrArtifactNotFound
	}
	artifactID = strings.TrimSpace(artifactID)
	deviceID = strings.TrimSpace(deviceID)
	if _, err := uuid.Parse(artifactID); err != nil || deviceID == "" {
		return nil, ArtifactMetadata{}, ErrArtifactNotFound
	}

	dir := filepath.Join(r.root, artifactID)
	metadata, err := readArtifactMetadata(filepath.Join(dir, metadataFilename))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ArtifactMetadata{}, ErrArtifactNotFound
		}
		return nil, ArtifactMetadata{}, fmt.Errorf("%w: %v", ErrArtifactCorrupt, err)
	}
	if err := validateArtifactMetadata(metadata); err != nil ||
		metadata.ArtifactID != artifactID ||
		metadata.DeviceID != deviceID {
		return nil, ArtifactMetadata{}, ErrArtifactNotFound
	}

	file, err := os.Open(filepath.Join(dir, artifactFilename))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ArtifactMetadata{}, ErrArtifactNotFound
	}
	if err != nil {
		return nil, ArtifactMetadata{}, err
	}

	if err := verifyArtifactFile(file, metadata); err != nil {
		_ = file.Close()
		return nil, ArtifactMetadata{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, ArtifactMetadata{}, err
	}
	return file, metadata, nil
}

func validateArtifactMetadata(metadata ArtifactMetadata) error {
	if _, err := uuid.Parse(strings.TrimSpace(metadata.ArtifactID)); err != nil {
		return fmt.Errorf("%w: artifact_id must be a UUID", ErrArtifactInvalid)
	}
	if _, err := uuid.Parse(strings.TrimSpace(metadata.DeviceID)); err != nil {
		return fmt.Errorf("%w: device_id must be a UUID", ErrArtifactInvalid)
	}
	if strings.TrimSpace(metadata.ProfileID) == "" || metadata.ProfileID != strings.TrimSpace(metadata.ProfileID) {
		return fmt.Errorf("%w: profile_id must be non-empty and trimmed", ErrArtifactInvalid)
	}
	if err := validateVersion("version", metadata.Version); err != nil {
		return fmt.Errorf("%w: %v", ErrArtifactInvalid, err)
	}
	if !canonicalLowerHex(metadata.SourceRevision, 40) {
		return fmt.Errorf("%w: source_revision must be canonical lowercase hex", ErrArtifactInvalid)
	}
	if metadata.Qualification != QualificationQualified {
		return fmt.Errorf("%w: artifact qualification must be QUALIFIED", ErrArtifactInvalid)
	}
	if metadata.SizeBytes <= 0 || metadata.SizeBytes > MaxArtifactBytes {
		return fmt.Errorf("%w: size_bytes must be within 1..%d", ErrArtifactInvalid, MaxArtifactBytes)
	}
	if !canonicalLowerHex(metadata.SHA256, 64) {
		return fmt.Errorf("%w: sha256 must be canonical lowercase SHA-256", ErrArtifactInvalid)
	}
	if metadata.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at is required", ErrArtifactInvalid)
	}
	if metadata.OriginalFilename != "" {
		if metadata.OriginalFilename != filepath.Base(metadata.OriginalFilename) ||
			strings.TrimSpace(metadata.OriginalFilename) != metadata.OriginalFilename {
			return fmt.Errorf("%w: original_filename must be a canonical basename", ErrArtifactInvalid)
		}
	}
	return nil
}

func readArtifactMetadata(filename string) (ArtifactMetadata, error) {
	file, err := os.Open(filename)
	if err != nil {
		return ArtifactMetadata{}, err
	}
	defer file.Close()

	var metadata ArtifactMetadata
	decoder := json.NewDecoder(io.LimitReader(file, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return ArtifactMetadata{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArtifactMetadata{}, errors.New("artifact metadata has trailing content")
	}
	return metadata, nil
}

func verifyArtifactFile(file *os.File, metadata ArtifactMetadata) error {
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() != metadata.SizeBytes {
		return ErrArtifactCorrupt
	}

	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, MaxArtifactBytes+1))
	if err != nil {
		return err
	}
	if written != metadata.SizeBytes || hex.EncodeToString(hasher.Sum(nil)) != metadata.SHA256 {
		return ErrArtifactCorrupt
	}
	return nil
}
