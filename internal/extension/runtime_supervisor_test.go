package extension

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/pluginprotocol"
	"github.com/ali96adil/StageCore/internal/store"
)

type fakeRuntimeLifecycleHost struct {
	ready  *pluginprotocol.Ready
	probeErr error
	exitCh chan error
	once   sync.Once
}

func newFakeRuntimeLifecycleHost(ready *pluginprotocol.Ready) *fakeRuntimeLifecycleHost {
	return &fakeRuntimeLifecycleHost{ready: ready, exitCh: make(chan error, 1)}
}

func (h *fakeRuntimeLifecycleHost) Probe(context.Context) (*pluginprotocol.Ready, error) {
	return h.ready, h.probeErr
}

func (h *fakeRuntimeLifecycleHost) Wait() error {
	err, ok := <-h.exitCh
	if !ok {
		return nil
	}
	return err
}

func (h *fakeRuntimeLifecycleHost) Close() {
	h.once.Do(func() { close(h.exitCh) })
}

func (h *fakeRuntimeLifecycleHost) crash(err error) {
	h.once.Do(func() {
		h.exitCh <- err
		close(h.exitCh)
	})
}

func lifecycleReady(pkg Package) *pluginprotocol.Ready {
	return &pluginprotocol.Ready{
		Type: pluginprotocol.Ready{}.Type,
		SchemaVersion: pluginprotocol.SchemaVersion,
		PluginID: pkg.Manifest.ExtensionID,
		PluginVersion: pkg.Manifest.Version,
		Capabilities: append([]string(nil), pkg.Manifest.Capabilities...),
	}
}

func validLifecycleReady(pkg Package) *pluginprotocol.Ready {
	ready := lifecycleReady(pkg)
	ready.Type = "plugin.ready"
	return ready
}

func newRuntimeSupervisorForHarness(t *testing.T, h *dependencyTestHarness, pkg Package) (*RuntimeSupervisor, *RuntimeProbe, *PermissionReviewer, *[]*fakeRuntimeLifecycleHost) {
	t.Helper()
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probe, reviewer := newRuntimeProbeForHarness(t, h, sandboxPath)
	probe.hostFactory = func(string, []string, pluginhost.Manifest) runtimeProbeHost {
		return &fakeRuntimeProbeHost{ready: validLifecycleReady(pkg)}
	}
	supervisor, err := NewRuntimeSupervisor(h.installer, probe.isolator, probe)
	if err != nil {
		t.Fatal(err)
	}
	hosts := make([]*fakeRuntimeLifecycleHost, 0)
	supervisor.hostFactory = func(string, []string, pluginhost.Manifest) runtimeLifecycleHost {
		host := newFakeRuntimeLifecycleHost(validLifecycleReady(pkg))
		hosts = append(hosts, host)
		return host
	}
	return supervisor, probe, reviewer, &hosts
}

func TestRuntimeSupervisorPersistsIntentSupervisesCrashAndUsesGenerationCAS(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, hosts := newRuntimeSupervisorForHarness(t, h, pkg)
	defer supervisor.Close()

	status, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredEnabled || status.ObservedState != store.ExtensionRuntimeObservedReady || status.Generation != 1 || status.PluginReady == nil {
		t.Fatalf("enabled status=%+v", status)
	}
	if len(*hosts) != 1 {
		t.Fatalf("persistent hosts=%d want 1", len(*hosts))
	}
	entries, err := os.ReadDir(supervisor.activeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("active runtime copies=%v", entries)
	}

	(*hosts)[0].crash(errors.New("synthetic runtime crash"))
	deadline := time.Now().Add(time.Second)
	for {
		status, err = supervisor.Status(h.ctx, installed.InstallationID)
		if err != nil {
			t.Fatal(err)
		}
		if status.ObservedState == store.ExtensionRuntimeObservedFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime crash was not observed: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredEnabled || status.Generation != 1 || status.LastErrorCode != RuntimeLifecycleErrorProcessExited {
		t.Fatalf("crashed status=%+v", status)
	}
	entries, err = os.ReadDir(supervisor.activeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("crashed runtime copy leaked: %v", entries)
	}

	status, err = supervisor.Disable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredDisabled || status.ObservedState != store.ExtensionRuntimeObservedStopped || status.Generation != 2 {
		t.Fatalf("disabled status=%+v", status)
	}
	updated, err := h.installer.library.store.UpdateExtensionRuntimeObservedState(h.ctx, installed.InstallationID, 1, store.ExtensionRuntimeObservedFailed, "STALE", "stale generation")
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("stale generation unexpectedly overwrote current runtime state")
	}
	status, err = supervisor.Status(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if status.ObservedState != store.ExtensionRuntimeObservedStopped || status.Generation != 2 {
		t.Fatalf("stale write changed status=%+v", status)
	}
}

func TestRuntimeSupervisorReconcilesEnabledIntentAfterHubRestart(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	first, probe, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	status, err := first.Enable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	generation := status.Generation
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	stopped, err := first.Status(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.DesiredState != store.ExtensionRuntimeDesiredEnabled || stopped.ObservedState != store.ExtensionRuntimeObservedStopped || stopped.Generation != generation {
		t.Fatalf("shutdown status=%+v", stopped)
	}

	second, err := NewRuntimeSupervisor(h.installer, probe.isolator, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	restoredHost := newFakeRuntimeLifecycleHost(validLifecycleReady(pkg))
	second.hostFactory = func(string, []string, pluginhost.Manifest) runtimeLifecycleHost { return restoredHost }
	if err := second.Reconcile(h.ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := second.Status(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.DesiredState != store.ExtensionRuntimeDesiredEnabled || restored.ObservedState != store.ExtensionRuntimeObservedReady || restored.Generation != generation || restored.PluginReady == nil {
		t.Fatalf("restored status=%+v", restored)
	}
}

func TestRuntimeSupervisorNetworkBlockerDoesNotPersistEnabledIntent(t *testing.T) {
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
	supervisor, err := NewRuntimeSupervisor(h.installer, probe.isolator, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer supervisor.Close()
	started := false
	supervisor.hostFactory = func(string, []string, pluginhost.Manifest) runtimeLifecycleHost {
		started = true
		return newFakeRuntimeLifecycleHost(validLifecycleReady(pkg))
	}

	_, err = supervisor.Enable(h.ctx, installed.InstallationID, "owner")
	if !errors.Is(err, ErrRuntimeProbeNotReady) {
		t.Fatalf("enable err=%v want runtime probe not ready", err)
	}
	if started {
		t.Fatal("network-blocked persistent process was started")
	}
	status, err := supervisor.Status(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredDisabled || status.ObservedState != store.ExtensionRuntimeObservedStopped || status.Generation != 0 {
		t.Fatalf("network blocker left misleading intent=%+v", status)
	}
}
