package db

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db/migrations"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/pressly/goose/v3"
)

var fixedMigrationTime = time.Date(2026, 8, 30, 18, 0, 0, 0, time.UTC)

func TestSessionFoundationMigrationPreservesHistoricalSessionTruth(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), FileName)
	database, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := VerifyConnection(ctx, database); err != nil {
		t.Fatal(err)
	}

	migrationFS, err := fs.Sub(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	migrationMu.Lock()
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		goose.SetBaseFS(nil)
		migrationMu.Unlock()
		t.Fatal(err)
	}
	if err := goose.UpTo(database, ".", 12); err != nil {
		goose.SetBaseFS(nil)
		migrationMu.Unlock()
		t.Fatal(err)
	}
	goose.SetBaseFS(nil)
	migrationMu.Unlock()

	s := store.New(database, clock.Fixed{Time: fixedMigrationTime})
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Historical M6 Session"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Historical Cue", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "pre-f027")
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "corr-m6", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, session.ID, cue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, execution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	migrationMu.Lock()
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		goose.SetBaseFS(nil)
		migrationMu.Unlock()
		t.Fatal(err)
	}
	if err := goose.UpTo(database, ".", 13); err != nil {
		goose.SetBaseFS(nil)
		migrationMu.Unlock()
		t.Fatal(err)
	}
	goose.SetBaseFS(nil)
	migrationMu.Unlock()

	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ContractVersion != domain.SessionContractVersion1 {
		t.Fatalf("contract version=%d", loaded.ContractVersion)
	}
	if loaded.StartPosition.Kind != domain.SessionStartUnspecified {
		t.Fatalf("historical start=%s want UNSPECIFIED", loaded.StartPosition.Kind)
	}
	if loaded.LifecycleState != domain.SessionLifecycleActive {
		t.Fatalf("historical lifecycle=%s want ACTIVE", loaded.LifecycleState)
	}
	if loaded.CurrentCueID == nil || *loaded.CurrentCueID != cue.ID || loaded.LastCompletedCueID == nil || *loaded.LastCompletedCueID != cue.ID {
		t.Fatalf("historical progress current=%v last=%v", loaded.CurrentCueID, loaded.LastCompletedCueID)
	}
	if loaded.StateTruth.RestorationStatus != domain.SessionRestorationNotAssessed || loaded.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("historical truth=%+v", loaded.StateTruth)
	}
}
