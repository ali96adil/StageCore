package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/db/migrations"
	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/doctor"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	stagevault "github.com/ali96adil/StageCore/internal/vault"
	"github.com/pressly/goose/v3"
)

// This integration test runs the real update Service, deployment Installer,
// and Snapshotter against disposable roots. systemd and Doctor remain fakes;
// readiness is served only by an in-process loopback HTTP server.
func TestIsolatedInstalledCopyUpdateAndRollback(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("the deployment installer requires a supported Linux ELF target")
	}
	if os.Geteuid() != 0 {
		t.Skip("isolated installer ownership checks require root; Core CI runs this test explicitly under sudo")
	}

	for _, scenario := range []struct {
		name            string
		unreadyRequests int64
		doctorReports   []doctor.Report
		wantRollback    bool
		wantErr         error
		wantErrorText   string
	}{
		{name: "candidate ready", doctorReports: []doctor.Report{readyReport()}, wantRollback: false},
		{name: "readiness failure restores installed copy", unreadyRequests: 100, doctorReports: []doctor.Report{readyReport()}, wantRollback: true, wantErrorText: "did not become ready"},
		{name: "postflight blocker restores installed copy", doctorReports: []doctor.Report{readyReport(), blockedReport(), readyReport()}, wantRollback: true, wantErr: ErrPostflightBlocked},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			runIsolatedInstalledCopyScenario(t, scenario.unreadyRequests, scenario.doctorReports, scenario.wantRollback, scenario.wantErr, scenario.wantErrorText)
		})
	}
}

