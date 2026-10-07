package deviceupdate

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	SchemaVersion1       = 1
	QualificationQualified = "QUALIFIED"
	ArtifactPathPrefix   = "/api/v1/stage-device-firmware/artifacts/"
	MaxArtifactBytes     = int64(64 << 20)
	MaxManifestLifetime  = 30 * time.Minute
	MaxFutureClockSkew   = 30 * time.Second
)

type Manifest struct {
	SchemaVersion    int       `json:"schema_version"`
	UpdateID         string    `json:"update_id"`
	DeviceID         string    `json:"device_id"`
	ProfileID        string    `json:"profile_id"`
	CurrentVersion   string    `json:"current_version"`
	TargetVersion    string    `json:"target_version"`
	SourceRevision   string    `json:"source_revision"`
	Qualification    string    `json:"qualification"`
	ArtifactPath     string    `json:"artifact_path"`
	ArtifactSize     int64     `json:"artifact_size"`
	ArtifactSHA256   string    `json:"artifact_sha256"`
	IssuedAt         time.Time `json:"issued_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	RollbackRequired bool      `json:"rollback_required"`
}

type ValidationContext struct {
	ExpectedDeviceID string
	ExpectedProfileID string
	Now              time.Time
	RequireRollback  bool
}

func ValidateManifest(manifest Manifest, context ValidationContext) error {
	if manifest.SchemaVersion != SchemaVersion1 {
		return fmt.Errorf("unsupported firmware manifest schema version %d", manifest.SchemaVersion)
	}

	if _, err := uuid.Parse(manifest.UpdateID); err != nil {
		return fmt.Errorf("update_id must be a UUID")
	}
	if _, err := uuid.Parse(manifest.DeviceID); err != nil {
		return fmt.Errorf("device_id must be a UUID")
	}

	if strings.TrimSpace(context.ExpectedDeviceID) == "" ||
		manifest.DeviceID != context.ExpectedDeviceID {
		return fmt.Errorf("firmware manifest device_id does not match the target device")
	}
	if strings.TrimSpace(context.ExpectedProfileID) == "" ||
		manifest.ProfileID != context.ExpectedProfileID {
		return fmt.Errorf("firmware manifest profile_id does not match the target profile")
	}

	if err := validateVersion("current_version", manifest.CurrentVersion); err != nil {
		return err
	}
	if err := validateVersion("target_version", manifest.TargetVersion); err != nil {
		return err
	}
	if manifest.CurrentVersion == manifest.TargetVersion {
		return fmt.Errorf("target_version must differ from current_version")
	}

	if !canonicalLowerHex(manifest.SourceRevision, 40) {
		return fmt.Errorf("source_revision must be a canonical 40-character lowercase Git revision")
	}
	if manifest.Qualification != QualificationQualified {
		return fmt.Errorf("firmware artifact qualification must be QUALIFIED")
	}

	if err := validateArtifactPath(manifest.ArtifactPath); err != nil {
		return err
	}
	if manifest.ArtifactSize <= 0 || manifest.ArtifactSize > MaxArtifactBytes {
		return fmt.Errorf("artifact_size must be within 1..%d bytes", MaxArtifactBytes)
	}
	if !canonicalLowerHex(manifest.ArtifactSHA256, 64) {
		return fmt.Errorf("artifact_sha256 must be canonical lowercase SHA-256")
	}

	now := context.Now
	if now.IsZero() {
		return fmt.Errorf("validation time is required")
	}
	if manifest.IssuedAt.IsZero() || manifest.ExpiresAt.IsZero() {
		return fmt.Errorf("issued_at and expires_at are required")
	}
	if manifest.IssuedAt.After(now.Add(MaxFutureClockSkew)) {
		return fmt.Errorf("issued_at is too far in the future")
	}
	if !manifest.ExpiresAt.After(now) {
		return fmt.Errorf("firmware manifest is expired")
	}
	if !manifest.ExpiresAt.After(manifest.IssuedAt) {
		return fmt.Errorf("expires_at must be after issued_at")
	}
	if manifest.ExpiresAt.Sub(manifest.IssuedAt) > MaxManifestLifetime {
		return fmt.Errorf("firmware manifest lifetime exceeds %s", MaxManifestLifetime)
	}

	if context.RequireRollback && !manifest.RollbackRequired {
		return fmt.Errorf("rollback_required must be true for this device policy")
	}

	return nil
}

func validateVersion(field, value string) error {
	if value != strings.TrimSpace(value) || value == "" || len(value) > 128 {
		return fmt.Errorf("%s must be non-empty, trimmed and at most 128 characters", field)
	}
	for _, r := range value {
		if r <= 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains control or whitespace characters", field)
		}
	}
	return nil
}

func validateArtifactPath(value string) error {
	if value != strings.TrimSpace(value) || value == "" {
		return fmt.Errorf("artifact_path must be a non-empty trimmed Hub-local path")
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("artifact_path must use URL path separators")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("artifact_path is invalid: %w", err)
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("artifact_path must be Hub-local and must not contain scheme, host, query or fragment")
	}
	if !strings.HasPrefix(parsed.Path, ArtifactPathPrefix) {
		return fmt.Errorf("artifact_path must be under %s", ArtifactPathPrefix)
	}
	if path.Clean(parsed.Path) != parsed.Path || strings.Contains(parsed.Path, "..") ||
		strings.HasSuffix(parsed.Path, "/") {
		return fmt.Errorf("artifact_path must be canonical and address one artifact")
	}
	if len(strings.TrimPrefix(parsed.Path, ArtifactPathPrefix)) == 0 {
		return fmt.Errorf("artifact_path must identify an artifact")
	}
	return nil
}

func canonicalLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
