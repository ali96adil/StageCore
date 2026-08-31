package doctor

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deployment"
	_ "modernc.org/sqlite"
)

func TestHealthyDoctorReportUsesReadOnlyLocalSources(t *testing.T) {
	root := t.TempDir()
	installRoot := filepath.Join(root, "opt", "stagecore")
	configRoot := filepath.Join(root, "etc", "stagecore")
	dataRoot := filepath.Join(root, "data")
	vaultRoot := filepath.Join(root, "vault")
	unitPath := filepath.Join(root, "stagecore-hub.service")
	for _, path := range []string{filepath.Join(installRoot, "bin"), configRoot, filepath.Join(dataRoot, "db"), vaultRoot} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range deployment.RequiredBinaries {
		if err := os.WriteFile(filepath.Join(installRoot, "bin", name), []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health/live":
			_, _ = w.Write([]byte(`{"status":"LIVE"}`))
		case "/health/ready":
			_, _ = w.Write([]byte(`{"status":"READY","storage_state":"HEALTHY"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	listen := strings.TrimPrefix(server.URL, "http://")

	env := strings.Join([]string{
		"STAGECORE_DATA_ROOT=" + dataRoot,
		"STAGECORE_VAULT_ROOT=" + vaultRoot,
		"STAGECORE_LISTEN=" + listen,
		"STAGECORE_OSC_PLUGIN_PATH=" + filepath.Join(installRoot, "bin", "stagecore-osc-plugin"),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(configRoot, "stagecore.env"), []byte(env), 0o640); err != nil {
		t.Fatal(err)
	}
	unit := "EnvironmentFile=" + filepath.Join(configRoot, "stagecore.env") + "\nExecStart=" + filepath.Join(installRoot, "bin", "stagecore-hub") + "\n"
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	createDoctorTestDatabase(t, filepath.Join(dataRoot, "db", "stagecore.sqlite3"))

	runner := NewRunner()
	runner.HTTPClient = server.Client()
	runner.Now = func() time.Time { return time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC) }
	runner.StatFS = func(string) (Filesystem, error) {
		return Filesystem{TotalBytes: 100 << 30, FreeBytes: 50 << 30}, nil
	}
	runner.Command = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "is-enabled" {
			return []byte("enabled\n"), nil
		}
		if len(args) > 0 && args[0] == "is-active" {
			return []byte("active\n"), nil
		}
		return nil, errors.New("unexpected command")
	}

	report := runner.Run(context.Background(), Options{InstallRoot: installRoot, ConfigRoot: configRoot, SystemdUnit: unitPath, HTTPTimeout: time.Second})
	if report.Overall != OverallReady || report.ExitCode() != 0 {
		t.Fatalf("overall=%s exit=%d checks=%+v", report.Overall, report.ExitCode(), report.Checks)
	}
	if report.Counts.Ready != len(report.Checks) || report.Counts.Blocker != 0 || report.Counts.Warning != 0 || report.Counts.Advisory != 0 {
		t.Fatalf("unexpected counts: %+v", report.Counts)
	}
	if got := findCheck(t, report, "database.readonly"); got.MessageKey != "database.ok" || got.MessageArgs[0] != "15" {
		t.Fatalf("database check=%+v", got)
	}
	if got := findCheck(t, report, "pairing.summary"); got.Status != Ready || !strings.Contains(got.Detail, "trusted_ready=1") {
		t.Fatalf("pairing check=%+v", got)
	}
}

func TestDoctorBlocksOnInactiveHubButKeepsOtherEvidence(t *testing.T) {
	report := Report{Checks: []Check{
		{ID: "systemd.active", Status: Blocker, MessageKey: "service.inactive"},
		{ID: "storage.data", Status: Ready, MessageKey: "storage.ok"},
		{ID: "systemd.enabled", Status: Warning, MessageKey: "service.disabled"},
		{ID: "hub.ready", Status: Advisory, MessageKey: "check.skipped.hub_live"},
	}}
	finalizeReport(&report)
	if report.Overall != OverallBlocked || report.ExitCode() != 1 {
		t.Fatalf("overall=%s exit=%d", report.Overall, report.ExitCode())
	}
	if report.Counts.Blocker != 1 || report.Counts.Warning != 1 || report.Counts.Advisory != 1 || report.Counts.Ready != 1 {
		t.Fatalf("counts=%+v", report.Counts)
	}
}

func TestMissingConfigSkipsDependentChecksWithoutGuessingPaths(t *testing.T) {
	root := t.TempDir()
	installRoot := filepath.Join(root, "opt", "stagecore")
	if err := os.MkdirAll(filepath.Join(installRoot, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range deployment.RequiredBinaries {
		if err := os.WriteFile(filepath.Join(installRoot, "bin", name), []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	unitPath := filepath.Join(root, "stagecore-hub.service")
	if err := os.WriteFile(unitPath, []byte("EnvironmentFile="+filepath.Join(root, "etc", "stagecore", "stagecore.env")+"\nExecStart="+filepath.Join(installRoot, "bin", "stagecore-hub")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner()
	runner.Command = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "is-enabled" {
			return []byte("enabled\n"), nil
		}
		return []byte("active\n"), nil
	}
	report := runner.Run(context.Background(), Options{InstallRoot: installRoot, ConfigRoot: filepath.Join(root, "etc", "stagecore"), SystemdUnit: unitPath})
	if findCheck(t, report, "deployment.config").Status != Blocker {
		t.Fatalf("config check=%+v", findCheck(t, report, "deployment.config"))
	}
	for _, id := range []string{"storage.data", "storage.vault", "database.readonly", "pairing.summary", "hub.live", "hub.ready"} {
		if got := findCheck(t, report, id); got.Status != Advisory {
			t.Fatalf("%s status=%s, want advisory", id, got.Status)
		}
	}
}

func TestArabicHumanOutputAndStableJSON(t *testing.T) {
	report := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   time.Date(2026, 8, 31, 7, 0, 0, 0, time.UTC),
		Overall:       OverallWarning,
		Counts:        Counts{Ready: 1, Warning: 1},
		Checks: []Check{
			{ID: "hub.live", Status: Ready, MessageKey: "hub.live_ok"},
			{ID: "systemd.enabled", Status: Warning, MessageKey: "service.disabled", RemedyKey: "remedy.service.enable"},
		},
	}
	var human bytes.Buffer
	WriteHuman(&human, report, LocaleArabic)
	for _, marker := range []string{"طبيب StageCore", "تحذير", "الإجراء", "sudo systemctl enable"} {
		if !strings.Contains(human.String(), marker) {
			t.Fatalf("Arabic output missing %q: %s", marker, human.String())
		}
	}
	var machine bytes.Buffer
	if err := WriteJSON(&machine, report); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`"schema_version": 1`, `"overall": "WARNING"`, `"id": "hub.live"`, `"message_key": "hub.live_ok"`} {
		if !strings.Contains(machine.String(), marker) {
			t.Fatalf("JSON output missing %q: %s", marker, machine.String())
		}
	}
}

func TestReadOnlySQLiteDSNDoesNotPermitCreationOrWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite3")
	database, err := sql.Open("sqlite", readOnlySQLiteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Ping(); err == nil {
		t.Fatal("read-only DSN unexpectedly created a missing database")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing DB path was changed: %v", err)
	}
}

func createDoctorTestDatabase(t *testing.T, path string) {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	statements := []string{
		`CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER NOT NULL, is_applied INTEGER NOT NULL, tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO goose_db_version(version_id, is_applied) VALUES (15, 1)`,
		`CREATE TABLE companions (companion_id TEXT PRIMARY KEY, trust_state TEXT NOT NULL, readiness TEXT NOT NULL)`,
		`INSERT INTO companions(companion_id, trust_state, readiness) VALUES ('00000000-0000-0000-0000-000000000001', 'TRUSTED', 'READY')`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

func findCheck(t *testing.T, report Report, id string) Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("check %s not found", id)
	return Check{}
}
