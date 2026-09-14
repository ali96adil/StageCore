package extension

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/recovery"
	"github.com/ali96adil/StageCore/internal/store"
)

type scriptedRuntimeLifecycleProbe struct {
	mu    sync.Mutex
	errs  []error
	calls int
}

func (p *scriptedRuntimeLifecycleProbe) Probe(context.Context, string) (RuntimeProbeResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	index := p.calls - 1
	if index < len(p.errs) && p.errs[index] != nil {
		return RuntimeProbeResult{}, p.errs[index]
	}
	return RuntimeProbeResult{}, nil
}

func (p *scriptedRuntimeLifecycleProbe) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func zeroBackoffRecoveryPolicy() recovery.BackoffPolicy {
	return recovery.BackoffPolicy{MaxAttempts: 3, InitialBackoff: 0, MaxBackoff: 0}
}

func enabledStoppedRuntimeForRecovery(t *testing.T) (*dependencyTestHarness, Package, Installation, *RuntimeProbe, int64) {
	t.Helper()
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
	if stopped.DesiredState != store.ExtensionRuntimeDesiredEnabled || stopped.ObservedState != store.ExtensionRuntimeObservedStopped {
		t.Fatalf("prepared runtime status=%+v", stopped)
	}
	return h, pkg, installed, probe, generation
}

func TestRuntimeSupervisorBoundedStartupRecoveryRetriesTransientHandshake(t *testing.T) {
	h, pkg, installed, probe, generation := enabledStoppedRuntimeForRecovery(t)
	scripted := &scriptedRuntimeLifecycleProbe{errs: []error{
		fmt.Errorf("%w: synthetic startup handshake interruption", ErrRuntimeProbeHandshake),
		nil,
	}}
	second, err := NewRuntimeSupervisor(h.installer, probe.isolator, scripted)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.hostFactory = func(string, []string, pluginhost.Manifest) runtimeLifecycleHost {
		return newFakeRuntimeLifecycleHost(validLifecycleReady(pkg))
	}

	if err := second.reconcileBounded(h.ctx, zeroBackoffRecoveryPolicy()); err != nil {
		t.Fatal(err)
	}
	if scripted.Calls() != 2 {
		t.Fatalf("probe calls=%d want 2", scripted.Calls())
	}
	status, err := second.Status(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredEnabled || status.ObservedState != store.ExtensionRuntimeObservedReady || status.Generation != generation || status.PluginReady == nil {
		t.Fatalf("recovered runtime status=%+v", status)
	}
}

func TestRuntimeSupervisorBoundedStartupRecoveryFailsFastOnIntegrityError(t *testing.T) {
	h, _, installed, probe, generation := enabledStoppedRuntimeForRecovery(t)
	scripted := &scriptedRuntimeLifecycleProbe{errs: []error{
		fmt.Errorf("%w: synthetic runtime hash mismatch", ErrRuntimeProbeIntegrity),
		nil,
	}}
	second, err := NewRuntimeSupervisor(h.installer, probe.isolator, scripted)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	started := 0
	second.hostFactory = func(string, []string, pluginhost.Manifest) runtimeLifecycleHost {
		started++
		return nil
	}

	err = second.reconcileBounded(h.ctx, zeroBackoffRecoveryPolicy())
	if !errors.Is(err, ErrRuntimeProbeIntegrity) {
		t.Fatalf("reconcile err=%v want runtime probe integrity", err)
	}
	if scripted.Calls() != 1 {
		t.Fatalf("probe calls=%d want 1", scripted.Calls())
	}
	if started != 0 {
		t.Fatalf("persistent runtime starts=%d want 0", started)
	}
	status, statusErr := second.Status(h.ctx, installed.InstallationID)
	if statusErr != nil {
		t.Fatal(statusErr)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredEnabled || status.ObservedState != store.ExtensionRuntimeObservedFailed || status.Generation != generation || status.LastErrorCode != RuntimeLifecycleErrorProbeFailed {
		t.Fatalf("failed runtime status=%+v", status)
	}
}

func TestExtensionStartupRecoveryClassifierRejectsMixedPermanentFailure(t *testing.T) {
	transient := fmt.Errorf("restore one: %w", ErrRuntimeProbeHandshake)
	permanent := fmt.Errorf("restore two: %w", ErrRuntimeProbeIntegrity)
	if !isTransientExtensionStartupReconcileError(transient) {
		t.Fatal("single handshake failure should be retryable")
	}
	if isTransientExtensionStartupReconcileError(errors.Join(transient, permanent)) {
		t.Fatal("mixed transient/permanent reconciliation must fail closed")
	}
}
