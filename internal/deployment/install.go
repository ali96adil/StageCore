package deployment

import (
	"bufio"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultInstallRoot = "/opt/stagecore"
	DefaultConfigRoot  = "/etc/stagecore"
	DefaultDataRoot    = "/var/lib/stagecore/data"
	DefaultVaultRoot   = "/var/lib/stagecore/vault"
	DefaultServiceUser = "stagecore"
	DefaultServiceGroup = "stagecore"
	DefaultListen      = "127.0.0.1:7840"
	DefaultUnitPath    = "/etc/systemd/system/stagecore-hub.service"
	checksumFileName   = "SHA256SUMS"
)

var RequiredBinaries = []string{
	"stagecore-hub",
	"stagecore-osc-plugin",
	"stagecore-pairing",
	"stagecore-setup",
}

type Options struct {
	BundleDir      string
	InstallRoot    string
	ConfigRoot     string
	DataRoot       string
	VaultRoot      string
	ServiceUser    string
	ServiceGroup   string
	Listen         string
	SystemdUnit    string
	ReplaceConfig  bool
	NoStart        bool
	DryRun         bool
	ReadinessTimeout time.Duration
}

type Result struct {
	Effective       Options
	Actions         []string
	ConfigPreserved bool
	ReadinessURL    string
}

type commandRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return output, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return output, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, message)
	}
	return output, nil
}

type Installer struct {
	GOOS       string
	GOARCH     string
	EUID       func() int
	Runner     commandRunner
	HTTPClient *http.Client
	Now        func() time.Time
}

func NewInstaller() *Installer {
	return &Installer{
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
		EUID:   os.Geteuid,
		Runner: execRunner{},
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
		Now: time.Now,
	}
}

func DefaultOptions(bundleDir string) Options {
	return Options{
		BundleDir:       bundleDir,
		InstallRoot:     DefaultInstallRoot,
		ConfigRoot:      DefaultConfigRoot,
		DataRoot:        DefaultDataRoot,
		VaultRoot:       DefaultVaultRoot,
		ServiceUser:     DefaultServiceUser,
		ServiceGroup:    DefaultServiceGroup,
		Listen:          DefaultListen,
		SystemdUnit:     DefaultUnitPath,
		ReadinessTimeout: 20 * time.Second,
	}
}

