package extension

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/pluginhost"
	"github.com/ali96adil/StageCore/internal/store"
)

func brokeredRuntimeHarness(t *testing.T) (*dependencyTestHarness, Package, Installation, *PermissionReviewer, *RuntimeNetworkBroker, *RuntimeIsolator) {
	t.Helper()
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, []string{RuntimeNetworkBrokerPermissionUDPSend})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	stager, reviewer := newActivationStagerForHarness(t, h)
	if _, err := reviewer.Decide(h.ctx, installed.InstallationID, RuntimeNetworkBrokerPermissionUDPSend, PermissionDecisionApproved, "owner"); err != nil {
		t.Fatal(err)
	}
	broker, err := NewRuntimeNetworkBroker(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	isolator, err := NewRuntimeIsolator(h.installer, reviewer, stager.assessor, sandboxPath, WithRuntimeNetworkBroker(broker))
	if err != nil {
		t.Fatal(err)
	}
	return h, pkg, installed, reviewer, broker, isolator
}

func TestRuntimeIsolationAllowsBrokeredUDPSendWithoutHostNetwork(t *testing.T) {
	h, _, installed, _, broker, isolator := brokeredRuntimeHarness(t)
	assessment, err := isolator.Assess(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != RuntimeIsolationReady || !assessment.ProbeAuthorized || assessment.Blocker != "" {
		t.Fatalf("brokered isolation assessment=%+v", assessment)
	}
	if assessment.NetworkMode != RuntimeIsolationNetworkModeBrokeredUDP {
		t.Fatalf("network mode=%q", assessment.NetworkMode)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	plan, planned, err := isolator.PlanProbe(context.Background(), installed.InstallationID, executable)
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if !planned.ProbeAuthorized || plan.broker == nil {
		t.Fatalf("brokered plan=%+v assessment=%+v", plan, planned)
	}
	joined := strings.Join(plan.Args, " ")
	for _, required := range []string{
		"--unshare-all",
		"--ro-bind " + plan.broker.HostDirectory() + " " + RuntimeNetworkBrokerSandboxDirectory,
		"--setenv " + RuntimeNetworkBrokerSocketEnv + " " + RuntimeNetworkBrokerSandboxSocket,
		"--setenv " + RuntimeNetworkBrokerTokenEnv + " " + plan.broker.Token(),
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("brokered sandbox args missing %q: %v", required, plan.Args)
		}
	}
	if strings.Contains(joined, "--share-net") {
		t.Fatalf("brokered plugin must not receive host network namespace: %v", plan.Args)
	}
	entries, err := os.ReadDir(broker.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("broker plan sessions=%d want 1", len(entries))
	}
}

func TestRuntimeIsolationKeepsUDPListenFailClosedWithBrokerPresent(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, []string{"network.udp.listen"})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	stager, reviewer := newActivationStagerForHarness(t, h)
	if _, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.listen", PermissionDecisionApproved, "owner"); err != nil {
		t.Fatal(err)
	}
	broker, err := NewRuntimeNetworkBroker(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	isolator, err := NewRuntimeIsolator(h.installer, reviewer, stager.assessor, sandboxPath, WithRuntimeNetworkBroker(broker))
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := isolator.Assess(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != RuntimeIsolationNotReady || assessment.ProbeAuthorized || assessment.Blocker != RuntimeIsolationBlockerNetworkBroker {
		t.Fatalf("udp.listen assessment=%+v", assessment)
	}
}

func TestRuntimeProbeBrokerSessionIsBoundedAndCleaned(t *testing.T) {
	h, pkg, installed, _, broker, isolator := brokeredRuntimeHarness(t)
	probe, err := NewRuntimeProbe(h.installer, isolator)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	probe.hostFactory = func(_ string, gotArgs []string, _ pluginhost.Manifest) runtimeProbeHost {
		args = append([]string(nil), gotArgs...)
		return &fakeRuntimeProbeHost{ready: validLifecycleReady(pkg)}
	}
	result, err := probe.Probe(context.Background(), installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if result.NetworkMode != RuntimeIsolationNetworkModeBrokeredUDP || !result.ProbeExecutionAuthorized {
		t.Fatalf("brokered probe result=%+v", result)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, RuntimeNetworkBrokerSocketEnv) || !strings.Contains(joined, RuntimeNetworkBrokerSandboxSocket) {
		t.Fatalf("brokered probe args=%v", args)
	}
	entries, err := os.ReadDir(broker.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("bounded runtime probe leaked broker session: %v", entries)
	}
}

func TestRuntimeSupervisorOwnsBrokerSessionForEnabledGeneration(t *testing.T) {
	h, pkg, installed, _, broker, isolator := brokeredRuntimeHarness(t)
	probe, err := NewRuntimeProbe(h.installer, isolator)
	if err != nil {
		t.Fatal(err)
	}
	probe.hostFactory = func(string, []string, pluginhost.Manifest) runtimeProbeHost {
		return &fakeRuntimeProbeHost{ready: validLifecycleReady(pkg)}
	}
	supervisor, err := NewRuntimeSupervisor(h.installer, isolator, probe)
	if err != nil {
		t.Fatal(err)
	}
	defer supervisor.Close()
	supervisor.hostFactory = func(string, []string, pluginhost.Manifest) runtimeLifecycleHost {
		return newFakeRuntimeLifecycleHost(validLifecycleReady(pkg))
	}

	status, err := supervisor.Enable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredEnabled || status.ObservedState != store.ExtensionRuntimeObservedReady {
		t.Fatalf("brokered enabled status=%+v", status)
	}
	entries, err := os.ReadDir(broker.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("persistent broker sessions=%d want 1", len(entries))
	}

	status, err = supervisor.Disable(h.ctx, installed.InstallationID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredState != store.ExtensionRuntimeDesiredDisabled || status.ObservedState != store.ExtensionRuntimeObservedStopped {
		t.Fatalf("brokered disabled status=%+v", status)
	}
	entries, err = os.ReadDir(broker.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("disable leaked broker sessions: %v", entries)
	}
}
