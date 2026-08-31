package extension

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
)

type fakeRuntimeProbeHost struct {
	ready  *pluginprotocol.Ready
	err    error
	closed bool
}

func (h *fakeRuntimeProbeHost) Probe(context.Context) (*pluginprotocol.Ready, error) {
	return h.ready, h.err
}

func (h *fakeRuntimeProbeHost) Close() { h.closed = true }

func newRuntimeProbeForHarness(t *testing.T, h *dependencyTestHarness, sandboxPath string) (*RuntimeProbe, *PermissionReviewer) {
	t.Helper()
	stager, reviewer := newActivationStagerForHarness(t, h)
	isolator, err := NewRuntimeIsolator(h.installer, reviewer, stager.assessor, sandboxPath)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := NewRuntimeProbe(h.installer, isolator)
	if err != nil {
		t.Fatal(err)
	}
	return probe, reviewer
}

func TestRuntimeProbeVerifiesReadyThenStopsAndCleansTransientExecutable(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := newRuntimeProbeForHarness(t, h, sandboxPath)
	fake := &fakeRuntimeProbeHost{ready: &pluginprotocol.Ready{
		Type:          "plugin.ready",
		SchemaVersion: pluginprotocol.SchemaVersion,
		PluginID:      pkg.Manifest.ExtensionID,
		PluginVersion: pkg.Manifest.Version,
		Capabilities:  []string{"test.execute"},
	}}
	var command string
	var args []string
	var manifest pluginhost.Manifest
	probe.hostFactory = func(gotCommand string, gotArgs []string, gotManifest pluginhost.Manifest) runtimeProbeHost {
		command = gotCommand
		args = append([]string(nil), gotArgs...)
		manifest = gotManifest
		return fake
	}

	result, err := probe.Probe(context.Background(), installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != RuntimeProbeStatusVerified || !result.ProbeExecutionAuthorized || !result.ProcessStarted || !result.ProcessStopped {
		t.Fatalf("runtime probe result=%+v", result)
	}
	if result.PersistentExecutionAuthorized || result.PersistentExecutionBlocker != RuntimeProbePersistentLifecycleRequired {
		t.Fatalf("persistent execution result=%+v", result)
	}
	if !fake.closed {
		t.Fatal("runtime probe process was not closed after plugin.ready")
	}
	if command != sandboxPath {
		t.Fatalf("sandbox command=%q want %q", command, sandboxPath)
	}
	joined := strings.Join(args, " ")
	for _, required := range []string{"--unshare-all", "--clearenv", "--ro-bind", "/stagecore/plugin"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("runtime probe args missing %q: %v", required, args)
		}
	}
	if manifest.PluginID != pkg.Manifest.ExtensionID || len(manifest.CapabilityPermissions) != 1 {
		t.Fatalf("runtime probe manifest=%+v", manifest)
	}
	entries, err := os.ReadDir(probe.probeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("runtime probe transient executable was not cleaned: %v", entries)
	}
	installedPath, err := h.installer.absoluteInstalledPath(installed.PayloadRelativePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != installedPayloadMode || info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("immutable installed payload mode=%#o", info.Mode().Perm())
	}
}

func TestRuntimeProbeRejectsReadyIdentityMismatchAndCleansUp(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := newRuntimeProbeForHarness(t, h, sandboxPath)
	fake := &fakeRuntimeProbeHost{ready: &pluginprotocol.Ready{
		Type:          "plugin.ready",
		SchemaVersion: pluginprotocol.SchemaVersion,
		PluginID:      pkg.Manifest.ExtensionID,
		PluginVersion: "9.9.9",
		Capabilities:  []string{"test.execute"},
	}}
	probe.hostFactory = func(string, []string, pluginhost.Manifest) runtimeProbeHost { return fake }

	_, err = probe.Probe(context.Background(), installed.InstallationID)
	if !errors.Is(err, ErrRuntimeProbeHandshake) {
		t.Fatalf("runtime probe err=%v want handshake failure", err)
	}
	if !fake.closed {
		t.Fatal("mismatched runtime probe process was not closed")
	}
	entries, readErr := os.ReadDir(probe.probeRoot)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed runtime probe leaked transient executable: %v", entries)
	}
}

func TestRuntimeProbeBlocksNetworkPermissionBeforeProcessStart(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, []string{"network.udp.send"})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probe, reviewer := newRuntimeProbeForHarness(t, h, sandboxPath)
	if _, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.send", PermissionDecisionApproved, "owner"); err != nil {
		t.Fatal(err)
	}
	started := false
	probe.hostFactory = func(string, []string, pluginhost.Manifest) runtimeProbeHost {
		started = true
		return &fakeRuntimeProbeHost{}
	}

	_, err = probe.Probe(context.Background(), installed.InstallationID)
	if !errors.Is(err, ErrRuntimeProbeNotReady) {
		t.Fatalf("runtime probe err=%v want not ready", err)
	}
	var notReady *RuntimeProbeNotReadyError
	if !errors.As(err, &notReady) || notReady.Assessment.Blocker != RuntimeIsolationBlockerNetworkBroker {
		t.Fatalf("runtime probe assessment=%+v err=%v", notReady, err)
	}
	if started {
		t.Fatal("network-blocked extension process was started")
	}
	entries, readErr := os.ReadDir(probe.probeRoot)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("network-blocked probe created transient files: %v", entries)
	}
}

func TestRuntimeProbeStartupCleanupFailsClosedOnUnexpectedEntry(t *testing.T) {
	h := newDependencyTestHarness(t)
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probe, _ := newRuntimeProbeForHarness(t, h, sandboxPath)
	stale := probe.probeRoot + string(os.PathSeparator) + "probe-stale.bin"
	if err := os.WriteFile(stale, []byte("stale"), runtimeProbeFileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntimeProbe(h.installer, probe.isolator); err != nil {
		t.Fatalf("managed stale runtime probe file was not cleaned: %v", err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale runtime probe file still exists: %v", err)
	}
	unexpected := probe.probeRoot + string(os.PathSeparator) + "do-not-touch"
	if err := os.WriteFile(unexpected, []byte("unexpected"), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntimeProbe(h.installer, probe.isolator); err == nil {
		t.Fatal("unexpected runtime probe entry should fail closed")
	}
}