func (i *Installer) Install(ctx context.Context, opts Options) (Result, error) {
	if i == nil {
		return Result{}, fmt.Errorf("installer is required")
	}
	if i.Runner == nil {
		i.Runner = execRunner{}
	}
	if i.EUID == nil {
		i.EUID = os.Geteuid
	}
	if i.HTTPClient == nil {
		i.HTTPClient = &http.Client{Timeout: 2 * time.Second}
	}
	if i.Now == nil {
		i.Now = time.Now
	}

	effective, err := normalizeOptions(opts)
	if err != nil {
		return Result{}, err
	}
	if err := validatePlatform(i.GOOS, i.GOARCH); err != nil {
		return Result{}, err
	}
	if err := ValidateBundle(effective.BundleDir, i.GOARCH); err != nil {
		return Result{}, err
	}

	result := Result{Effective: effective}
	result.Actions = append(result.Actions,
		fmt.Sprintf("validate release bundle %s", effective.BundleDir),
		fmt.Sprintf("ensure service identity %s:%s", effective.ServiceUser, effective.ServiceGroup),
		fmt.Sprintf("ensure install/config/data/vault directories under %s, %s, %s and %s", effective.InstallRoot, effective.ConfigRoot, effective.DataRoot, effective.VaultRoot),
		fmt.Sprintf("install %d StageCore binaries into %s", len(RequiredBinaries), filepath.Join(effective.InstallRoot, "bin")),
	)

	envPath := filepath.Join(effective.ConfigRoot, "stagecore.env")
	if !effective.ReplaceConfig {
		if existing, readErr := os.ReadFile(envPath); readErr == nil {
			adopted, adoptErr := adoptExistingConfig(effective, existing)
			if adoptErr != nil {
				return Result{}, fmt.Errorf("preserve existing %s: %w (use --replace-config only after reviewing the existing deployment configuration)", envPath, adoptErr)
			}
			effective = adopted
			result.Effective = effective
			result.ConfigPreserved = true
			result.Actions = append(result.Actions, fmt.Sprintf("preserve existing deployment configuration %s", envPath))
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return Result{}, fmt.Errorf("read existing deployment configuration %s: %w", envPath, readErr)
		} else {
			result.Actions = append(result.Actions, fmt.Sprintf("write managed deployment configuration %s", envPath))
		}
	} else {
		result.Actions = append(result.Actions, fmt.Sprintf("replace deployment configuration %s by explicit request", envPath))
	}

	result.Actions = append(result.Actions,
		fmt.Sprintf("install systemd unit %s", effective.SystemdUnit),
		"systemctl daemon-reload",
		"systemctl enable stagecore-hub.service",
	)
	if effective.NoStart {
		result.Actions = append(result.Actions, "leave stagecore-hub stopped/not restarted (--no-start)")
	} else {
		result.ReadinessURL, err = ReadinessURL(effective.Listen)
		if err != nil {
			return Result{}, err
		}
		result.Actions = append(result.Actions,
			"systemctl restart stagecore-hub.service",
			fmt.Sprintf("wait for readiness at %s", result.ReadinessURL),
		)
	}

	if effective.DryRun {
		return result, nil
	}
	if i.EUID() != 0 {
		return Result{}, fmt.Errorf("installation requires root privileges; rerun with sudo or use --dry-run")
	}

	if err := i.ensureIdentity(ctx, effective.ServiceUser, effective.ServiceGroup, effective.DataRoot); err != nil {
		return Result{}, err
	}
	uid, gid, err := lookupIdentity(effective.ServiceUser, effective.ServiceGroup)
	if err != nil {
		return Result{}, err
	}
	if err := ensureDirectory(filepath.Join(effective.InstallRoot, "bin"), 0o755, 0, 0); err != nil {
		return Result{}, err
	}
	if err := ensureDirectory(effective.ConfigRoot, 0o755, 0, 0); err != nil {
		return Result{}, err
	}
	if err := ensureDirectory(effective.DataRoot, 0o750, uid, gid); err != nil {
		return Result{}, err
	}
	if err := ensureDirectory(effective.VaultRoot, 0o750, uid, gid); err != nil {
		return Result{}, err
	}

	for _, name := range RequiredBinaries {
		source := filepath.Join(effective.BundleDir, name)
		destination := filepath.Join(effective.InstallRoot, "bin", name)
		if err := copyFileAtomic(source, destination, 0o755); err != nil {
			return Result{}, fmt.Errorf("install %s: %w", name, err)
		}
	}

	if !result.ConfigPreserved || effective.ReplaceConfig {
		content := []byte(RenderEnvironment(effective))
		if err := writeFileAtomic(envPath, content, 0o640); err != nil {
			return Result{}, fmt.Errorf("write deployment configuration: %w", err)
		}
	}
	unit := []byte(RenderSystemdUnit(effective))
	if err := writeFileAtomic(effective.SystemdUnit, unit, 0o644); err != nil {
		return Result{}, fmt.Errorf("write systemd unit: %w", err)
	}

	if _, err := i.Runner.Output(ctx, "systemctl", "daemon-reload"); err != nil {
		return Result{}, err
	}
	if _, err := i.Runner.Output(ctx, "systemctl", "enable", "stagecore-hub.service"); err != nil {
		return Result{}, err
	}
	if effective.NoStart {
		return result, nil
	}
	if _, err := i.Runner.Output(ctx, "systemctl", "restart", "stagecore-hub.service"); err != nil {
		return Result{}, err
	}
	if err := i.waitReady(ctx, result.ReadinessURL, effective.ReadinessTimeout); err != nil {
		return Result{}, err
	}
	return result, nil
}

