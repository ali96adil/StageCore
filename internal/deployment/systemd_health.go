package deployment

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const SystemdHealthProbeCommand = "__health-probe"

type SystemdCommandFunc func(context.Context, string, ...string) ([]byte, error)

type SystemdHealthProbeResult struct {
	HTTPStatus   int    `json:"http_status"`
	Status       string `json:"status"`
	StorageState string `json:"storage_state"`
}

type systemdServiceProcess struct {
	PID          int
	ControlGroup string
}

func SystemdNamespaceHealthProbe(ctx context.Context, opts Options, helperBinary, endpoint string, timeout time.Duration, run SystemdCommandFunc) ([]byte, error) {
	if run == nil {
		return nil, fmt.Errorf("systemd command runner is required")
	}
	if !IsolatedService(opts) {
		return nil, fmt.Errorf("network namespace health probes require an isolated service")
	}
	if err := ValidateSystemdOperationPath(opts); err != nil {
		return nil, err
	}
	if err := ValidateSystemdHealthProbeBinary(opts, helperBinary); err != nil {
		return nil, err
	}
	if err := ValidateSystemdHealthProbeURL(endpoint); err != nil {
		return nil, err
	}
	if timeout < time.Second || timeout > 2*time.Minute {
		return nil, fmt.Errorf("systemd health probe timeout must be between 1s and 2m")
	}
	if err := ValidateSystemdServiceTarget(ctx, opts, systemdCommand(run)); err != nil {
		return nil, err
	}
	output, err := run(ctx, "systemctl", "show", "--no-pager", "--property=MainPID", "--property=ControlGroup", opts.ServiceName)
	if err != nil {
		return nil, fmt.Errorf("inspect MainPID for isolated systemd target %s: %w", opts.ServiceName, err)
	}
	process, err := parseSystemdServiceProcess(string(output))
	if err != nil {
		return nil, fmt.Errorf("inspect isolated systemd process %s: %w", opts.ServiceName, err)
	}
	return runSystemdNamespaceProbe(ctx, process, helperBinary, endpoint, timeout)
}

func ValidateSystemdHealthProbeURL(endpoint string) error {
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return fmt.Errorf("invalid systemd health probe URL: %w", err)
	}
	if parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("systemd health probe URL must be a plain HTTP loopback endpoint")
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("systemd health probe URL must use a loopback IP address")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("systemd health probe URL must include a valid port")
	}
	if parsed.Path != "/health/live" && parsed.Path != "/health/ready" {
		return fmt.Errorf("systemd health probe URL path must be /health/live or /health/ready")
	}
	return nil
}

func ValidateSystemdHealthProbeBinary(opts Options, helperBinary string) error {
	cleaned := filepath.Clean(strings.TrimSpace(helperBinary))
	if !filepath.IsAbs(cleaned) || hasParentPathComponent(helperBinary) {
		return fmt.Errorf("systemd health probe binary must be an absolute path without traversal")
	}
	allowed := map[string]bool{
		filepath.Join(opts.InstallRoot, "bin", "stagecore-setup"): true,
	}
	if strings.TrimSpace(opts.BundleDir) != "" {
		allowed[filepath.Join(opts.BundleDir, "stagecore-setup")] = true
	}
	if !allowed[cleaned] {
		return fmt.Errorf("systemd health probe binary must be the managed stagecore-setup executable")
	}
	if err := rejectSymlinkPath(cleaned); err != nil {
		return fmt.Errorf("systemd health probe binary: %w", err)
	}
	info, err := os.Lstat(cleaned)
	if err != nil {
		return fmt.Errorf("inspect systemd health probe binary: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("systemd health probe binary must be a regular executable")
	}
	return nil
}

func parseSystemdServiceProcess(output string) (systemdServiceProcess, error) {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if _, duplicate := values[key]; duplicate {
			return systemdServiceProcess{}, fmt.Errorf("systemd returned duplicate %s properties", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	pid, err := strconv.Atoi(values["MainPID"])
	if err != nil || pid <= 1 {
		return systemdServiceProcess{}, fmt.Errorf("isolated service MainPID is not running")
	}
	controlGroup := values["ControlGroup"]
	if !filepath.IsAbs(controlGroup) || hasParentPathComponent(controlGroup) || strings.Contains(controlGroup, "\\") {
		return systemdServiceProcess{}, fmt.Errorf("isolated service ControlGroup is invalid")
	}
	return systemdServiceProcess{PID: pid, ControlGroup: filepath.Clean(controlGroup)}, nil
}

func processBelongsToSystemdControlGroup(contents, expected string) bool {
	for _, line := range strings.Split(contents, "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) != 3 || fields[0] != "0" || fields[1] != "" {
			continue
		}
		actual := filepath.Clean(fields[2])
		if actual == expected || strings.HasPrefix(actual, expected+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
