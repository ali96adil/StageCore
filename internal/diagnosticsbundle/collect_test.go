package diagnosticsbundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
)

func TestCollectDeploymentMetadataUsesStrictAllowlist(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "etc", "stagecore")
	if err := os.MkdirAll(configRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("STAGECORE_DATA_ROOT=/safe/data\nSTAGECORE_LISTEN=127.0.0.1:7840\nSTAGECORE_API_TOKEN=do-not-export\nUNRELATED_PASSWORD=also-do-not-export\n")
	if err := os.WriteFile(filepath.Join(configRoot, "stagecore.env"), content, 0o640); err != nil {
		t.Fatal(err)
	}
	metadata, dataRoot, err := collectDeploymentMetadata(Options{
		InstallRoot: "/opt/stagecore",
		ConfigRoot:  configRoot,
		SystemdUnit: "/etc/systemd/system/stagecore-hub.service",
	})
	if err != nil {
		t.Fatalf("collectDeploymentMetadata() error = %v", err)
	}
	if dataRoot != "/safe/data" {
		t.Fatalf("dataRoot = %q", dataRoot)
	}
	if metadata.IgnoredKeyCount != 2 {
		t.Fatalf("IgnoredKeyCount = %d, want 2", metadata.IgnoredKeyCount)
	}
	if _, ok := metadata.SafeValues["STAGECORE_API_TOKEN"]; ok {
		t.Fatal("strict allowlist exported STAGECORE_API_TOKEN")
	}
	if _, ok := metadata.SafeValues["UNRELATED_PASSWORD"]; ok {
		t.Fatal("strict allowlist exported unrelated password")
	}
}

func TestCollectStateSummaryReadsCurrentEmptySchema(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	handle, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	summary, err := collectStateSummary(ctx, dataRoot)
	if err != nil {
		t.Fatalf("collectStateSummary() error = %v", err)
	}
	if summary.DatabaseSchema <= 0 {
		t.Fatalf("DatabaseSchema = %d", summary.DatabaseSchema)
	}
	if summary.Projects != 0 || summary.LocalUsers != 0 || summary.CueExecutions != 0 || summary.EventRecords != 0 {
		t.Fatalf("unexpected non-empty summary: %#v", summary)
	}
	if summary.Sessions == nil || summary.Companions == nil || summary.Extensions == nil {
		t.Fatalf("summary slices must be stable empty arrays: %#v", summary)
	}
}