func normalizeOptions(opts Options) (Options, error) {
	defaults := DefaultOptions(opts.BundleDir)
	if strings.TrimSpace(opts.BundleDir) == "" {
		return Options{}, fmt.Errorf("release bundle directory is required")
	}
	if strings.TrimSpace(opts.InstallRoot) == "" {
		opts.InstallRoot = defaults.InstallRoot
	}
	if strings.TrimSpace(opts.ConfigRoot) == "" {
		opts.ConfigRoot = defaults.ConfigRoot
	}
	if strings.TrimSpace(opts.DataRoot) == "" {
		opts.DataRoot = defaults.DataRoot
	}
	if strings.TrimSpace(opts.VaultRoot) == "" {
		opts.VaultRoot = defaults.VaultRoot
	}
	if strings.TrimSpace(opts.ServiceUser) == "" {
		opts.ServiceUser = defaults.ServiceUser
	}
	if strings.TrimSpace(opts.ServiceGroup) == "" {
		opts.ServiceGroup = defaults.ServiceGroup
	}
	if strings.TrimSpace(opts.Listen) == "" {
		opts.Listen = defaults.Listen
	}
	if strings.TrimSpace(opts.SystemdUnit) == "" {
		opts.SystemdUnit = defaults.SystemdUnit
	}
	if opts.ReadinessTimeout <= 0 {
		opts.ReadinessTimeout = defaults.ReadinessTimeout
	}

	var err error
	for label, value := range map[string]string{
		"bundle directory": opts.BundleDir,
		"install root":     opts.InstallRoot,
		"config root":      opts.ConfigRoot,
		"data root":        opts.DataRoot,
		"vault root":       opts.VaultRoot,
		"systemd unit":     opts.SystemdUnit,
	} {
		cleaned := filepath.Clean(strings.TrimSpace(value))
		if !filepath.IsAbs(cleaned) {
			return Options{}, fmt.Errorf("%s must be an absolute path: %q", label, value)
		}
		switch label {
		case "bundle directory":
			opts.BundleDir = cleaned
		case "install root":
			opts.InstallRoot = cleaned
		case "config root":
			opts.ConfigRoot = cleaned
		case "data root":
			opts.DataRoot = cleaned
		case "vault root":
			opts.VaultRoot = cleaned
		case "systemd unit":
			opts.SystemdUnit = cleaned
		}
	}
	if opts.DataRoot == opts.VaultRoot {
		return Options{}, fmt.Errorf("data root and vault root must be distinct deployment paths")
	}
	if opts.ServiceUser = strings.TrimSpace(opts.ServiceUser); opts.ServiceUser == "" || strings.ContainsAny(opts.ServiceUser, " /\\\t\n") {
		return Options{}, fmt.Errorf("invalid service user %q", opts.ServiceUser)
	}
	if opts.ServiceGroup = strings.TrimSpace(opts.ServiceGroup); opts.ServiceGroup == "" || strings.ContainsAny(opts.ServiceGroup, " /\\\t\n") {
		return Options{}, fmt.Errorf("invalid service group %q", opts.ServiceGroup)
	}
	if _, _, err = net.SplitHostPort(strings.TrimSpace(opts.Listen)); err != nil {
		return Options{}, fmt.Errorf("invalid listen address %q: %w", opts.Listen, err)
	}
	opts.Listen = strings.TrimSpace(opts.Listen)
	return opts, nil
}

func validatePlatform(goos, goarch string) error {
	if goos != "linux" {
		return fmt.Errorf("unsupported install OS %q; F-005 foundation supports 64-bit Linux only", goos)
	}
	if goarch != "arm64" && goarch != "amd64" {
		return fmt.Errorf("unsupported install architecture %q; supported architectures are arm64 and amd64", goarch)
	}
	return nil
}

