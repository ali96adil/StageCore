package deviceupdate

import (
	"strings"
	"testing"
	"time"
)

func validManifest(now time.Time) Manifest {
	return Manifest{
		SchemaVersion:    SchemaVersion1,
		UpdateID:         "018f2744-0cb0-7bf6-9637-4a3a467a7a01",
		DeviceID:         "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ProfileID:        "stagecore.esp32-stagelaser",
		CurrentVersion:   "0.1.0-dev.1",
		TargetVersion:    "0.1.0",
		SourceRevision:   strings.Repeat("a", 40),
		Qualification:    QualificationQualified,
		ArtifactPath:     ArtifactPathPrefix + "018f2744-0cb0-7bf6-9637-4a3a467a7a01/firmware.bin",
		ArtifactSize:     1_000_000,
		ArtifactSHA256:   strings.Repeat("b", 64),
		IssuedAt:         now.Add(-time.Minute),
		ExpiresAt:        now.Add(10 * time.Minute),
		RollbackRequired: true,
	}
}

func validContext(now time.Time) ValidationContext {
	return ValidationContext{
		ExpectedDeviceID:  "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ExpectedProfileID: "stagecore.esp32-stagelaser",
		Now:               now,
		RequireRollback:   true,
	}
}

func TestValidateManifestAcceptsQualifiedPinnedHubArtifact(t *testing.T) {
	now := time.Date(2026, 10, 7, 6, 50, 0, 0, time.UTC)
	if err := ValidateManifest(validManifest(now), validContext(now)); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestValidateManifestFailsClosed(t *testing.T) {
	now := time.Date(2026, 10, 7, 6, 50, 0, 0, time.UTC)

	tests := []struct {
		name string
		edit func(*Manifest, *ValidationContext)
		want string
	}{
		{
			name: "wrong device",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.DeviceID = "12345678-1234-1234-1234-123456789012"
			},
			want: "device_id does not match",
		},
		{
			name: "wrong profile",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ProfileID = "stagecore.esp32-lighting"
			},
			want: "profile_id does not match",
		},
		{
			name: "unqualified artifact",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.Qualification = "UNQUALIFIED"
			},
			want: "qualification must be QUALIFIED",
		},
		{
			name: "absolute external URL",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ArtifactPath = "https://example.com/firmware.bin"
			},
			want: "Hub-local",
		},
		{
			name: "path traversal",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ArtifactPath = ArtifactPathPrefix + "../secret.bin"
			},
			want: "canonical",
		},
		{
			name: "query string",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ArtifactPath += "?token=secret"
			},
			want: "Hub-local",
		},
		{
			name: "oversized artifact",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ArtifactSize = MaxArtifactBytes + 1
			},
			want: "artifact_size",
		},
		{
			name: "bad sha256",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ArtifactSHA256 = strings.Repeat("A", 64)
			},
			want: "canonical lowercase SHA-256",
		},
		{
			name: "bad source revision",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.SourceRevision = strings.Repeat("g", 40)
			},
			want: "source_revision",
		},
		{
			name: "expired",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ExpiresAt = now.Add(-time.Second)
			},
			want: "expired",
		},
		{
			name: "too long lived",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.ExpiresAt = m.IssuedAt.Add(MaxManifestLifetime + time.Second)
			},
			want: "lifetime",
		},
		{
			name: "issued too far in future",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.IssuedAt = now.Add(MaxFutureClockSkew + time.Second)
				m.ExpiresAt = m.IssuedAt.Add(time.Minute)
			},
			want: "future",
		},
		{
			name: "rollback required",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.RollbackRequired = false
			},
			want: "rollback_required",
		},
		{
			name: "same version",
			edit: func(m *Manifest, _ *ValidationContext) {
				m.TargetVersion = m.CurrentVersion
			},
			want: "must differ",
		},
		{
			name: "missing validation time",
			edit: func(_ *Manifest, c *ValidationContext) {
				c.Now = time.Time{}
			},
			want: "validation time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validManifest(now)
			context := validContext(now)
			tt.edit(&manifest, &context)

			err := ValidateManifest(manifest, context)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got error %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRollbackCanBeOptionalForAnotherDevicePolicy(t *testing.T) {
	now := time.Date(2026, 10, 7, 6, 50, 0, 0, time.UTC)
	manifest := validManifest(now)
	manifest.RollbackRequired = false

	context := validContext(now)
	context.RequireRollback = false

	if err := ValidateManifest(manifest, context); err != nil {
		t.Fatalf("manifest rejected under non-rollback policy: %v", err)
	}
}
