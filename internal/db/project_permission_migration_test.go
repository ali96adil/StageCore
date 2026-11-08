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