// This joins the installed-copy transaction with the real Project grants,
// published Snapshot and Vault data. The candidate restart hook simulates a
// failed candidate changing those records before Doctor rejects postflight;
// the real Snapshotter must restore the installation and SQLite/Vault state.
func TestIsolatedInstalledCopyProjectACLAndVaultRollback(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("the deployment installer requires a supported Linux ELF target")
	}
	if os.Geteuid() != 0 {
		t.Skip("isolated installer ownership checks require root; Core CI runs this test explicitly under sudo")
	}

	ctx := context.Background()
	root := t.TempDir()
	bundleDir := filepath.Join(root, "candidate")
	installRoot := filepath.Join(root, "opt", "stagecore-rc-acl-rollback")
	configRoot := filepath.Join(root, "etc", "stagecore-rc-acl-rollback")
	dataRoot := filepath.Join(root, "var", "lib", "stagecore-rc-acl-rollback", "data")
	vaultRoot := filepath.Join(root, "var", "lib", "stagecore-rc-acl-rollback", "vault")
	serviceName := "stagecore-rc-acl-rollback.service"
	unitPath := filepath.Join(root, "systemd", serviceName)
	backupRoot := filepath.Join(root, "backups", "updates")
	serviceUser, serviceGroup := isolatedTestIdentity(t)
	readiness := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health/ready" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer readiness.Close()
	listenHost, listenPort, err := net.SplitHostPort(readiness.Listener.Addr().String())
	if err != nil {
		t.Fatalf("readiness listener address: %v", err)
	}
	options := deployment.Options{
		BundleDir: bundleDir, InstallRoot: installRoot, ConfigRoot: configRoot,
		DataRoot: dataRoot, VaultRoot: vaultRoot, ServiceUser: serviceUser,
		ServiceGroup: serviceGroup, Listen: net.JoinHostPort(listenHost, listenPort),
		OperatorTLSListen: "127.0.0.1:18742", DeviceListen: "127.0.0.1:18741",
		SystemdUnit: unitPath, ServiceName: serviceName, ReadinessTimeout: time.Second,
	}
	if err := deployment.ValidateIsolatedInstallPaths(options); err != nil {
		t.Fatalf("validate isolated test roots: %v", err)
	}
	if err := deployment.ValidateIsolatedUpdatePaths(options, backupRoot); err != nil {
		t.Fatalf("validate isolated rollback root: %v", err)
	}

	oldConfig := deployment.RenderEnvironment(options) + "# Project ACL rollback fixture\n"
	oldUnit := deployment.RenderSystemdUnit(options) + "# Project ACL rollback fixture\n"
	for _, name := range deployment.RequiredBinaries {
		writeTestFile(t, filepath.Join(installRoot, "bin", name), []byte("old-release:"+name), 0o755)
	}
	writeTestFile(t, filepath.Join(configRoot, "stagecore.env"), []byte(oldConfig), 0o640)
	writeTestFile(t, unitPath, []byte(oldUnit), 0o644)
	if err := os.MkdirAll(vaultRoot, 0o750); err != nil {
		t.Fatal(err)
	}

	baseDB := openUpdateTestSQLite(t, dataRoot)
	migrateUpdateTestSQLiteTo(t, baseDB, 52)
	baseStore := store.New(baseDB, clock.Real{})
	nowUS := time.Now().UTC().UnixMicro()
	for _, user := range []struct{ id, role string }{{"rollback-owner", "OWNER"}, {"rollback-operator", "OPERATOR"}} {
		if _, err := baseDB.ExecContext(ctx, `
			INSERT INTO local_users (user_id, username, password_hash, role, created_at_us, updated_at_us)
			VALUES (?, ?, 'unused-test-hash', ?, ?, ?)`, user.id, user.id, user.role, nowUS, nowUS); err != nil {
			_ = baseDB.Close()
			t.Fatal(err)
		}
	}
	project, revision, err := baseStore.CreateProject(ctx, store.CreateProjectParams{Name: "Installed-copy ACL rollback", CreatedBy: "rollback-owner"})
	if err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	cue, err := baseStore.CreateCueWithActions(ctx, domain.Cue{RevisionID: revision.ID, Name: "Rollback Cue", OrderIndex: 0, Enabled: true}, nil)
	if err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	if err := baseStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	published, _, err := snapshot.NewBuilder(baseStore).Create(ctx, revision.ID, "installed-copy rollback fixture")
	if err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	migrateUpdateTestSQLiteTo(t, baseDB, 53)
	access, err := projectaccess.New(baseDB)
	if err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	permissions := []projectaccess.Permission{
		projectaccess.ProjectView, projectaccess.ShowGO,
		projectaccess.ShowStop, projectaccess.ShowBlackoutEngage,
	}
	if err := access.SetPermissions(ctx, "rollback-owner", "rollback-operator", project.ID, permissions); err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	preview, err := access.PreviewProjectCutover(ctx, "rollback-owner")
	if err != nil || !preview.ReadOnly || preview.Enforced || preview.ReadyForCutover || preview.ProjectCount != 1 {
		_ = baseDB.Close()
		t.Fatalf("baseline ACL cutover preview = %+v, %v", preview, err)
	}
	payload := []byte("original-vault-blob")
	payloadHash := sha256.Sum256(payload)
	objects, err := stagevault.Open(vaultRoot, baseStore)
	if err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	managed, err := objects.ImportVerifiedObject(ctx, hex.EncodeToString(payloadHash[:]), int64(len(payload)), bytes.NewReader(payload))
	if err != nil {
		_ = baseDB.Close()
		t.Fatal(err)
	}
	if err := baseDB.Close(); err != nil {
		t.Fatal(err)
	}
	vaultPath, err := objects.ObjectPath(managed.ContentHash)
	if err != nil {
		t.Fatal(err)
	}

	candidateProjectID := ""
	mutateCandidate := func() error {
		candidate, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
		if err != nil {
			return err
		}
		defer candidate.Close()
		candidateStore := store.New(candidate.DB, clock.Real{})
		version, err := db.SchemaVersion(candidate.DB)
		if err != nil || version <= 53 {
			return fmt.Errorf("candidate did not apply migrations beyond version 53: version=%d err=%v", version, err)
		}
		migratedAccess, err := projectaccess.New(candidate.DB)
		if err != nil {
			return err
		}
		for _, permission := range permissions {
			if err := migratedAccess.Check(ctx, "rollback-operator", project.ID, permission); err != nil {
				return fmt.Errorf("candidate migration lost Project grant %s: %w", permission, err)
			}
		}
		migratedSnapshot, err := candidateStore.GetRuntimeSnapshot(ctx, published.ID)
		if err != nil || migratedSnapshot.ContentHash != published.ContentHash || !bytes.Equal(migratedSnapshot.Manifest, published.Manifest) {
			return fmt.Errorf("candidate migration changed published Snapshot: %+v err=%v", migratedSnapshot, err)
		}
		migratedVault, err := stagevault.Open(vaultRoot, candidateStore)
		if err != nil {
			return err
		}
		migratedFile, migratedObject, err := migratedVault.OpenObject(ctx, managed.ContentHash)
		if err != nil {
			return fmt.Errorf("candidate migration lost Vault object: %w", err)
		}
		migratedPayload, readErr := io.ReadAll(migratedFile)
		closeErr := migratedFile.Close()
		if readErr != nil || closeErr != nil || migratedObject.ContentHash != managed.ContentHash || !bytes.Equal(migratedPayload, payload) {
			return fmt.Errorf("candidate migration changed Vault object: object=%+v payload=%q read=%v close=%v", migratedObject, migratedPayload, readErr, closeErr)
		}
		created, _, err := candidateStore.CreateProject(ctx, store.CreateProjectParams{Name: "Candidate-only Project"})
		if err != nil {
			return err
		}
		candidateProjectID = created.ID
		candidateAccess, err := projectaccess.New(candidate.DB)
		if err != nil {
			return err
		}
		if err := candidateAccess.SetPermissions(ctx, "rollback-owner", "rollback-operator", project.ID, nil); err != nil {
			return err
		}
		if _, err := candidate.DB.ExecContext(ctx, "DELETE FROM runtime_snapshots WHERE runtime_snapshot_id = ?", published.ID); err != nil {
			return err
		}
		return os.WriteFile(vaultPath, []byte("tampered-vault-blob"), 0o600)
	}

	candidate := makeIsolatedELFBundle(t, bundleDir)
	installCommands := &isolatedInstallRunner{
		user: serviceUser, group: serviceGroup, uid: identityUID(t, serviceUser),
		onRestart: mutateCandidate,
	}
	installer := &deployment.Installer{
		GOOS: "linux", GOARCH: runtime.GOARCH, EUID: func() int { return 0 },
		Runner: installCommands, HTTPClient: &http.Client{Timeout: time.Second}, Now: time.Now,
	}
	snapshots := NewSnapshotter()
	serviceCommands := &fakeCommands{}
	service := &Service{
		installer: installer,
		doctor:    &fakeDoctor{reports: []doctor.Report{readyReport(), blockedReport(), readyReport()}},
		snapshots: snapshots,
		commands:  serviceCommands,
		euid:      func() int { return 0 },
		showGuard: func(context.Context, string) (bool, error) { return false, nil },
	}
	dryRun := &Service{
		installer: installer,
		doctor:    &fakeDoctor{reports: []doctor.Report{readyReport()}},
		snapshots: snapshots,
		commands:  &fakeCommands{},
		euid:      func() int { return 0 },
		showGuard: func(context.Context, string) (bool, error) { return false, nil },
	}
	opts := Options{Deployment: options, BackupRoot: backupRoot}
	dryRunOpts := opts
	dryRunOpts.DryRun = true
	dryResult, err := dryRun.Apply(ctx, dryRunOpts)
	if err != nil || !dryResult.DryRun || dryResult.BackupPath != "" || candidateProjectID != "" || installCommands.restartCount != 0 {
		t.Fatalf("installed-copy dry-run changed candidate state: result=%+v candidate=%q restarts=%d err=%v", dryResult, candidateProjectID, installCommands.restartCount, err)
	}
	assertTestFile(t, filepath.Join(installRoot, "bin", "stagecore-hub"), []byte("old-release:stagecore-hub"))

	result, applyErr := service.Apply(ctx, opts)
	if !errors.Is(applyErr, ErrPostflightBlocked) || errors.Is(applyErr, ErrRollbackFailed) ||
		!result.RolledBack || result.RollbackReport.Overall != doctor.OverallReady || candidateProjectID == "" {
		t.Fatalf("expected verified installed-copy rollback after candidate damaged ACL/Snapshot/Vault: result=%+v err=%v", result, applyErr)
	}
	if installCommands.restartCount != 1 || serviceCommands.calls[len(serviceCommands.calls)-1] != "systemctl start "+serviceName {
		t.Fatalf("isolated candidate/rollback start trace: installer restarts=%d service calls=%v", installCommands.restartCount, serviceCommands.calls)
	}
	assertTestFile(t, filepath.Join(installRoot, "bin", "stagecore-hub"), []byte("old-release:stagecore-hub"))
	assertTestFile(t, filepath.Join(configRoot, "stagecore.env"), []byte(oldConfig))
	if bytes.Equal([]byte(readTestFile(t, filepath.Join(installRoot, "bin", "stagecore-hub"))), candidate) {
		t.Fatal("candidate binary remained installed after rollback")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("verified rollback snapshot missing: %v", err)
	}

	restoredDB := openUpdateTestSQLite(t, dataRoot)
	defer restoredDB.Close()
	version, err := db.SchemaVersion(restoredDB)
	if err != nil || version != 53 {
		t.Fatalf("rollback did not restore the pre-candidate schema version: got=%d err=%v", version, err)
	}
	var integrity string
	if err := restoredDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("restored database integrity: %q, %v", integrity, err)
	}
	rows, err := restoredDB.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		_ = rows.Close()
		t.Fatal("restored installed-copy database has foreign-key violations")
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	restoredStore := store.New(restoredDB, clock.Real{})
	if _, err := restoredStore.GetProject(ctx, project.ID); err != nil {
		t.Fatalf("baseline project missing after installed-copy rollback: %v", err)
	}
	if _, err := restoredStore.GetProject(ctx, candidateProjectID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("candidate Project survived installed-copy rollback: %v", err)
	}
	restoredAccess, err := projectaccess.New(restoredDB)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range permissions {
		if err := restoredAccess.Check(ctx, "rollback-operator", project.ID, permission); err != nil {
			t.Fatalf("rollback did not restore Project grant %s: %v", permission, err)
		}
	}
	postRollbackPreview, err := restoredAccess.PreviewProjectCutover(ctx, "rollback-owner")
	if err != nil || !postRollbackPreview.ReadOnly || postRollbackPreview.Enforced || postRollbackPreview.ReadyForCutover || postRollbackPreview.ProjectCount != 1 {
		t.Fatalf("post-rollback cutover preview = %+v, %v", postRollbackPreview, err)
	}
	restoredSnapshot, err := restoredStore.GetRuntimeSnapshot(ctx, published.ID)
	if err != nil || restoredSnapshot.ContentHash != published.ContentHash || !bytes.Equal(restoredSnapshot.Manifest, published.Manifest) || restoredSnapshot.Status != domain.SnapshotPublished {
		t.Fatalf("published Snapshot changed after installed-copy rollback: %+v err=%v", restoredSnapshot, err)
	}
	manifest, err := snapshot.Decode(restoredSnapshot.Manifest)
	if err != nil || len(manifest.Cues) != 1 || manifest.Cues[0].ID != cue.ID {
		t.Fatalf("installed-copy rollback lost the published Cue: %+v err=%v", manifest, err)
	}
	restoredVault, err := stagevault.Open(vaultRoot, restoredStore)
	if err != nil {
		t.Fatal(err)
	}
	verifiedFile, verifiedObject, err := restoredVault.OpenObject(ctx, managed.ContentHash)
	if err != nil {
		t.Fatalf("restored Vault object unavailable: %v", err)
	}
	defer verifiedFile.Close()
	gotPayload, err := io.ReadAll(verifiedFile)
	if err != nil || verifiedObject.ContentHash != managed.ContentHash || !bytes.Equal(gotPayload, payload) {
		t.Fatalf("restored Vault bytes/metadata changed: object=%+v bytes=%q err=%v", verifiedObject, gotPayload, err)
	}
	if err := restoredDB.Close(); err != nil {
		t.Fatal(err)
	}

	// Opening the restored version-53 installation with the current schema code
	// must migrate it forward without changing the recovered authorization and
	// immutable show data.
	reopened, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if version, err := db.SchemaVersion(reopened.DB); err != nil || version <= 53 {
		t.Fatalf("restored installation did not migrate forward: version=%d err=%v", version, err)
	}
	reopenedStore := store.New(reopened.DB, clock.Real{})
	reopenedAccess, err := projectaccess.New(reopened.DB)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range permissions {
		if err := reopenedAccess.Check(ctx, "rollback-operator", project.ID, permission); err != nil {
			t.Fatalf("forward migration lost restored Project grant %s: %v", permission, err)
		}
	}
	reopenedSnapshot, err := reopenedStore.GetRuntimeSnapshot(ctx, published.ID)
	if err != nil || reopenedSnapshot.ContentHash != published.ContentHash || !bytes.Equal(reopenedSnapshot.Manifest, published.Manifest) {
		t.Fatalf("forward migration changed restored Snapshot: %+v err=%v", reopenedSnapshot, err)
	}
}

