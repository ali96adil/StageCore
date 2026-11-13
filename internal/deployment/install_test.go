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

	"github.com/ali96adil/StageCore/internal/config"
)

func TestDefaultOptionsMatchReferenceDeployment(t *testing.T) {
	opts := DefaultOptions("/tmp/release")
	if opts.InstallRoot != "/opt/stagecore" || opts.ConfigRoot != "/etc/stagecore" {
		t.Fatalf("unexpected install/config roots: %+v", opts)
	}
	if opts.DataRoot != "/var/lib/stagecore/data" || opts.VaultRoot != "/var/lib/stagecore/vault" {
		t.Fatalf("unexpected data/vault roots: %+v", opts)
	}
	if opts.ServiceUser != "stagecore" || opts.ServiceGroup != "stagecore" || opts.Listen != "127.0.0.1:7840" || opts.OperatorTLSListen != "0.0.0.0:7842" {
		t.Fatalf("unexpected service defaults: %+v", opts)
	}
	if name, err := SystemdServiceName(opts.SystemdUnit); err != nil || name != DefaultServiceName {
		t.Fatalf("default service identity = %q, %v", name, err)
	}
}

func TestSystemdServiceNameRejectsShellSyntaxAndTraversal(t *testing.T) {
	for _, path := range []string{
		"relative/stagecore-rc.service",
		"/etc/systemd/system/../stagecore-hub.service",
		"/etc/systemd/system/stagecore-rc.service;id",
		"/etc/systemd/system/-stagecore-rc.service",
		"/etc/systemd/system/stagecore-hub.socket",
	} {
		t.Run(path, func(t *testing.T) {
			if name, err := SystemdServiceName(path); err == nil {
				t.Fatalf("unsafe unit path resolved to %q", name)
			}
		})
	}
}

func TestNormalizeOptionsDerivesAndChecksServiceIdentity(t *testing.T) {
	opts := DefaultOptions("/tmp/release")
	opts.SystemdUnit = "/etc/systemd/system/stagecore-rc-qualification.service"
	normalized, err := normalizeOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ServiceName != "stagecore-rc-qualification.service" {
		t.Fatalf("derived service name = %q", normalized.ServiceName)
	}

	opts.ServiceName = DefaultServiceName
	if _, err := normalizeOptions(opts); err == nil || !strings.Contains(err.Error(), "must match unit filename") {
		t.Fatalf("mismatched service identity error = %v", err)
	}
}

func TestIsolatedDefaultsAndEnvironmentCannotReachProductionEndpoints(t *testing.T) {
	opts := DefaultOptions("/tmp/stagecore-rc-release")
	opts.InstallRoot = "/opt/stagecore-rc-qualification"
	opts.ConfigRoot = "/etc/stagecore-rc-qualification"
	opts.DataRoot = "/var/lib/stagecore-rc-qualification/data"
	opts.VaultRoot = "/var/lib/stagecore-rc-qualification/vault"
	opts.SystemdUnit = "/etc/systemd/system/stagecore-rc-qualification.service"
	normalized, err := normalizeOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Listen != "127.0.0.1:17840" || normalized.OperatorTLSListen != "127.0.0.1:17842" || normalized.DeviceListen != "127.0.0.1:17841" {
		t.Fatalf("isolated listeners are not loopback-only: %+v", normalized)
	}
	if err := ValidateIsolatedInstallPaths(normalized); err != nil {
		t.Fatal(err)
	}
	env := RenderEnvironment(normalized)
	if !strings.Contains(env, "STAGECORE_HA_MODE=STANDALONE") {
		t.Fatalf("isolated environment has a Hub-incompatible HA mode: %s", env)
	}
	if err := ValidateIsolatedEnvironment([]byte(env), normalized); err != nil {
		t.Fatalf("rendered isolated environment rejected: %v", err)
	}

	for _, unsafe := range []string{
		env + "STAGECORE_OSC_INPUT_LISTEN=0.0.0.0:9000\n",
		env + "STAGECORE_USB_DMX_ENABLE=1\n",
		env + "STAGECORE_HA_MODE=cluster\n",
		env + "STAGECORE_DATA_ROOT=/var/lib/stagecore/data\n",
	} {
		if err := ValidateIsolatedEnvironment([]byte(unsafe), normalized); err == nil {
			t.Fatalf("unsafe isolated environment accepted:\n%s", unsafe)
		}
	}
	if _, err := normalizeOptions(Options{
		BundleDir: "/tmp/stagecore-rc-release", InstallRoot: normalized.InstallRoot,
		ConfigRoot: normalized.ConfigRoot, DataRoot: normalized.DataRoot, VaultRoot: normalized.VaultRoot,
		SystemdUnit: normalized.SystemdUnit, Listen: "0.0.0.0:17840",
	}); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("non-loopback isolated Hub listener error = %v", err)
	}
}

