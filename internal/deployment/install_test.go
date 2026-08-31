package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDefaultOptionsMatchReferenceDeployment(t *testing.T) {
	opts := DefaultOptions("/tmp/release")
	if opts.InstallRoot != "/opt/stagecore" || opts.ConfigRoot != "/etc/stagecore" {
		t.Fatalf("unexpected install/config roots: %+v", opts)
	}
	if opts.DataRoot != "/var/lib/stagecore/data" || opts.VaultRoot != "/var/lib/stagecore/vault" {
		t.Fatalf("unexpected data/vault roots: %+v", opts)
	}
	if opts.ServiceUser != "stagecore" || opts.ServiceGroup != "stagecore" || opts.Listen != "127.0.0.1:7840" {
		t.Fatalf("unexpected service defaults: %+v", opts)
	}
}

func TestNormalizeOptionsRejectsUnsafeOrUnsupportedValues(t *testing.T) {
	base := DefaultOptions("/tmp/release")
	cases := []struct {
		name string
		edit func(*Options)
		want string
	}{
		{"relative bundle", func(o *Options) { o.BundleDir = "release" }, "bundle directory must be an absolute path"},
		{"same data and vault", func(o *Options) { o.VaultRoot = o.DataRoot }, "must be distinct"},
		{"bad listen", func(o *Options) { o.Listen = "localhost" }, "invalid listen address"},
		{"bad user", func(o *Options) { o.ServiceUser = "stage core" }, "invalid service user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := base
			tc.edit(&opts)
			_, err := normalizeOptions(opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, want substring %q", err, tc.want)
			}
		})
	}
	if err := validatePlatform("darwin", "arm64"); err == nil {
		t.Fatal("darwin unexpectedly accepted")
	}
	if err := validatePlatform("linux", "386"); err == nil {
		t.Fatal("386 unexpectedly accepted")
	}
}

