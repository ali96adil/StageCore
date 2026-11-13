package update

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/deployment"
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

	mustWrite(t, filepath.Join(opts.Deployment.InstallRoot, "bin", "stagecore-hub"), "candidate-hub", 0o755)
	mustWrite(t, filepath.Join(opts.Deployment.ConfigRoot, "stagecore.env"), "candidate=config\n", 0o640)
	mustWrite(t, filepath.Join(opts.Deployment.DataRoot, "db", "state.txt"), "candidate-state", 0o640)
	mustWrite(t, opts.Deployment.SystemdUnit, "candidate-unit", 0o644)

	if err := snapshotter.Restore(context.Background(), snapshot); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	assertFile(t, filepath.Join(opts.Deployment.InstallRoot, "bin", "stagecore-hub"), "old-hub")
	assertFile(t, filepath.Join(opts.Deployment.ConfigRoot, "stagecore.env"), "old=config\n")
	assertFile(t, filepath.Join(opts.Deployment.DataRoot, "db", "state.txt"), "old-state")
	assertFile(t, opts.Deployment.SystemdUnit, "old-unit")
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
	mustWrite(t, filepath.Join(opts.Deployment.InstallRoot, "bin", "stagecore-hub"), "candidate-hub", 0o755)
	mustWrite(t, filepath.Join(snapshot.Path, "managed-binaries", "stagecore-hub"), "tampered", 0o755)

	err = snapshotter.Restore(context.Background(), snapshot)
	if !errors.Is(err, ErrSnapshotIntegrity) {
		t.Fatalf("Restore() error = %v, want ErrSnapshotIntegrity", err)
	}
	assertFile(t, filepath.Join(opts.Deployment.InstallRoot, "bin", "stagecore-hub"), "candidate-hub")
}

