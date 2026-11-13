//go:build linux

package deployment

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

func runSystemdNamespaceProbe(ctx context.Context, process systemdServiceProcess, helperBinary, endpoint string, timeout time.Duration) ([]byte, error) {
	pidfd, err := unix.PidfdOpen(process.PID, 0)
	if err != nil {
		return nil, fmt.Errorf("pin isolated service process %d: %w", process.PID, err)
	}
	defer unix.Close(pidfd)

	cgroupPath := filepath.Join("/proc", strconv.Itoa(process.PID), "cgroup")
	contents, err := os.ReadFile(cgroupPath)
	if err != nil || !processBelongsToSystemdControlGroup(string(contents), process.ControlGroup) {
		return nil, fmt.Errorf("MainPID %d no longer belongs to the selected systemd service", process.PID)
	}
	networkNamespace, err := os.Open(filepath.Join("/proc", strconv.Itoa(process.PID), "ns", "net"))
	if err != nil {
		return nil, fmt.Errorf("open network namespace for isolated service %d: %w", process.PID, err)
	}
	defer networkNamespace.Close()

	contents, err = os.ReadFile(cgroupPath)
	if err != nil || !processBelongsToSystemdControlGroup(string(contents), process.ControlGroup) {
		return nil, fmt.Errorf("isolated service process changed while opening its network namespace")
	}
	if _, err := os.Stat("/usr/bin/nsenter"); err != nil {
		return nil, fmt.Errorf("required namespace tool /usr/bin/nsenter is unavailable: %w", err)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/nsenter", "--net=/proc/self/fd/3", "--", helperBinary,
		SystemdHealthProbeCommand, "--url", endpoint, "--timeout", timeout.String())
	cmd.ExtraFiles = []*os.File{networkNamespace}
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run isolated systemd health probe: %w", err)
	}
	return output, nil
}