func ValidateBundle(bundleDir, goarch string) error {
	checksums, err := readChecksums(filepath.Join(bundleDir, checksumFileName))
	if err != nil {
		return err
	}
	for _, name := range RequiredBinaries {
		path := filepath.Join(bundleDir, name)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("required release artifact %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("required release artifact %s must not be a symlink", name)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("required release artifact %s is not a regular file", name)
		}
		expected, ok := checksums[name]
		if !ok {
			return fmt.Errorf("%s has no checksum entry in %s", name, checksumFileName)
		}
		actual, err := fileSHA256(path)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", name, err)
		}
		if actual != expected {
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", name, actual, expected)
		}
		if err := validateELFArchitecture(path, goarch); err != nil {
			return fmt.Errorf("validate %s architecture: %w", name, err)
		}
	}
	return nil
}

func readChecksums(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", checksumFileName, err)
	}
	defer file.Close()
	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid %s line %q", checksumFileName, line)
		}
		hash := strings.ToLower(fields[0])
		if len(hash) != sha256.Size*2 {
			return nil, fmt.Errorf("invalid SHA-256 for %s", fields[1])
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, fmt.Errorf("invalid SHA-256 for %s: %w", fields[1], err)
		}
		name := strings.TrimPrefix(fields[1], "*")
		if filepath.Base(name) != name {
			return nil, fmt.Errorf("checksum filename must be a bundle-local basename: %q", name)
		}
		checksums[name] = hash
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", checksumFileName, err)
	}
	return checksums, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateELFArchitecture(path, goarch string) error {
	file, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open ELF: %w", err)
	}
	defer file.Close()
	var expected elf.Machine
	switch goarch {
	case "arm64":
		expected = elf.EM_AARCH64
	case "amd64":
		expected = elf.EM_X86_64
	default:
		return fmt.Errorf("unsupported architecture %q", goarch)
	}
	if file.Machine != expected {
		return fmt.Errorf("ELF machine=%s, expected %s for %s", file.Machine, expected, goarch)
	}
	return nil
}

func adoptExistingConfig(opts Options, content []byte) (Options, error) {
	values, err := parseEnvironment(content)
	if err != nil {
		return Options{}, err
	}
	required := []string{"STAGECORE_DATA_ROOT", "STAGECORE_VAULT_ROOT", "STAGECORE_LISTEN", "STAGECORE_OSC_PLUGIN_PATH"}
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			return Options{}, fmt.Errorf("existing configuration is missing %s", key)
		}
	}
	opts.DataRoot = values["STAGECORE_DATA_ROOT"]
	opts.VaultRoot = values["STAGECORE_VAULT_ROOT"]
	opts.Listen = values["STAGECORE_LISTEN"]
	pluginDir := filepath.Dir(values["STAGECORE_OSC_PLUGIN_PATH"])
	expectedBinDir := filepath.Join(opts.InstallRoot, "bin")
	if filepath.Clean(pluginDir) != filepath.Clean(expectedBinDir) {
		return Options{}, fmt.Errorf("existing STAGECORE_OSC_PLUGIN_PATH %q is outside managed binary directory %q", values["STAGECORE_OSC_PLUGIN_PATH"], expectedBinDir)
	}
	return normalizeOptions(opts)
}

func parseEnvironment(content []byte) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid environment line %q", line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if key == "" {
			return nil, fmt.Errorf("environment key is empty")
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func RenderEnvironment(opts Options) string {
	plugin := filepath.Join(opts.InstallRoot, "bin", "stagecore-osc-plugin")
	return strings.Join([]string{
		"# Managed by stagecore-setup install. Review before using --replace-config.",
		"STAGECORE_DATA_ROOT=" + opts.DataRoot,
		"STAGECORE_VAULT_ROOT=" + opts.VaultRoot,
		"STAGECORE_LISTEN=" + opts.Listen,
		"STAGECORE_OSC_PLUGIN_PATH=" + plugin,
		"",
	}, "\n")
}