func TestValidateIsolatedEnvironmentUsesHubStandaloneToken(t *testing.T) {
	opts := DefaultOptions("/tmp/stagecore-rc-release")
	opts.InstallRoot = "/opt/stagecore-rc-qualification"
	opts.ConfigRoot = "/etc/stagecore-rc-qualification"
	opts.DataRoot = "/var/lib/stagecore-rc-qualification/data"
	opts.VaultRoot = "/var/lib/stagecore-rc-qualification/vault"
	opts.SystemdUnit = "/etc/systemd/system/stagecore-rc-qualification.service"
	var err error
	opts, err = normalizeOptions(opts)
	if err != nil {
		t.Fatal(err)
	}

	legacyConfig := []byte(RenderEnvironment(opts))
	if err := ValidateIsolatedEnvironment(legacyConfig, opts); err != nil {
		t.Fatalf("Hub's STANDALONE token should be accepted: %v", err)
	}

	lowercaseConfig := strings.Replace(string(legacyConfig), "STAGECORE_HA_MODE=STANDALONE", "STAGECORE_HA_MODE=standalone", 1)
	if err := ValidateIsolatedEnvironment([]byte(lowercaseConfig), opts); err == nil || !strings.Contains(err.Error(), "STAGECORE_HA_MODE=STANDALONE") {
		t.Fatalf("Hub-incompatible lowercase mode was accepted: %v", err)
	}
}

func TestRenderedIsolatedEnvironmentLoadsInHubConfig(t *testing.T) {
	opts := DefaultOptions("/tmp/stagecore-rc-release")
	opts.InstallRoot = "/opt/stagecore-rc-qualification"
	opts.ConfigRoot = "/etc/stagecore-rc-qualification"
	opts.DataRoot = "/var/lib/stagecore-rc-qualification/data"
	opts.VaultRoot = "/var/lib/stagecore-rc-qualification/vault"
	opts.SystemdUnit = "/etc/systemd/system/stagecore-rc-qualification.service"
	var err error
	opts, err = normalizeOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(RenderEnvironment(opts), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			t.Setenv(key, value)
		}
	}

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Hub rejected generated isolated environment: %v", err)
	}
	if cfg.HAMode != config.HAModeStandalone {
		t.Fatalf("Hub HA mode = %q, want %q", cfg.HAMode, config.HAModeStandalone)
	}
}

