package db_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
)

func TestOpenAppliesMigrationAndRequiredPragmas(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	if err := db.VerifyConnection(ctx, h.DB); err != nil {
		t.Fatal(err)
	}
	version, err := db.SchemaVersion(h.DB)
	if err != nil {
		t.Fatal(err)
	}
	if version != 16 {
		t.Fatalf("schema version=%d, want 16", version)
	}
}

func TestBackupCreatesReopenableDatabase(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.DB.ExecContext(ctx, `INSERT INTO projects (project_id, name, lifecycle_state, created_at_us, updated_at_us) VALUES ('00000000-0000-7000-8000-000000000001', 'Backup Test', 'ACTIVE', 1, 1)`); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(t.TempDir(), "backup.sqlite3")
	if err := db.Backup(ctx, h.DB, backupPath); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatal(err)
	}

	backupRoot := t.TempDir()
	backupDBPath := filepath.Join(backupRoot, "db", db.FileName)
	if err := os.MkdirAll(filepath.Dir(backupDBPath), 0o750); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupDBPath, content, 0o640); err != nil {
		t.Fatal(err)
	}
	copyHandle, err := db.Open(ctx, db.Config{DataRoot: backupRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer copyHandle.Close()
	var name string
	if err := copyHandle.DB.QueryRowContext(ctx, `SELECT name FROM projects WHERE project_id = '00000000-0000-7000-8000-000000000001'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Backup Test" {
		t.Fatalf("name=%q", name)
	}
}
