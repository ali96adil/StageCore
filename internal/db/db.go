package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ali96adil/StageCore/internal/db/migrations"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

const FileName = "stagecore.sqlite3"

var migrationMu sync.Mutex

type Config struct {
	DataRoot string
}

type Handle struct {
	DB   *sql.DB
	Path string
}

func Open(ctx context.Context, cfg Config) (*Handle, error) {
	if strings.TrimSpace(cfg.DataRoot) == "" {
		return nil, fmt.Errorf("data root is required")
	}

	dbDir := filepath.Join(cfg.DataRoot, "db")
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	path := filepath.Join(dbDir, FileName)
	dsn := sqliteDSN(path)
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	closeOnError := func(err error) (*Handle, error) {
		_ = database.Close()
		return nil, err
	}

	if err := database.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("ping sqlite: %w", err))
	}
	if err := VerifyConnection(ctx, database); err != nil {
		return closeOnError(err)
	}
	if err := migrate(database); err != nil {
		return closeOnError(fmt.Errorf("apply migrations: %w", err))
	}
	if err := readiness(ctx, database); err != nil {
		return closeOnError(err)
	}

	return &Handle{DB: database, Path: path}, nil
}

func (h *Handle) Close() error {
	if h == nil || h.DB == nil {
		return nil
	}
	return h.DB.Close()
}

func sqliteDSN(path string) string {
	q := url.Values{}
	q.Set("_foreign_keys", "on")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	q.Set("_busy_timeout", "5000")
	q.Set("_defensive", "1")
	q.Set("_dqs", "0")
	q.Add("_pragma", "trusted_schema(OFF)")
	q.Set("_txlock", "immediate")

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: q.Encode()}
	return u.String()
}

func VerifyConnection(ctx context.Context, database *sql.DB) error {
	checks := []struct {
		query string
		want  int
		name  string
	}{
		{"PRAGMA foreign_keys", 1, "foreign_keys"},
		{"PRAGMA synchronous", 2, "synchronous"},
		{"PRAGMA busy_timeout", 5000, "busy_timeout"},
		{"PRAGMA trusted_schema", 0, "trusted_schema"},
	}
	for _, check := range checks {
		var got int
		if err := database.QueryRowContext(ctx, check.query).Scan(&got); err != nil {
			return fmt.Errorf("verify PRAGMA %s: %w", check.name, err)
		}
		if got != check.want {
			return fmt.Errorf("PRAGMA %s=%d, want %d", check.name, got, check.want)
		}
	}

	var journalMode string
	if err := database.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("verify PRAGMA journal_mode: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("PRAGMA journal_mode=%q, want WAL", journalMode)
	}

	var ignored string
	if err := database.QueryRowContext(ctx, `SELECT "stagecore_missing_identifier"`).Scan(&ignored); err == nil {
		return errors.New("sqlite DQS compatibility is enabled")
	}

	_, _ = database.ExecContext(ctx, "PRAGMA writable_schema=ON")
	var writableSchema int
	if err := database.QueryRowContext(ctx, "PRAGMA writable_schema").Scan(&writableSchema); err != nil {
		return fmt.Errorf("verify defensive mode: %w", err)
	}
	if writableSchema != 0 {
		return errors.New("sqlite defensive mode is not effective")
	}

	return nil
}

func migrate(database *sql.DB) error {
	migrationMu.Lock()
	defer migrationMu.Unlock()

	migrationFS, err := fs.Sub(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration fs: %w", err)
	}
	goose.SetBaseFS(migrationFS)
	defer goose.SetBaseFS(nil)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	// Migration 58 adopts existing StageLaser command generations from
	// deployed databases. SQLite has no ADD COLUMN IF NOT EXISTS, so first
	// migrate to 57, then add the column only on schemas that lack it.
	if err := goose.UpTo(database, ".", 57); err != nil {
		return err
	}
	version, err := goose.GetDBVersion(database)
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if version == 57 {
		if err := ensureStageLaserControlGenerationColumn(database); err != nil {
			return fmt.Errorf("prepare StageLaser migration 58: %w", err)
		}
	}
	if err := goose.Up(database, "."); err != nil {
		return err
	}
	return nil
}

// ensureStageLaserControlGenerationColumn preserves legacy command generations.
// The SQL migration seeds its monotonic sequence from MAX(control_generation).
func ensureStageLaserControlGenerationColumn(database *sql.DB) error {
	rows, err := database.Query(`PRAGMA table_info("stage_device_commands")`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if name != "control_generation" {
			continue
		}
		found = true
		if !strings.EqualFold(strings.TrimSpace(columnType), "INTEGER") || notNull != 1 ||
			!defaultValue.Valid || strings.Trim(defaultValue.String, " ()") != "0" {
			_ = rows.Close()
			return fmt.Errorf("existing control_generation column has unsupported definition")
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !found {
		if _, err := database.Exec(`ALTER TABLE stage_device_commands ADD COLUMN control_generation
			INTEGER NOT NULL DEFAULT 0 CHECK (control_generation BETWEEN 0 AND 9007199254740991)`); err != nil {
			return err
		}
	}

	var invalid int
	if err := database.QueryRow(`SELECT COUNT(*) FROM stage_device_commands
		WHERE control_generation IS NULL
		   OR control_generation < 0 OR control_generation > 9007199254740991`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("found %d invalid existing control_generation values", invalid)
	}
	var duplicate int
	err = database.QueryRow(`SELECT control_generation FROM stage_device_commands
		WHERE control_generation > 0
		GROUP BY control_generation HAVING COUNT(*) > 1 LIMIT 1`).Scan(&duplicate)
	if err == nil {
		return fmt.Errorf("duplicate existing control_generation value %d", duplicate)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func readiness(ctx context.Context, database *sql.DB) error {
	var one int
	if err := database.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("database readiness query: %w", err)
	}
	if one != 1 {
		return errors.New("database readiness query returned unexpected value")
	}
	return nil
}

func SchemaVersion(database *sql.DB) (int64, error) {
	migrationMu.Lock()
	defer migrationMu.Unlock()
	return goose.GetDBVersion(database)
}

func Backup(ctx context.Context, database *sql.DB, destination string) error {
	if strings.TrimSpace(destination) == "" {
		return fmt.Errorf("backup destination is required")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove previous backup destination: %w", err)
	}
	if _, err := database.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("vacuum into backup: %w", err)
	}
	return nil
}