func TestIsolatedSystemdUnitIsConstrainedAndNotEnabled(t *testing.T) {
	opts := DefaultOptions("/tmp/stagecore-rc-release")
	opts.InstallRoot = "/opt/stagecore-rc-qualification"
	opts.ConfigRoot = "/etc/stagecore-rc-qualification"
	opts.DataRoot = "/var/lib/stagecore-rc-qualification/data"
	opts.VaultRoot = "/var/lib/stagecore-rc-qualification/vault"
	opts.SystemdUnit = "/etc/systemd/system/stagecore-rc-qualification.service"
	var err error
	opts, err = normalizeOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	unit := RenderSystemdUnit(opts)
	for _, marker := range []string{
		"Description=StageCore isolated qualification Hub (stagecore-rc-qualification.service)",
		"ExecStart=/opt/stagecore-rc-qualification/bin/stagecore-hub",
		"EnvironmentFile=/etc/stagecore-rc-qualification/stagecore.env",
		"PrivateDevices=yes", "DevicePolicy=closed", "PrivateNetwork=yes", "IPAddressDeny=any", "IPAddressAllow=localhost",
		"RestrictAddressFamilies=AF_INET AF_INET6", "InaccessiblePaths=-/var/lib/stagecore -/var/backups/stagecore", "ProtectSystem=strict",
		"ReadWritePaths=/var/lib/stagecore-rc-qualification/data /var/lib/stagecore-rc-qualification/vault",
		"Restart=no", "UMask=0077",
	} {
		if !strings.Contains(unit, marker) {
			t.Fatalf("isolated unit missing %q:\n%s", marker, unit)
		}
	}
	if strings.Contains(unit, "stagecore-hub.service") || strings.Contains(unit, "stagecore-camera-relay.service") || strings.Contains(unit, "[Install]") {
		t.Fatalf("isolated unit references or enables a production unit:\n%s", unit)
	}
	if strings.Contains(unit, "/run") || strings.Contains(unit, "/var/run") {
		t.Fatalf("isolated unit hides systemd's mount-namespace runtime paths:\n%s", unit)
	}
	defaultUnit := RenderSystemdUnit(DefaultOptions("/tmp/release"))
	for _, marker := range []string{"Restart=on-failure", "ProtectSystem=strict", "[Install]", "WantedBy=multi-user.target"} {
		if !strings.Contains(defaultUnit, marker) {
			t.Fatalf("production default unit changed; missing %q", marker)
		}
	}
}

func TestValidateIsolatedUpdatePathsRejectsProductionAndOverlappingRoots(t *testing.T) {
	root := t.TempDir()
	base := Options{
		BundleDir:   filepath.Join(root, "install", "bundles", "candidate"),
		InstallRoot: filepath.Join(root, "install"),
		ConfigRoot:  filepath.Join(root, "config"),
		DataRoot:    filepath.Join(root, "data"),
		VaultRoot:   filepath.Join(root, "vault"),
		SystemdUnit: filepath.Join(root, "systemd", "stagecore-rc-qualification.service"),
		ServiceName: "stagecore-rc-qualification.service",
	}
	backupRoot := filepath.Join(root, "update-backups")
	if err := ValidateIsolatedUpdatePaths(base, backupRoot); err != nil {
		bundlePath, _ := canonicalPath(base.BundleDir)
		backupPath, backupErr := canonicalPath("/var/backups/stagecore")
		t.Fatalf("safe isolated paths rejected: %v; bundle=%q => %q, protected=%q (%v)", err, base.BundleDir, bundlePath, backupPath, backupErr)
	}

	cases := []struct {
		name string
		edit func(*Options, *string)
	}{
		{"production data root", func(o *Options, _ *string) { o.DataRoot = DefaultDataRoot }},
		{"overlapping install and config", func(o *Options, _ *string) { o.ConfigRoot = filepath.Join(o.InstallRoot, "etc") }},
		{"production service unit", func(o *Options, _ *string) { o.SystemdUnit = DefaultUnitPath }},
		{"production backup root", func(_ *Options, b *string) { *b = "/var/backups/stagecore/updates" }},
		{"relative backup root", func(_ *Options, b *string) { *b = "../updates" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := base
			backup := backupRoot
			tc.edit(&opts, &backup)
			if err := ValidateIsolatedUpdatePaths(opts, backup); err == nil {
				t.Fatal("unsafe isolated target accepted")
			}
		})
	}
}

func TestValidateIsolatedPathsRejectsSymlinkedRoots(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "data-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		BundleDir:   filepath.Join(root, "install", "bundles"),
		InstallRoot: filepath.Join(root, "install"),
		ConfigRoot:  filepath.Join(root, "config"),
		DataRoot:    link,
		VaultRoot:   filepath.Join(root, "vault"),
		SystemdUnit: filepath.Join(root, "systemd", "stagecore-rc-qualification.service"),
		ServiceName: "stagecore-rc-qualification.service",
	}
	if err := ValidateIsolatedInstallPaths(opts); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked data root error = %v", err)
	}
}

