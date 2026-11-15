package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/deployment"
	"github.com/ali96adil/StageCore/internal/doctor"
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
	user  string
	group string
	uid   string
	calls []string
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
