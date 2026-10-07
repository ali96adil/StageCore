package deviceupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validArtifactMetadata(payload []byte) ArtifactMetadata {
	hash := sha256.Sum256(payload)
	return ArtifactMetadata{
		ArtifactID:       "018f2744-0cb0-7bf6-9637-4a3a467a7a11",
		DeviceID:         "23c45a07-7286-4afc-91d9-7e54df72aeee",
		ProfileID:        "stagecore.esp32-stagelaser",
		Version:          "0.1.0",
		SourceRevision:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Qualification:    QualificationQualified,
		SizeBytes:        int64(len(payload)),
		SHA256:           hex.EncodeToString(hash[:]),
		OriginalFilename: "stagelaser-0.1.0.bin",
		CreatedAt:        time.Date(2026, 10, 7, 10, 55, 0, 0, time.UTC),
	}
}

func TestArtifactRegistryImportsAndReopensVerifiedArtifact(t *testing.T) {
	registry, err := NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("stagecore-stagelaser-qualified-image")
	metadata := validArtifactMetadata(payload)

	got, err := registry.ImportQualified(metadata, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("import artifact: %v", err)
	}
	if got.ArtifactID != metadata.ArtifactID {
		t.Fatalf("artifact id = %q", got.ArtifactID)
	}
	if path := ArtifactPath(metadata.ArtifactID); path != ArtifactPathPrefix+metadata.ArtifactID+"/firmware.bin" {
		t.Fatalf("artifact path = %q", path)
	}

	file, opened, err := registry.OpenForDevice(metadata.ArtifactID, metadata.DeviceID)
	if err != nil {
		t.Fatalf("open artifact: %v", err)
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("payload = %q", body)
	}
	if opened.SHA256 != metadata.SHA256 || opened.SizeBytes != metadata.SizeBytes {
		t.Fatalf("opened metadata = %#v", opened)
	}
}

func TestArtifactRegistryFailsClosed(t *testing.T) {
	payload := []byte("qualified-image")
	base := validArtifactMetadata(payload)

	tests := []struct {
		name string
		edit func(*ArtifactMetadata)
		want error
	}{
		{
			name: "unqualified",
			edit: func(m *ArtifactMetadata) { m.Qualification = "UNQUALIFIED" },
			want: ErrArtifactInvalid,
		},
		{
			name: "bad hash",
			edit: func(m *ArtifactMetadata) { m.SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" },
			want: ErrArtifactInvalid,
		},
		{
			name: "wrong size",
			edit: func(m *ArtifactMetadata) { m.SizeBytes++ },
			want: ErrArtifactInvalid,
		},
		{
			name: "bad filename",
			edit: func(m *ArtifactMetadata) { m.OriginalFilename = "../firmware.bin" },
			want: ErrArtifactInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := NewArtifactRegistry(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			metadata := base
			tt.edit(&metadata)
			_, err = registry.ImportQualified(metadata, bytes.NewReader(payload))
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestArtifactRegistryIsImmutableAndDeviceBound(t *testing.T) {
	registry, err := NewArtifactRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("qualified-image")
	metadata := validArtifactMetadata(payload)

	if _, err := registry.ImportQualified(metadata, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ImportQualified(metadata, bytes.NewReader(payload)); !errors.Is(err, ErrArtifactConflict) {
		t.Fatalf("second import error = %v", err)
	}
	if file, _, err := registry.OpenForDevice(metadata.ArtifactID, "12345678-1234-1234-1234-123456789012"); !errors.Is(err, ErrArtifactNotFound) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("cross-device open error = %v", err)
	}
}

func TestArtifactRegistryDetectsOnDiskCorruption(t *testing.T) {
	root := t.TempDir()
	registry, err := NewArtifactRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("qualified-image")
	metadata := validArtifactMetadata(payload)
	if _, err := registry.ImportQualified(metadata, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, metadata.ArtifactID, artifactFilename)
	if err := os.WriteFile(path, []byte("tampered-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, _, err := registry.OpenForDevice(metadata.ArtifactID, metadata.DeviceID)
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, ErrArtifactCorrupt) {
		t.Fatalf("corruption error = %v", err)
	}
}
