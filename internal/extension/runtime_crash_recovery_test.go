package extension

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/store"
)

type crashRecoveryHostSequence struct {
	mu    sync.Mutex
	pkg   Package
	hosts []*fakeRuntimeLifecycleHost
}

func newCrashRecoveryHostSequence(pkg Package) *crashRecoveryHostSequence {
	return &crashRecoveryHostSequence{pkg: pkg}
}

func (s *crashRecoveryHostSequence) factory(string, []string, pluginhost.Manifest) runtimeLifecycleHost {
	s.mu.Lock()
	defer s.mu.Unlock()
	host := newFakeRuntimeLifecycleHost(validLifecycleReady(s.pkg))
	s.hosts = append(s.hosts, host)
	return host
}

func (s *crashRecoveryHostSequence) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.hosts)
}

func (s *crashRecoveryHostSequence) host(index int) *fakeRuntimeLifecycleHost {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.hosts) {
		return nil
	}
	return s.hosts[index]
}

func waitForRuntimeLifecycle(t *testing.T, supervisor *RuntimeSupervisor, installationID string, timeout time.Duration, match func(RuntimeLifecycleStatus) bool) RuntimeLifecycleStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last RuntimeLifecycleStatus
	for {
		status, err := supervisor.Status(t.Context(), installationID)
		if err != nil {
			t.Fatal(err)
		}
		last = status
		if match(status) {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime lifecycle condition not reached; last=%+v", last)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForProbeCalls(t *testing.T, probe *scriptedRuntimeLifecycleProbe, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for probe.Calls() < want {
		if time.Now().After(deadline) {
			t.Fatalf("probe calls=%d want at least %d", probe.Calls(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRuntimeSupervisorCrashRecoveryRetriesTransientHandshake(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	defer supervisor.Close()
	sequence := newCrashRecoveryHostSequence(pkg)
	supervisor.hostFactory = sequence.factory
	supervisor.EnableAutomaticCrashRecovery()

	initial, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	generation := initial.Generation
	probe := &scriptedRuntimeLifecycleProbe{errs: []error{
		fmt.Errorf("%w: synthetic crash recovery handshake failure", ErrRuntimeProbeHandshake),
		nil,
	}}
	supervisor.probe = probe
	first := sequence.host(0)
	if first == nil {
		t.Fatal("initial runtime host was not created")
	}
	first.crash(errors.New("synthetic runtime crash"))

	status := waitForRuntimeLifecycle(t, supervisor, installed.InstallationID, 3*time.Second, func(status RuntimeLifecycleStatus) bool {
		return status.ObservedState == store.ExtensionRuntimeObservedReady && status.PluginReady != nil && sequence.count() >= 2
	})
	if probe.Calls() != 2 {
		t.Fatalf("probe calls=%d want 2", probe.Calls())
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredEnabled || status.Generation != generation {
		t.Fatalf("recovered status=%+v", status)
	}
}

func TestRuntimeSupervisorCrashRecoveryFailsFastOnIntegrityError(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	defer supervisor.Close()
	sequence := newCrashRecoveryHostSequence(pkg)
	supervisor.hostFactory = sequence.factory
	supervisor.EnableAutomaticCrashRecovery()
	if _, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner"); err != nil {
		t.Fatal(err)
	}

	probe := &scriptedRuntimeLifecycleProbe{errs: []error{
		fmt.Errorf("%w: synthetic crash recovery integrity failure", ErrRuntimeProbeIntegrity),
		nil,
	}}
	supervisor.probe = probe
	sequence.host(0).crash(errors.New("synthetic runtime crash"))
	waitForProbeCalls(t, probe, 1)
	status := waitForRuntimeLifecycle(t, supervisor, installed.InstallationID, time.Second, func(status RuntimeLifecycleStatus) bool {
		return status.ObservedState == store.ExtensionRuntimeObservedFailed && status.LastErrorCode == RuntimeLifecycleErrorProbeFailed
	})
	time.Sleep(350 * time.Millisecond)
	if probe.Calls() != 1 {
		t.Fatalf("integrity failure probe calls=%d want 1", probe.Calls())
	}
	if sequence.count() != 1 {
		t.Fatalf("persistent runtime hosts=%d want 1", sequence.count())
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredEnabled {
		t.Fatalf("failed recovery changed operator intent: %+v", status)
	}
}

func TestRuntimeSupervisorDisableCancelsCrashRecoveryBackoff(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	defer supervisor.Close()
	sequence := newCrashRecoveryHostSequence(pkg)
	supervisor.hostFactory = sequence.factory
	supervisor.EnableAutomaticCrashRecovery()
	initial, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}

	probe := &scriptedRuntimeLifecycleProbe{errs: []error{
		fmt.Errorf("%w: synthetic crash recovery handshake failure", ErrRuntimeProbeHandshake),
		nil,
	}}
	supervisor.probe = probe
	sequence.host(0).crash(errors.New("synthetic runtime crash"))
	waitForProbeCalls(t, probe, 1)

	disabled, err := supervisor.Disable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if disabled.DesiredState != store.ExtensionRuntimeDesiredDisabled || disabled.ObservedState != store.ExtensionRuntimeObservedStopped || disabled.Generation != initial.Generation+1 {
		t.Fatalf("disabled status=%+v", disabled)
	}
	time.Sleep(350 * time.Millisecond)
	if probe.Calls() != 1 {
		t.Fatalf("probe calls after disable=%d want 1", probe.Calls())
	}
	if sequence.count() != 1 {
		t.Fatalf("runtime restarted after disable; hosts=%d", sequence.count())
	}
}

func TestRuntimeSupervisorCloseCancelsCrashRecoveryBackoff(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	sequence := newCrashRecoveryHostSequence(pkg)
	supervisor.hostFactory = sequence.factory
	supervisor.EnableAutomaticCrashRecovery()
	if _, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner"); err != nil {
		t.Fatal(err)
	}

	probe := &scriptedRuntimeLifecycleProbe{errs: []error{
		fmt.Errorf("%w: synthetic crash recovery handshake failure", ErrRuntimeProbeHandshake),
		nil,
	}}
	supervisor.probe = probe
	sequence.host(0).crash(errors.New("synthetic runtime crash"))
	waitForProbeCalls(t, probe, 1)
	if err := supervisor.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(350 * time.Millisecond)
	if probe.Calls() != 1 {
		t.Fatalf("probe calls after close=%d want 1", probe.Calls())
	}
	if sequence.count() != 1 {
		t.Fatalf("runtime restarted after close; hosts=%d", sequence.count())
	}
}

func TestRuntimeSupervisorCrashRecoveryBudgetStopsRestartStorm(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	supervisor, _, _, _ := newRuntimeSupervisorForHarness(t, h, pkg)
	defer supervisor.Close()
	sequence := newCrashRecoveryHostSequence(pkg)
	supervisor.hostFactory = sequence.factory
	supervisor.EnableAutomaticCrashRecovery()
	initial, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}

	for crash := 0; crash < maxExtensionCrashRecoveryAttempts; crash++ {
		host := sequence.host(crash)
		if host == nil {
			t.Fatalf("runtime host %d missing", crash)
		}
		host.crash(fmt.Errorf("synthetic crash %d", crash+1))
		wantHosts := crash + 2
		waitForRuntimeLifecycle(t, supervisor, installed.InstallationID, 2*time.Second, func(status RuntimeLifecycleStatus) bool {
			return status.ObservedState == store.ExtensionRuntimeObservedReady && status.PluginReady != nil && sequence.count() >= wantHosts
		})
	}

	last := sequence.host(maxExtensionCrashRecoveryAttempts)
	if last == nil {
		t.Fatal("last recovered runtime host missing")
	}
	last.crash(errors.New("synthetic crash after recovery budget exhausted"))
	status := waitForRuntimeLifecycle(t, supervisor, installed.InstallationID, time.Second, func(status RuntimeLifecycleStatus) bool {
		return status.ObservedState == store.ExtensionRuntimeObservedFailed && status.LastErrorCode == RuntimeLifecycleErrorCrashRecoveryExhausted
	})
	if sequence.count() != maxExtensionCrashRecoveryAttempts+1 {
		t.Fatalf("restart storm exceeded budget; hosts=%d", sequence.count())
	}
	if status.Generation != initial.Generation || status.DesiredState != store.ExtensionRuntimeDesiredEnabled {
		t.Fatalf("budget exhaustion changed lifecycle authority: %+v", status)
	}
}

func TestReconcileBoundedEnablesOngoingCrashRecovery(t *testing.T) {
	h, pkg, installed, probe, generation := enabledStoppedRuntimeForRecovery(t)
	second, err := NewRuntimeSupervisor(h.installer, probe.isolator, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	sequence := newCrashRecoveryHostSequence(pkg)
	second.hostFactory = sequence.factory

	if err := second.ReconcileBounded(h.ctx); err != nil {
		t.Fatal(err)
	}
	if sequence.count() != 1 {
		t.Fatalf("startup restored hosts=%d want 1", sequence.count())
	}
	sequence.host(0).crash(errors.New("synthetic post-startup runtime crash"))
	status := waitForRuntimeLifecycle(t, second, installed.InstallationID, 2*time.Second, func(status RuntimeLifecycleStatus) bool {
		return status.ObservedState == store.ExtensionRuntimeObservedReady && status.PluginReady != nil && sequence.count() >= 2
	})
	if status.Generation != generation || status.DesiredState != store.ExtensionRuntimeDesiredEnabled {
		t.Fatalf("post-startup recovery status=%+v", status)
	}
}