func RenderSystemdUnit(opts Options) string {
	envPath := filepath.Join(opts.ConfigRoot, "stagecore.env")
	hub := filepath.Join(opts.InstallRoot, "bin", "stagecore-hub")
	return fmt.Sprintf(`[Unit]
Description=StageCore Hub
After=local-fs.target network.target
Wants=network.target
RequiresMountsFor=%s %s

[Service]
Type=simple
User=%s
Group=%s
EnvironmentFile=%s
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=2s
TimeoutStopSec=10s
KillSignal=SIGTERM
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=%s %s
UMask=0027
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`, opts.DataRoot, opts.VaultRoot, opts.ServiceUser, opts.ServiceGroup, envPath, opts.DataRoot, hub, opts.DataRoot, opts.VaultRoot)
}

func ReadinessURL(listen string) (string, error) {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("derive readiness URL from %q: %w", listen, err)
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/health/ready", nil
}

func (i *Installer) ensureIdentity(ctx context.Context, serviceUser, serviceGroup, dataRoot string) error {
	if _, err := i.Runner.Output(ctx, "getent", "group", serviceGroup); err != nil {
		if _, createErr := i.Runner.Output(ctx, "groupadd", "--system", serviceGroup); createErr != nil {
			return fmt.Errorf("create service group %s: %w", serviceGroup, createErr)
		}
	}
	if _, err := i.Runner.Output(ctx, "id", "-u", serviceUser); err != nil {
		if _, createErr := i.Runner.Output(ctx, "useradd", "--system", "--gid", serviceGroup, "--home-dir", dataRoot, "--shell", "/usr/sbin/nologin", serviceUser); createErr != nil {
			return fmt.Errorf("create service user %s: %w", serviceUser, createErr)
		}
		return nil
	}
	group, err := i.Runner.Output(ctx, "id", "-gn", serviceUser)
	if err != nil {
		return fmt.Errorf("inspect primary group for %s: %w", serviceUser, err)
	}
	if strings.TrimSpace(string(group)) != serviceGroup {
		return fmt.Errorf("existing service user %s has primary group %q, expected %q; refusing to rewrite account ownership", serviceUser, strings.TrimSpace(string(group)), serviceGroup)
	}
	return nil
}

func lookupIdentity(serviceUser, serviceGroup string) (int, int, error) {
	u, err := user.Lookup(serviceUser)
	if err != nil {
		return 0, 0, fmt.Errorf("lookup service user %s: %w", serviceUser, err)
	}
	g, err := user.LookupGroup(serviceGroup)
	if err != nil {
		return 0, 0, fmt.Errorf("lookup service group %s: %w", serviceGroup, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse service uid: %w", err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, 0, fmt.Errorf("parse service gid: %w", err)
	}
	return uid, gid, nil
}

func ensureDirectory(path string, mode os.FileMode, uid, gid int) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod directory %s: %w", path, err)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown directory %s: %w", path, err)
	}
	return nil
}

func copyFileAtomic(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	return writeFromReaderAtomic(destination, input, mode)
}

func writeFileAtomic(destination string, content []byte, mode os.FileMode) error {
	return writeFromReaderAtomic(destination, strings.NewReader(string(content)), mode)
}

func writeFromReaderAtomic(destination string, source io.Reader, mode os.FileMode) error {
	dir := filepath.Dir(destination)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".stagecore-install-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
		}
	}()
	if _, err := io.Copy(tmp, source); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return err
	}
	ok = true
	return nil
}

func (i *Installer) waitReady(ctx context.Context, readinessURL string, timeout time.Duration) error {
	deadline := i.Now().Add(timeout)
	var last string
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, readinessURL, nil)
		if err != nil {
			return err
		}
		response, err := i.HTTPClient.Do(req)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			last = fmt.Sprintf("HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return nil
			}
		} else {
			last = err.Error()
		}
		if !i.Now().Before(deadline) {
			return fmt.Errorf("stagecore-hub did not become ready at %s within %s; last result: %s", readinessURL, timeout, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
