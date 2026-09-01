package update

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
)

func TestSnapshotCreateRestoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	opts := makeSnapshotFixture(t, root)
	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }

	snapshot, err := snapshotter.Create(context.Background(), opts)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if snapshot.Path == "" || len(snapshot.Manifest.Items) != 4 {
		t.Fatalf("unexpected snapshot = %#v", snapshot)
	}

	mustWrite(t, filepath.Join(opts.InstallRoot, "bin", "stagecore-hub"), "candidate-hub", 0o755)
	mustWrite(t, filepath.Join(opts.ConfigRoot, "stagecore.env"), "candidate=config\n", 0o640)
	mustWrite(t, filepath.Join(opts.DataRoot, "db", "state.txt"), "candidate-state", 0o640)
	mustWrite(t, opts.SystemdUnit, "candidate-unit", 0o644)

	if err := snapshotter.Restore(context.Background(), snapshot); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	assertFile(t, filepath.Join(opts.InstallRoot, "bin", "stagecore-hub"), "old-hub")
	assertFile(t, filepath.Join(opts.ConfigRoot, "stagecore.env"), "old=config\n")
	assertFile(t, filepath.Join(opts.DataRoot, "db", "state.txt"), "old-state")
	assertFile(t, opts.SystemdUnit, "old-unit")
}

func TestSnapshotRestoreRejectsTamperingBeforeMutation(t *testing.T) {
	root := t.TempDir()
	opts := makeSnapshotFixture(t, root)
	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }

	snapshot, err := snapshotter.Create(context.Background(), opts)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	mustWrite(t, filepath.Join(opts.InstallRoot, "bin", "stagecore-hub"), "candidate-hub", 0o755)
	mustWrite(t, filepath.Join(snapshot.Path, "managed-binaries", "stagecore-hub"), "tampered", 0o755)

	err = snapshotter.Restore(context.Background(), snapshot)
	if !errors.Is(err, ErrSnapshotIntegrity) {
		t.Fatalf("Restore() error = %v, want ErrSnapshotIntegrity", err)
	}
	assertFile(t, filepath.Join(opts.InstallRoot, "bin", "stagecore-hub"), "candidate-hub")
}

func TestSnapshotCreateRejectsSymlinkInManagedState(t *testing.T) {
	root := t.TempDir()
	opts := makeSnapshotFixture(t, root)
	if err := os.Symlink(filepath.Join(opts.ConfigRoot, "stagecore.env"), filepath.Join(opts.ConfigRoot, "linked.env")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }

	if _, err := snapshotter.Create(context.Background(), opts); err == nil {
		t.Fatal("Create() succeeded with symlink in managed snapshot state")
	}
}

func TestActiveShowReadsDatabaseWithoutMutatingIt(t *testing.T) {
	dataRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataRoot, "db"), 0o750); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(dataRoot, "db", db.FileName)
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE sessions (session_type TEXT NOT NULL, status TEXT NOT NULL)`); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sessions (session_type, status) VALUES ('SHOW', 'ACTIVE')`); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	active, err := activeShow(context.Background(), dataRoot)
	if err != nil {
		t.Fatalf("activeShow() error = %v", err)
	}
	if !active {
		t.Fatal("activeShow() = false, want true")
	}
}

func makeSnapshotFixture(t *testing.T, root string) SnapshotOptions {
	t.Helper()
	installRoot := filepath.Join(root, "opt", "stagecore")
	configRoot := filepath.Join(root, "etc", "stagecore")
	dataRoot := filepath.Join(root, "var", "lib", "stagecore", "data")
	unit := filepath.Join(root, "etc", "systemd", "system", "stagecore-hub.service")
	backupRoot := filepath.Join(root, "backups")
	mustWrite(t, filepath.Join(installRoot, "bin", "stagecore-hub"), "old-hub", 0o755)
	mustWrite(t, filepath.Join(installRoot, "bin", "stagecore"), "old-cli", 0o755)
	mustWrite(t, filepath.Join(configRoot, "stagecore.env"), "old=config\n", 0o640)
	mustWrite(t, filepath.Join(dataRoot, "db", "state.txt"), "old-state", 0o640)
	mustWrite(t, unit, "old-unit", 0o644)
	return SnapshotOptions{
		BackupRoot: backupRoot, InstallRoot: installRoot, ConfigRoot: configRoot,
		DataRoot: dataRoot, SystemdUnit: unit,
	}
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", path, string(content), want)
	}
}