func TestValidateBundleAcceptsNativeELFAndRejectsCorruption(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64") {
		t.Skip("bundle ELF contract test requires supported Linux CI architecture")
	}
	bundle := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	var sums strings.Builder
	for _, name := range RequiredBinaries {
		path := filepath.Join(bundle, name)
		if err := os.WriteFile(path, bytes, 0o755); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(bytes)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(hash[:]), name)
	}
	if err := os.WriteFile(filepath.Join(bundle, checksumFileName), []byte(sums.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBundle(bundle, runtime.GOARCH); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}

	if err := os.WriteFile(filepath.Join(bundle, "stagecore-hub"), append(bytes, 'x'), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBundle(bundle, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "checksum mismatch for stagecore-hub") {
		t.Fatalf("corrupt bundle err=%v", err)
	}
}

func TestValidateBundleRejectsSymlinkedArtifact(t *testing.T) {
	bundle := t.TempDir()
	if err := os.WriteFile(filepath.Join(bundle, checksumFileName), []byte(strings.Repeat("0", 64)+"  stagecore-hub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/true", filepath.Join(bundle, "stagecore-hub")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBundle(bundle, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("symlink bundle err=%v", err)
	}
}

func TestExistingConfigurationIsAdoptedWithoutReplacement(t *testing.T) {
	opts := DefaultOptions("/tmp/release")
	content := []byte(`# existing qualified deployment
STAGECORE_DATA_ROOT=/srv/stagecore/data
STAGECORE_VAULT_ROOT=/mnt/media/stagecore-vault
STAGECORE_LISTEN=10.20.30.40:7840
STAGECORE_OSC_PLUGIN_PATH=/opt/stagecore/bin/stagecore-osc-plugin
`)
	adopted, err := adoptExistingConfig(opts, content)
	if err != nil {
		t.Fatal(err)
	}
	if adopted.DataRoot != "/srv/stagecore/data" || adopted.VaultRoot != "/mnt/media/stagecore-vault" || adopted.Listen != "10.20.30.40:7840" {
		t.Fatalf("existing config not adopted: %+v", adopted)
	}
}

func TestExistingConfigurationMustContainCriticalDeploymentValues(t *testing.T) {
	opts := DefaultOptions("/tmp/release")
	_, err := adoptExistingConfig(opts, []byte("STAGECORE_DATA_ROOT=/srv/data\n"))
	if err == nil || !strings.Contains(err.Error(), "STAGECORE_VAULT_ROOT") {
		t.Fatalf("err=%v", err)
	}
}

func TestRenderEnvironmentAndSystemdUnit(t *testing.T) {
	opts := DefaultOptions("/tmp/release")
	env := RenderEnvironment(opts)
	for _, marker := range []string{
		"STAGECORE_DATA_ROOT=/var/lib/stagecore/data",
		"STAGECORE_VAULT_ROOT=/var/lib/stagecore/vault",
		"STAGECORE_LISTEN=127.0.0.1:7840",
		"STAGECORE_OSC_PLUGIN_PATH=/opt/stagecore/bin/stagecore-osc-plugin",
	} {
		if !strings.Contains(env, marker) {
			t.Fatalf("managed environment missing %q", marker)
		}
	}
	unit := RenderSystemdUnit(opts)
	for _, marker := range []string{
		"After=local-fs.target network.target",
		"Wants=network.target",
		"RequiresMountsFor=/var/lib/stagecore/data /var/lib/stagecore/vault",
		"User=stagecore",
		"EnvironmentFile=/etc/stagecore/stagecore.env",
		"ExecStart=/opt/stagecore/bin/stagecore-hub",
		"Restart=on-failure",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"ProtectHome=true",
		"PrivateTmp=true",
		"ReadWritePaths=/var/lib/stagecore/data /var/lib/stagecore/vault",
	} {
		if !strings.Contains(unit, marker) {
			t.Fatalf("systemd unit missing %q", marker)
		}
	}
	if strings.Contains(unit, "network-online.target") {
		t.Fatal("systemd unit must not depend on network-online.target")
	}
}

func TestReadinessURLUsesLoopbackForWildcardListen(t *testing.T) {
	cases := map[string]string{
		"0.0.0.0:7840": "http://127.0.0.1:7840/health/ready",
		":7840":        "http://127.0.0.1:7840/health/ready",
		"[::]:7840":    "http://[::1]:7840/health/ready",
		"10.0.0.2:7840": "http://10.0.0.2:7840/health/ready",
	}
	for listen, want := range cases {
		got, err := ReadinessURL(listen)
		if err != nil {
			t.Fatalf("%s: %v", listen, err)
		}
		if got != want {
			t.Fatalf("%s => %s, want %s", listen, got, want)
		}
	}
}

func TestDryRunValidatesBundleWithoutRootOrSystemMutation(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64") {
		t.Skip("dry-run bundle test requires supported Linux CI architecture")
	}
	bundle := makeNativeTestBundle(t)
	opts := DefaultOptions(bundle)
	opts.ConfigRoot = filepath.Join(t.TempDir(), "etc-stagecore")
	opts.InstallRoot = "/opt/stagecore"
	opts.DataRoot = "/var/lib/stagecore/data"
	opts.VaultRoot = "/var/lib/stagecore/vault"
	opts.DryRun = true
	installer := NewInstaller()
	installer.EUID = func() int { return 1000 }
	installer.Runner = panicRunner{t: t}
	result, err := installer.Install(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) < 7 {
		t.Fatalf("dry-run plan too small: %v", result.Actions)
	}
	if _, err := os.Stat(filepath.Join(opts.InstallRoot, "bin", "stagecore-hub")); err == nil {
		t.Fatal("dry run unexpectedly wrote installation artifact")
	}
}

type panicRunner struct{ t *testing.T }

func (p panicRunner) Output(context.Context, string, ...string) ([]byte, error) {
	p.t.Fatal("dry-run invoked system command")
	return nil, nil
}

func makeNativeTestBundle(t *testing.T) string {
	t.Helper()
	bundle := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	var sums strings.Builder
	for _, name := range RequiredBinaries {
		if err := os.WriteFile(filepath.Join(bundle, name), bytes, 0o755); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(bytes)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(hash[:]), name)
	}
	if err := os.WriteFile(filepath.Join(bundle, checksumFileName), []byte(sums.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestReadinessTimeoutDefaultIsBounded(t *testing.T) {
	if got := DefaultOptions("/tmp/release").ReadinessTimeout; got != 20*time.Second {
		t.Fatalf("readiness timeout=%s", got)
	}
}