func runIsolatedInstalledCopyScenario(t *testing.T, unreadyRequests int64, reports []doctor.Report, wantRollback bool, wantErr error, wantErrorText string) {
	t.Helper()

	var readinessRequests atomic.Int64
	readiness := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health/ready" {
			http.NotFound(w, r)
			return
		}
		request := readinessRequests.Add(1)
		if request <= unreadyRequests {
			http.Error(w, "qualification candidate is not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer readiness.Close()

	root := t.TempDir()
	bundleDir := filepath.Join(root, "release")
	installRoot := filepath.Join(root, "opt", "stagecore-rc-update-test")
	configRoot := filepath.Join(root, "etc", "stagecore-rc-update-test")
	dataRoot := filepath.Join(root, "var", "lib", "stagecore-rc-update-test", "data")
	vaultRoot := filepath.Join(root, "var", "lib", "stagecore-rc-update-test", "vault")
	serviceName := "stagecore-rc-update-test.service"
	unitPath := filepath.Join(root, "systemd", serviceName)
	backupRoot := filepath.Join(root, "backups", "updates")
	serviceUser, serviceGroup := isolatedTestIdentity(t)
	listenHost, listenPort, err := net.SplitHostPort(readiness.Listener.Addr().String())
	if err != nil {
		t.Fatalf("readiness listener address: %v", err)
	}
	listen := net.JoinHostPort(listenHost, listenPort)

	options := deployment.Options{
		BundleDir:         bundleDir,
		InstallRoot:       installRoot,
		ConfigRoot:        configRoot,
		DataRoot:          dataRoot,
		VaultRoot:         vaultRoot,
		ServiceUser:       serviceUser,
		ServiceGroup:      serviceGroup,
		Listen:            listen,
		OperatorTLSListen: "127.0.0.1:18742",
		DeviceListen:      "127.0.0.1:18741",
		SystemdUnit:       unitPath,
		ServiceName:       serviceName,
		ReadinessTimeout:  40 * time.Millisecond,
	}
	if err := deployment.ValidateIsolatedInstallPaths(options); err != nil {
		t.Fatalf("validate isolated test roots: %v", err)
	}
	if err := deployment.ValidateIsolatedUpdatePaths(options, backupRoot); err != nil {
		t.Fatalf("validate isolated rollback root: %v", err)
	}

	oldConfig := deployment.RenderEnvironment(options) + "# qualification preserves this comment\n"
	oldUnit := deployment.RenderSystemdUnit(options) + "# installed-copy marker\n"
	oldData := []byte("temporary isolated database marker")
	oldVault := []byte("temporary isolated vault marker")
	for _, name := range deployment.RequiredBinaries {
		writeTestFile(t, filepath.Join(installRoot, "bin", name), []byte("old-release:"+name), 0o755)
	}
	writeTestFile(t, filepath.Join(configRoot, "stagecore.env"), []byte(oldConfig), 0o640)
	writeTestFile(t, filepath.Join(dataRoot, "qualification.db"), oldData, 0o600)
	writeTestFile(t, filepath.Join(vaultRoot, "qualification-object"), oldVault, 0o600)
	writeTestFile(t, unitPath, []byte(oldUnit), 0o644)
	candidate := makeIsolatedELFBundle(t, bundleDir)

	installCommands := &isolatedInstallRunner{
		user: serviceUser, group: serviceGroup,
		uid: identityUID(t, serviceUser),
	}
	diagnostics := &fakeDoctor{reports: reports}
	serviceCommands := &fakeCommands{}
	installer := &deployment.Installer{
		GOOS:       "linux",
		GOARCH:     runtime.GOARCH,
		EUID:       func() int { return 0 },
		Runner:     installCommands,
		HTTPClient: &http.Client{Timeout: time.Second},
		Now:        time.Now,
	}
	service := &Service{
		installer: installer,
		doctor:    diagnostics,
		snapshots: NewSnapshotter(),
		commands:  serviceCommands,
		euid:      func() int { return 0 },
		showGuard: func(context.Context, string) (bool, error) { return false, nil },
	}

	result, applyErr := service.Apply(context.Background(), Options{Deployment: options, BackupRoot: backupRoot})
	if wantErr != nil && !errors.Is(applyErr, wantErr) {
		t.Fatalf("Apply() error = %v, want %v", applyErr, wantErr)
	}
	if wantErr == nil && !wantRollback && applyErr != nil {
		t.Fatalf("Apply() error = %v", applyErr)
	}
	if wantErrorText != "" && (applyErr == nil || !strings.Contains(applyErr.Error(), wantErrorText)) {
		t.Fatalf("Apply() error = %v, want text %q", applyErr, wantErrorText)
	}
	if wantRollback && applyErr == nil {
		t.Fatal("Apply() succeeded, want an automatic rollback")
	}
	if result.RolledBack != wantRollback {
		t.Fatalf("RolledBack = %v, want %v (error %v)", result.RolledBack, wantRollback, applyErr)
	}
	if wantRollback && errors.Is(applyErr, ErrRollbackFailed) {
		t.Fatalf("automatic rollback unexpectedly failed: %v", applyErr)
	}
	if result.BackupPath == "" {
		t.Fatal("successful transaction did not retain its rollback snapshot path")
	}
	if readinessRequests.Load() == 0 {
		t.Fatal("candidate installation did not probe the loopback readiness server")
	}
	if result.Effective.ServiceName != serviceName || result.Effective.InstallRoot != installRoot || result.Effective.DataRoot != dataRoot || result.Effective.VaultRoot != vaultRoot {
		t.Fatalf("update escaped isolated deployment identity or roots: %+v", result.Effective)
	}

	if wantRollback {
		for _, name := range deployment.RequiredBinaries {
			assertTestFile(t, filepath.Join(installRoot, "bin", name), []byte("old-release:"+name))
		}
		assertTestFile(t, filepath.Join(configRoot, "stagecore.env"), []byte(oldConfig))
		assertTestFile(t, filepath.Join(dataRoot, "qualification.db"), oldData)
		assertTestFile(t, filepath.Join(vaultRoot, "qualification-object"), oldVault)
		assertTestFile(t, unitPath, []byte(oldUnit))
		if result.RollbackReport.Overall != doctor.OverallReady {
			t.Fatalf("restored-copy Doctor result = %s, want READY", result.RollbackReport.Overall)
		}
		if diagnostics.calls != 2 && wantErr != ErrPostflightBlocked {
			t.Fatalf("Doctor calls after candidate-readiness rollback = %d, want preflight and restored-install checks", diagnostics.calls)
		}
	} else {
		for _, name := range deployment.RequiredBinaries {
			assertTestFile(t, filepath.Join(installRoot, "bin", name), candidate)
		}
		assertTestFile(t, filepath.Join(configRoot, "stagecore.env"), []byte(oldConfig))
		assertTestFile(t, filepath.Join(dataRoot, "qualification.db"), oldData)
		assertTestFile(t, filepath.Join(vaultRoot, "qualification-object"), oldVault)
		if strings.Contains(readTestFile(t, unitPath), "installed-copy marker") {
			t.Fatal("successful candidate retained the old systemd unit")
		}
		if diagnostics.calls != 2 {
			t.Fatalf("Doctor calls = %d, want preflight and postflight", diagnostics.calls)
		}
	}

	commandCalls := strings.Join(serviceCommands.calls, "|")
	wantCommands := []string{"systemctl stop " + serviceName}
	if wantRollback {
		wantCommands = append(wantCommands, "systemctl stop "+serviceName, "systemctl daemon-reload", "systemctl start "+serviceName)
	}
	if commandCalls != strings.Join(wantCommands, "|") {
		t.Fatalf("update service command order = %v, want %v", serviceCommands.calls, wantCommands)
	}
	if !strings.Contains(commandCalls, serviceName) || strings.Contains(commandCalls, deployment.DefaultServiceName) || strings.Contains(commandCalls, "stagecore-camera-relay.service") || strings.Contains(commandCalls, DefaultBackupRoot) {
		t.Fatalf("update service commands touched unexpected targets: %v", serviceCommands.calls)
	}
	for _, call := range installCommands.calls {
		if strings.HasPrefix(call, "systemctl ") && !strings.Contains(call, serviceName) && !strings.Contains(call, "daemon-reload") {
			t.Fatalf("installer systemd command escaped isolated service: %s", call)
		}
	}
	if !strings.Contains(readTestFile(t, filepath.Join(result.BackupPath, snapshotManifestName)), `"vault-root"`) {
		t.Fatal("isolated snapshot omitted the Vault root from its manifest")
	}
}

func isolatedTestIdentity(t *testing.T) (string, string) {
	t.Helper()
	account, err := user.Current()
	if err != nil {
		t.Fatalf("current test account: %v", err)
	}
	if account.Uid == "0" {
		account, err = user.Lookup("nobody")
		if err != nil {
			t.Fatalf("lookup unprivileged qualification account: %v", err)
		}
	}
	group, err := user.LookupGroupId(account.Gid)
	if err != nil {
		t.Fatalf("lookup qualification account primary group: %v", err)
	}
	if account.Username == "root" || group.Name == "root" {
		t.Fatal("isolated update test identity must not be root")
	}
	return account.Username, group.Name
}

func identityUID(t *testing.T, name string) string {
	t.Helper()
	account, err := user.Lookup(name)
	if err != nil {
		t.Fatalf("lookup qualification service user: %v", err)
	}
	return account.Uid
}

type isolatedInstallRunner struct {
	user         string
	group        string
	uid          string
	calls        []string
	restartCount int
	onRestart    func() error
}

func (r *isolatedInstallRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.calls = append(r.calls, call)
	switch {
	case name == "getent" && len(args) == 2 && args[0] == "group" && args[1] == r.group:
		return []byte(r.group), nil
	case name == "id" && len(args) == 2 && args[0] == "-u" && args[1] == r.user:
		return []byte(r.uid), nil
	case name == "id" && len(args) == 2 && args[0] == "-gn" && args[1] == r.user:
		return []byte(r.group), nil
	case name == "systemctl" && len(args) == 1 && args[0] == "daemon-reload":
		return nil, nil
	case name == "systemctl" && len(args) == 2 && args[0] == "restart" && strings.HasPrefix(args[1], "stagecore-rc-"):
		r.restartCount++
		if r.onRestart != nil {
			if err := r.onRestart(); err != nil {
				return nil, err
			}
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected isolated installer command: %s", call)
	}
}

func makeIsolatedELFBundle(t *testing.T, bundleDir string) []byte {
	t.Helper()
	elfPath := "/bin/true"
	if _, err := os.Stat(elfPath); err != nil {
		elfPath = "/usr/bin/true"
	}
	candidate, err := os.ReadFile(elfPath)
	if err != nil {
		t.Fatalf("read Linux ELF fixture: %v", err)
	}
	var checksums strings.Builder
	for _, name := range deployment.RequiredBinaries {
		writeTestFile(t, filepath.Join(bundleDir, name), candidate, 0o755)
		digest := sha256.Sum256(candidate)
		fmt.Fprintf(&checksums, "%s  %s\n", hex.EncodeToString(digest[:]), name)
	}
	writeTestFile(t, filepath.Join(bundleDir, "SHA256SUMS"), []byte(checksums.String()), 0o644)
	return candidate
}

func writeTestFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create temporary test directory: %v", err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatalf("write temporary test file: %v", err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temporary test file: %v", err)
	}
	return string(content)
}

func openUpdateTestSQLite(t *testing.T, dataRoot string) *sql.DB {
	t.Helper()
	path := filepath.Join(dataRoot, "db", db.FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	query := url.Values{}
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "WAL")
	query.Set("_synchronous", "FULL")
	query.Set("_busy_timeout", "5000")
	query.Set("_defensive", "1")
	query.Set("_dqs", "0")
	query.Add("_pragma", "trusted_schema(OFF)")
	query.Set("_txlock", "immediate")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: query.Encode()}).String()
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open isolated migration database: %v", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Ping(); err != nil {
		t.Fatalf("ping isolated migration database: %v", err)
	}
	return database
}

func migrateUpdateTestSQLiteTo(t *testing.T, database *sql.DB, version int64) {
	t.Helper()
	goose.SetBaseFS(migrations.FS)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set isolated SQLite migration dialect: %v", err)
	}
	if err := goose.UpTo(database, ".", version); err != nil {
		t.Fatalf("migrate isolated SQLite database to version %d: %v", version, err)
	}
}

func assertTestFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installed-copy path %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("installed-copy path %s does not match expected bytes", path)
	}
}
