package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/db/migrations"
	"github.com/pressly/goose/v3"
)

func stageLaser58TestDatabase(t *testing.T, legacyColumn bool, values ...int) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "stagecore.sqlite3")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if legacyColumn {
		_, err = database.Exec(`CREATE TABLE stage_device_commands (
			command_id TEXT PRIMARY KEY,
			control_generation INTEGER NOT NULL DEFAULT 0
		)`)
	} else {
		_, err = database.Exec(`CREATE TABLE stage_device_commands (
			command_id TEXT PRIMARY KEY
		)`)
	}
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range values {
		if legacyColumn {
			_, err = database.Exec(
				`INSERT INTO stage_device_commands (command_id, control_generation) VALUES (?, ?)`,
				fmt.Sprintf("command-%03d", i), value)
		} else {
			_, err = database.Exec(
				`INSERT INTO stage_device_commands (command_id) VALUES (?)`, fmt.Sprintf("command-%03d", i))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a deployed database after migration 57, without touching any
	// real StageCore database or relying on Stage Device authorization.
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if _, err := goose.EnsureDBVersion(database); err != nil {
		t.Fatal(err)
	}
	if err := goose.SetVersion(database, 57); err != nil {
		t.Fatal(err)
	}
	return database
}

func TestStageLaser58AdoptsLegacyGenerations(t *testing.T) {
	values := make([]int, 74)
	for i := range values {
		values[i] = i + 1
	}
	database := stageLaser58TestDatabase(t, true, values...)
	if err := migrate(database); err != nil {
		t.Fatal(err)
	}
	version, err := goose.GetDBVersion(database)
	if err != nil || version != 58 {
		t.Fatalf("version=%d err=%v; want 58", version, err)
	}
	var sequence int64
	if err := database.QueryRow(`SELECT generation FROM stagelaser_control_sequence WHERE id=1`).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	if sequence != 74 {
		t.Fatalf("sequence=%d, want 74 (next must be 75)", sequence)
	}
	var actual int
	if err := database.QueryRow(`SELECT COUNT(*) FROM stage_device_commands WHERE control_generation > 0`).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != 74 {
		t.Fatalf("preserved commands=%d, want 74", actual)
	}

	if _, err := database.Exec(`INSERT INTO stage_device_commands (command_id, control_generation)
		VALUES ('duplicate-74', 74)`); err == nil {
		t.Fatal("positive generation uniqueness is not enforced")
	}
	if _, err := database.Exec(`INSERT INTO stage_device_commands (command_id, control_generation)
		VALUES ('oversized', 9007199254740992)`); err == nil {
		t.Fatal("generation upper bound is not enforced for legacy column")
	}
	if _, err := database.Exec(`INSERT INTO stage_device_commands (command_id, control_generation)
		VALUES ('negative', -1)`); err == nil {
		t.Fatal("generation lower bound is not enforced for legacy column")
	}
	if err := migrate(database); err != nil {
		t.Fatalf("second migration run must be idempotent: %v", err)
	}
}

func TestStageLaser58AddsMissingColumn(t *testing.T) {
	database := stageLaser58TestDatabase(t, false, 0, 0)
	if err := migrate(database); err != nil {
		t.Fatal(err)
	}
	var sequence int64
	if err := database.QueryRow(`SELECT generation FROM stagelaser_control_sequence WHERE id=1`).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	if sequence != 0 {
		t.Fatalf("fresh database sequence=%d, want 0", sequence)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM stage_device_commands WHERE control_generation=0`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("backfilled legacy commands=%d, want 2", count)
	}
}

func TestStageLaser58RejectsDuplicateDeployedGeneration(t *testing.T) {
	database := stageLaser58TestDatabase(t, true, 1, 1)
	if err := migrate(database); err == nil {
		t.Fatal("migration accepted duplicate deployed generations")
	}
	version, err := goose.GetDBVersion(database)
	if err != nil || version != 57 {
		t.Fatalf("version=%d err=%v; failed migration must remain at 57", version, err)
	}
}

func TestStageLaser58RejectsInvalidDeployedGeneration(t *testing.T) {
	database := stageLaser58TestDatabase(t, true, -1)
	if err := migrate(database); err == nil {
		t.Fatal("migration accepted invalid deployed generation")
	}
}

func TestStageLaser58RollbackPreservesCommandHistory(t *testing.T) {
	database := stageLaser58TestDatabase(t, true, 74)
	if err := migrate(database); err != nil {
		t.Fatal(err)
	}
	migrationMu.Lock()
	defer migrationMu.Unlock()
	goose.SetBaseFS(migrations.FS)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Down(database, "."); err != nil {
		t.Fatal(err)
	}
	var value int64
	if err := database.QueryRow(`SELECT control_generation FROM stage_device_commands WHERE command_id='command-000'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 74 {
		t.Fatalf("rollback lost command generation: got %d, want 74", value)
	}
}
