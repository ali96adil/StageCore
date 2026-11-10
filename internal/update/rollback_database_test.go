package update

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/doctor"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type rollbackFixtureInstaller func(context.Context, deployment.Options) (deployment.Result, error)

func (f rollbackFixtureInstaller) Install(ctx context.Context, opts deployment.Options) (deployment.Result, error) {
	return f(ctx, opts)
}

func TestIsolatedUpdateRollbackRestoresDatabaseAndPublishedSnapshot(t *testing.T) {
	ctx := context.Background()
	fixture := makeSnapshotFixture(t, t.TempDir())
	open := func() *db.Handle {
		t.Helper()
		handle, err := db.Open(ctx, db.Config{DataRoot: fixture.DataRoot})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = handle.Close() })
		return handle
	}

	baseline := open()
	baselineStore := store.New(baseline.DB, clock.Real{})
	project, revision, err := baselineStore.CreateProject(ctx, store.CreateProjectParams{Name: "Rollback rehearsal"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := baselineStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, Name: "Preserved Cue", OrderIndex: 0, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := baselineStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(baselineStore).Create(ctx, revision.ID, "rollback-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := baseline.Close(); err != nil {
		t.Fatal(err)
	}

	candidateProject := ""
	installer := rollbackFixtureInstaller(func(ctx context.Context, opts deployment.Options) (deployment.Result, error) {
		if !opts.DryRun {
			candidate := open()
			candidateStore := store.New(candidate.DB, clock.Real{})
			added, _, err := candidateStore.CreateProject(ctx, store.CreateProjectParams{Name: "Candidate-only project"})
			if err != nil {
				t.Fatal(err)
			}
			candidateProject = added.ID
			if _, err := candidate.DB.ExecContext(ctx, "DELETE FROM runtime_snapshots WHERE runtime_snapshot_id = ?", published.ID); err != nil {
				t.Fatal(err)
			}
			if err := candidate.Close(); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(fixture.InstallRoot, "bin", "stagecore-hub"), "candidate-hub", 0o755)
		}
		return deployment.Result{Effective: opts}, nil
	})

	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }
	service := &Service{
		installer: installer,
		doctor:    &fakeDoctor{reports: []doctor.Report{readyReport(), blockedReport(), readyReport()}},
		snapshots: snapshotter,
		commands:  &fakeCommands{},
		euid:      func() int { return 0 },
		showGuard: activeShow,
	}
	opts := testOptions(false)
	opts.Deployment.BundleDir = filepath.Join(filepath.Dir(fixture.BackupRoot), "candidate")
	opts.Deployment.InstallRoot = fixture.InstallRoot
	opts.Deployment.ConfigRoot = fixture.ConfigRoot
	opts.Deployment.DataRoot = fixture.DataRoot
	opts.Deployment.VaultRoot = filepath.Join(fixture.DataRoot, "vault")
	opts.Deployment.SystemdUnit = fixture.SystemdUnit
	opts.BackupRoot = fixture.BackupRoot
	result, err := service.Apply(ctx, opts)
	if !errors.Is(err, ErrPostflightBlocked) || errors.Is(err, ErrRollbackFailed) ||
		!result.RolledBack || result.RollbackReport.Overall != doctor.OverallReady || candidateProject == "" {
		t.Fatalf("expected verified automatic rollback after candidate changed the database: %+v, %v", result, err)
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(fixture.InstallRoot, "bin", "stagecore-hub"), "old-hub")

	restored := open()
	var integrity string
	if err := restored.DB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("database integrity: %q, %v", integrity, err)
	}
	rows, err := restored.DB.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		_ = rows.Close()
		t.Fatal("restored database has foreign key violations")
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	restoredStore := store.New(restored.DB, clock.Real{})
	if _, err := restoredStore.GetProject(ctx, project.ID); err != nil {
		t.Fatalf("baseline project missing: %v", err)
	}
	if _, err := restoredStore.GetProject(ctx, candidateProject); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("candidate project survived rollback: %v", err)
	}
	got, err := restoredStore.GetRuntimeSnapshot(ctx, published.ID)
	if err != nil || got.ProjectID != project.ID || got.RevisionID != revision.ID ||
		got.SnapshotVersion != published.SnapshotVersion || got.Status != domain.SnapshotPublished ||
		got.ContentHash != published.ContentHash || !bytes.Equal(got.Manifest, published.Manifest) {
		t.Fatalf("published snapshot changed after rollback: %+v, %v", got, err)
	}
	manifest, err := snapshot.Decode(got.Manifest)
	if err != nil || len(manifest.Cues) != 1 || manifest.Cues[0].ID != cue.ID {
		t.Fatalf("restored snapshot lost its Cue: %+v, %v", manifest.Cues, err)
	}
}
