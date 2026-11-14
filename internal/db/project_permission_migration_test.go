package db

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db/migrations"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/pressly/goose/v3"
)

func TestProjectPermissionMigrationPreservesLegacyShowIdentity(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), FileName)))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	migrate := func(version int64) {
		t.Helper()
		migrationMu.Lock()
		defer migrationMu.Unlock()
		goose.SetBaseFS(migrations.FS)
		defer goose.SetBaseFS(nil)
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatal(err)
		}
		if err := goose.UpTo(database, ".", version); err != nil {
			t.Fatal(err)
		}
	}
	migrate(52)
	st := store.New(database, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Legacy show", CreatedBy: "migration-test"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := st.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID,
		DisplayLabel: "1", Name: "Opening", Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	before, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, "migration-test")
	if err != nil {
		t.Fatal(err)
	}
	migrate(53)
	after, err := st.GetRuntimeSnapshot(ctx, before.ID)
	if err != nil || after.ProjectID != project.ID || !bytes.Equal(before.Manifest, after.Manifest) {
		t.Fatalf("migration changed published snapshot identity or bytes: %v", err)
	}
	afterCue, err := st.GetCue(ctx, cue.ID)
	if err != nil || afterCue.ID != cue.ID || afterCue.Name != cue.Name {
		t.Fatalf("migration changed Cue identity: %v", err)
	}
	var grants int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM project_permission_grants").Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("migration created unexpected grants: %d (%v)", grants, err)
	}
}

func TestProjectPermissionMigrationRollbackPreservesLegacyShowAndDropsOnlyProspectiveGrants(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), FileName)))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	migrate := func(version int64) {
		t.Helper()
		migrationMu.Lock()
		defer migrationMu.Unlock()
		goose.SetBaseFS(migrations.FS)
		defer goose.SetBaseFS(nil)
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatal(err)
		}
		if err := goose.UpTo(database, ".", version); err != nil {
			t.Fatal(err)
		}
	}
	migrate(52)
	st := store.New(database, clock.Real{})
	project, revision, err := st.CreateProject(ctx, store.CreateProjectParams{Name: "Rollback legacy show", CreatedBy: "migration-test"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := st.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	snapshotBefore, _, err := snapshot.NewBuilder(st).Create(ctx, revision.ID, "migration-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO local_users (user_id, username, password_hash, role, created_at_us, updated_at_us)
		VALUES ('migration-owner', 'migration-owner', 'unused', 'OWNER', 1, 1),
		       ('migration-operator', 'migration-operator', 'unused', 'OPERATOR', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	migrate(53)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO project_memberships (project_id, user_id, created_by_user_id, created_at_us, updated_at_us)
		VALUES (?, 'migration-operator', 'migration-owner', 1, 1);
		INSERT INTO project_permission_grants (project_id, user_id, permission, granted_by_user_id, granted_at_us)
		VALUES (?, 'migration-operator', 'project.view', 'migration-owner', 1)`, project.ID, project.ID); err != nil {
		t.Fatal(err)
	}
	migrationMu.Lock()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		migrationMu.Unlock()
		t.Fatal(err)
	}
	err = goose.DownTo(database, ".", 52)
	goose.SetBaseFS(nil)
	migrationMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"project_memberships", "project_permission_grants"} {
		var count int
		if err := database.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("migration rollback retained %s table: count=%d err=%v", table, count, err)
		}
	}
	storedProject, err := st.GetProject(ctx, project.ID)
	if err != nil || storedProject.CurrentRevisionID != revision.ID {
		t.Fatalf("migration rollback changed legacy Project: project=%+v err=%v", storedProject, err)
	}
	storedCue, err := st.GetCue(ctx, cue.ID)
	if err != nil || storedCue.Name != cue.Name {
		t.Fatalf("migration rollback changed legacy Cue: cue=%+v err=%v", storedCue, err)
	}
	storedSnapshot, err := st.GetRuntimeSnapshot(ctx, snapshotBefore.ID)
	if err != nil || storedSnapshot.ProjectID != project.ID || !bytes.Equal(storedSnapshot.Manifest, snapshotBefore.Manifest) {
		t.Fatalf("migration rollback changed legacy Snapshot: err=%v", err)
	}
	migrate(53)
	var grants int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM project_permission_grants`).Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("re-applied migration created grants: count=%d err=%v", grants, err)
	}
}
