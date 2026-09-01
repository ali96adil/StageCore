package diagnosticsbundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/doctor"
)

type fakeDoctor struct {
	report doctor.Report
}

func (f fakeDoctor) Run(context.Context, doctor.Options) doctor.Report {
	return f.report
}

type fakeCommands struct{}

func (fakeCommands) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	switch name {
	case "uname":
		return []byte("Linux 6.18.0 aarch64 GNU/Linux\n"), nil
	case "journalctl":
		return []byte(strings.Join([]string{
			`{"time":"2026-09-01T00:00:00Z","level":"ERROR","msg":"probe failed","token":"journal-secret"}`,
			`authorization=Bearer abcdefghijklmnopqrstuvwxyz012345`,
			"-----BEGIN PRIVATE KEY-----\njournal-private-material\n-----END PRIVATE KEY-----",
		}, "\n") + "\n"), nil
	default:
		return nil, nil
	}
}

func TestCreateProducesPrivateRedactedArchive(t *testing.T) {
	root := t.TempDir()
	installRoot := filepath.Join(root, "opt", "stagecore")
	configRoot := filepath.Join(root, "etc", "stagecore")
	unitPath := filepath.Join(root, "etc", "systemd", "system", "stagecore-hub.service")
	dataRoot := filepath.Join(root, "missing-data")
	if err := os.MkdirAll(filepath.Join(installRoot, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "stagecore.env"), []byte(strings.Join([]string{
		"STAGECORE_DATA_ROOT=" + dataRoot,
		"STAGECORE_VAULT_ROOT=" + filepath.Join(root, "vault"),
		"STAGECORE_LISTEN=127.0.0.1:7840",
		"STAGECORE_OSC_PLUGIN_PATH=" + filepath.Join(installRoot, "bin", "stagecore-osc-plugin"),
		"STAGECORE_API_TOKEN=configuration-secret",
		"STAGECORE_CLIENT_SECRET=configuration-client-secret",
		"",
	}, "\n")), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unitPath, []byte("[Service]\nExecStart=/opt/stagecore/bin/stagecore-hub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stagecore", "stagecore-hub", "stagecore-osc-plugin", "stagecore-pairing", "stagecore-setup"} {
		if err := os.WriteFile(filepath.Join(installRoot, "bin", name), []byte("fake-"+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	output := filepath.Join(root, "support.tar.gz")
	service := &Service{
		doctor: fakeDoctor{report: doctor.Report{
			SchemaVersion: doctor.ReportSchemaVersion,
			Overall:       doctor.OverallBlocked,
			Counts:        doctor.Counts{Blocker: 1},
			Checks: []doctor.Check{{
				ID: "test", Status: doctor.Blocker, MessageKey: "test",
				Detail: "request failed token=doctor-secret",
			}},
		}},
		commands: fakeCommands{},
		now:      func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) },
		hostname: func() (string, error) { return "stagecore-test", nil },
	}
	result, err := service.Create(context.Background(), Options{
		OutputPath: output, InstallRoot: installRoot, ConfigRoot: configRoot,
		SystemdUnit: unitPath, HTTPTimeout: time.Second, JournalLines: 100,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Path != output {
		t.Fatalf("Path = %q, want %q", result.Path, output)
	}
	if result.Manifest.DoctorOverall != doctor.OverallBlocked {
		t.Fatalf("DoctorOverall = %s", result.Manifest.DoctorOverall)
	}
	if len(result.Manifest.CollectionErrors) == 0 {
		t.Fatal("expected missing state database to be recorded as a collection warning")
	}
	if result.Manifest.Redactions < 3 {
		t.Fatalf("Redactions = %d, want at least 3", result.Manifest.Redactions)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode = %o, want 600", info.Mode().Perm())
	}

	entries := readArchive(t, output)
	for _, required := range []string{"manifest.json", "doctor.json", "system.json", "deployment.json", "binaries.json", "logs/stagecore-hub.log"} {
		if _, ok := entries[required]; !ok {
			t.Fatalf("archive missing %s; entries=%v", required, mapKeys(entries))
		}
	}
	for _, manifestEntry := range result.Manifest.Entries {
		data, ok := entries[manifestEntry.Path]
		if !ok {
			t.Fatalf("manifest entry %s missing from archive", manifestEntry.Path)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != manifestEntry.SHA256 {
			t.Fatalf("manifest SHA-256 for %s = %s, want %s", manifestEntry.Path, manifestEntry.SHA256, got)
		}
		if manifestEntry.SizeBytes != len(data) {
			t.Fatalf("manifest size for %s = %d, want %d", manifestEntry.Path, manifestEntry.SizeBytes, len(data))
		}
	}
	all := strings.Builder{}
	for _, data := range entries {
		all.Write(data)
	}
	content := all.String()
	for _, secret := range []string{
		"configuration-secret", "configuration-client-secret", "doctor-secret",
		"journal-secret", "abcdefghijklmnopqrstuvwxyz012345", "journal-private-material",
	} {
		if strings.Contains(content, secret) {
			t.Fatalf("support bundle leaked %q", secret)
		}
	}
	if !strings.Contains(content, "<redacted>") && !strings.Contains(content, "<redacted-private-key>") {
		t.Fatal("support bundle did not contain expected redaction markers")
	}
	if strings.Contains(string(entries["deployment.json"]), "STAGECORE_API_TOKEN") || strings.Contains(string(entries["deployment.json"]), "STAGECORE_CLIENT_SECRET") {
		t.Fatalf("deployment allowlist leaked ignored key names: %s", entries["deployment.json"])
	}
}

func TestCreateRefusesToOverwriteExistingBundle(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "support.tar.gz")
	if err := os.WriteFile(output, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService()
	_, err := service.Create(context.Background(), Options{OutputPath: output})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Create() error = %v, want existing-output refusal", err)
	}
	content, readErr := os.ReadFile(output)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "existing" {
		t.Fatalf("existing output was modified: %q", content)
	}
}

func TestNormalizeOptionsRejectsUnboundedJournalRequest(t *testing.T) {
	_, err := normalizeOptions(Options{JournalLines: MaxJournalLines + 1})
	if err == nil {
		t.Fatal("normalizeOptions() accepted excessive journal line request")
	}
}

func TestTailBytesKeepsMostRecentJournalContent(t *testing.T) {
	got := string(tailBytes([]byte("0123456789"), 4))
	if !strings.HasSuffix(got, "6789") {
		t.Fatalf("tailBytes() = %q", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("tailBytes() missing truncation marker: %q", got)
	}
}

func readArchive(t *testing.T, path string) map[string][]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	entries := map[string][]byte{}
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = data
	}
	return entries
}

func mapKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