func TestInstallerRejectsIsolatedProductionPathsBeforePlatformOrCommands(t *testing.T) {
	opts := DefaultOptions("/tmp/stagecore-rc-bundle")
	opts.InstallRoot = "/opt/stagecore-rc-test"
	opts.ConfigRoot = "/etc/stagecore-rc-test"
	opts.DataRoot = DefaultDataRoot
	opts.VaultRoot = "/var/lib/stagecore-rc-test/vault"
	opts.SystemdUnit = "/etc/systemd/system/stagecore-rc-test.service"
	installer := NewInstaller()
	installer.Runner = panicRunner{t: t}
	if _, err := installer.Install(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "protected production root") {
		t.Fatalf("unsafe isolated install error = %v", err)
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

func TestReplaceEnvironmentValuePreservesOtherDeploymentSettings(t *testing.T) {
	original := []byte("# existing\n" +
		"STAGECORE_DATA_ROOT=/srv/data\n" +
		"STAGECORE_VAULT_ROOT=/srv/vault\n" +
		"STAGECORE_LISTEN=127.0.0.1:7840\n" +
		"STAGECORE_OSC_INPUT_LISTEN=127.0.0.1:9000\n" +
		"STAGECORE_MTC_INPUT_DEVICE=/dev/snd/midiC4D1\n")
	updated, err := replaceEnvironmentValue(original, "STAGECORE_LISTEN", "0.0.0.0:7840")
	if err != nil {
		t.Fatal(err)
	}
	text := string(updated)
	if !strings.Contains(text, "STAGECORE_LISTEN=0.0.0.0:7840") {
		t.Fatalf("listen not updated: %s", text)
	}
	for _, preserved := range []string{
		"STAGECORE_OSC_INPUT_LISTEN=127.0.0.1:9000",
		"STAGECORE_MTC_INPUT_DEVICE=/dev/snd/midiC4D1",
		"STAGECORE_DATA_ROOT=/srv/data",
	} {
		if !strings.Contains(text, preserved) {
			t.Fatalf("existing setting %q was not preserved: %s", preserved, text)
		}
	}
}

func TestDryRunCanMigrateOnlyExistingListen(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64") {
		t.Skip("dry-run bundle test requires supported Linux CI architecture")
	}
	bundle := makeNativeTestBundle(t)
	configRoot := t.TempDir()
	env := []byte("STAGECORE_DATA_ROOT=/srv/stagecore/data\n" +
		"STAGECORE_VAULT_ROOT=/srv/stagecore/vault\n" +
		"STAGECORE_LISTEN=127.0.0.1:7840\n" +
		"STAGECORE_OSC_PLUGIN_PATH=/opt/stagecore/bin/stagecore-osc-plugin\n" +
		"STAGECORE_OSC_INPUT_LISTEN=127.0.0.1:9000\n")
	if err := os.WriteFile(filepath.Join(configRoot, "stagecore.env"), env, 0o640); err != nil {
		t.Fatal(err)
	}

	opts := DefaultOptions(bundle)
	opts.ConfigRoot = configRoot
	opts.Listen = "0.0.0.0:7840"
	opts.ListenExplicit = true
	opts.DryRun = true

	installer := NewInstaller()
	installer.EUID = func() int { return 1000 }
	installer.Runner = panicRunner{t: t}
	result, err := installer.Install(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ConfigPreserved || !result.ListenUpdated {
		t.Fatalf("expected preserved config with listen-only migration: %+v", result)
	}
	if result.Effective.Listen != "0.0.0.0:7840" {
		t.Fatalf("effective listen=%q", result.Effective.Listen)
	}
	if result.ReadinessURL != "http://127.0.0.1:7840/health/ready" {
		t.Fatalf("readiness URL=%q", result.ReadinessURL)
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
		"STAGECORE_OPERATOR_TLS_LISTEN=0.0.0.0:7842",
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
		"0.0.0.0:7840":  "http://127.0.0.1:7840/health/ready",
		":7840":         "http://127.0.0.1:7840/health/ready",
		"[::]:7840":     "http://[::1]:7840/health/ready",
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