func TestSnapshotRestoreRejectsManifestRetargetingBeforeMutation(t *testing.T) {
	root := t.TempDir()
	opts := makeSnapshotFixture(t, root)
	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }
	snapshot, err := snapshotter.Create(context.Background(), opts)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	mustWrite(t, filepath.Join(opts.Deployment.InstallRoot, "bin", "stagecore-hub"), "candidate-hub", 0o755)

	manifestPath := filepath.Join(snapshot.Path, snapshotManifestName)
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var tampered SnapshotManifest
	if err := json.Unmarshal(content, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered.Items[0].Source = "/var/lib/stagecore/data"
	content, err = json.MarshalIndent(tampered, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(content, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	err = snapshotter.Restore(context.Background(), snapshot)
	if !errors.Is(err, ErrSnapshotIntegrity) {
		t.Fatalf("Restore() error = %v, want manifest target rejection", err)
	}
	assertFile(t, filepath.Join(opts.Deployment.InstallRoot, "bin", "stagecore-hub"), "candidate-hub")
}

func TestIsolatedSnapshotRestoresVaultAndServiceSpecificRoots(t *testing.T) {
	root := t.TempDir()
	opts := makeSnapshotFixture(t, root)
	deployed := opts.Deployment
	deployed.BundleDir = filepath.Join(root, "candidate")
	deployed.InstallRoot = filepath.Join(root, "opt", "stagecore-rc-test")
	deployed.ConfigRoot = filepath.Join(root, "etc", "stagecore-rc-test")
	deployed.DataRoot = filepath.Join(root, "var", "lib", "stagecore-rc-test", "data")
	deployed.VaultRoot = filepath.Join(root, "var", "lib", "stagecore-rc-test", "vault")
	deployed.SystemdUnit = filepath.Join(root, "systemd", "stagecore-rc-test.service")
	deployed.ServiceName = "stagecore-rc-test.service"
	deployed.Listen = "127.0.0.1:17840"
	deployed.OperatorTLSListen = "127.0.0.1:17842"
	deployed.DeviceListen = "127.0.0.1:17841"
	if err := os.MkdirAll(deployed.VaultRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(deployed.VaultRoot, "media", "scene.bin")
	mustWrite(t, media, "old-media", 0o640)
	unitPath := deployed.SystemdUnit
	mustWrite(t, unitPath, "old-isolated-unit", 0o644)
	mustWrite(t, filepath.Join(deployed.InstallRoot, "bin", "stagecore-hub"), "old-hub", 0o755)
	mustWrite(t, filepath.Join(deployed.ConfigRoot, "stagecore.env"), "old=config\n", 0o640)
	mustWrite(t, filepath.Join(deployed.DataRoot, "db", "state.txt"), "old-state", 0o640)
	opts.Deployment = deployed

	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }
	snapshot, err := snapshotter.Create(context.Background(), opts)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(snapshot.Manifest.Items) != 5 || snapshot.Manifest.TargetService != deployed.ServiceName {
		t.Fatalf("isolated snapshot did not include its Vault and unit identity: %+v", snapshot.Manifest)
	}
	mustWrite(t, media, "candidate-media", 0o640)
	mustWrite(t, filepath.Join(deployed.DataRoot, "db", "state.txt"), "candidate-state", 0o640)
	mustWrite(t, filepath.Join(deployed.InstallRoot, "bin", "stagecore-hub"), "candidate-hub", 0o755)
	mustWrite(t, filepath.Join(deployed.ConfigRoot, "stagecore.env"), "candidate=config", 0o640)
	mustWrite(t, unitPath, "candidate-unit", 0o644)

	if err := snapshotter.Restore(context.Background(), snapshot); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	assertFile(t, media, "old-media")
	assertFile(t, filepath.Join(deployed.DataRoot, "db", "state.txt"), "old-state")
	assertFile(t, filepath.Join(deployed.InstallRoot, "bin", "stagecore-hub"), "old-hub")
	assertFile(t, filepath.Join(deployed.ConfigRoot, "stagecore.env"), "old=config\n")
	assertFile(t, unitPath, "old-isolated-unit")
}

func TestSnapshotCreateRejectsSymlinkInManagedState(t *testing.T) {
	root := t.TempDir()
	opts := makeSnapshotFixture(t, root)
	if err := os.Symlink(filepath.Join(opts.Deployment.ConfigRoot, "stagecore.env"), filepath.Join(opts.Deployment.ConfigRoot, "linked.env")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }

	if _, err := snapshotter.Create(context.Background(), opts); err == nil {
		t.Fatal("Create() succeeded with symlink in managed snapshot state")
	}
}

func TestSnapshotRejectsBackupRootAliasedIntoDataRoot(t *testing.T) {
	root := t.TempDir()
	opts := makeSnapshotFixture(t, root)
	alias := filepath.Join(root, "data-alias")
	if err := os.Symlink(opts.Deployment.DataRoot, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	opts.BackupRoot = filepath.Join(alias, "rollback")
	snapshotter := NewSnapshotter()
	snapshotter.EUID = func() int { return 1000 }
	if _, err := snapshotter.Create(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "must not overlap a deployment path") {
		t.Fatalf("backup root aliased into Data Root error = %v", err)
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

func TestActiveRehearsalAlsoBlocksUpdate(t *testing.T) {
	dataRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataRoot, "db"), 0o750); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", filepath.Join(dataRoot, "db", db.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE sessions (session_type TEXT NOT NULL, status TEXT NOT NULL)`); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO sessions (session_type, status) VALUES ('REHEARSAL', 'ACTIVE')`); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	active, err := activeShow(context.Background(), dataRoot)
	if err != nil || !active {
		t.Fatalf("activeShow(REHEARSAL) = %v, %v; want true", active, err)
	}
}

func makeSnapshotFixture(t *testing.T, root string) SnapshotOptions {
	t.Helper()
	installRoot := filepath.Join(root, "opt", "stagecore")
	configRoot := filepath.Join(root, "etc", "stagecore")
	dataRoot := filepath.Join(root, "var", "lib", "stagecore", "data")
	vaultRoot := filepath.Join(root, "var", "lib", "stagecore", "vault")
	unit := filepath.Join(root, "etc", "systemd", "system", "stagecore-hub.service")
	backupRoot := filepath.Join(root, "backups")
	mustWrite(t, filepath.Join(installRoot, "bin", "stagecore-hub"), "old-hub", 0o755)
	mustWrite(t, filepath.Join(installRoot, "bin", "stagecore"), "old-cli", 0o755)
	mustWrite(t, filepath.Join(configRoot, "stagecore.env"), "old=config\n", 0o640)
	mustWrite(t, filepath.Join(dataRoot, "db", "state.txt"), "old-state", 0o640)
	if err := os.MkdirAll(vaultRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, unit, "old-unit", 0o644)
	return SnapshotOptions{
		BackupRoot: backupRoot,
		Deployment: deployment.Options{
			BundleDir: filepath.Join(root, "candidate"), InstallRoot: installRoot,
			ConfigRoot: configRoot, DataRoot: dataRoot, VaultRoot: vaultRoot,
			SystemdUnit: unit, ServiceUser: deployment.DefaultServiceUser, ServiceGroup: deployment.DefaultServiceGroup,
		},
		InstallRoot: installRoot, ConfigRoot: configRoot, DataRoot: dataRoot,
		VaultRoot: vaultRoot, SystemdUnit: unit,
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
