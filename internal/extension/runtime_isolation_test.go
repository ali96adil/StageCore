package extension

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeIsolationAuthorizesMinimalPrivateSandboxProbe(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	stager, reviewer := newActivationStagerForHarness(t, h)
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	isolator, err := NewRuntimeIsolator(h.installer, reviewer, stager.assessor, sandboxPath)
	if err != nil {
		t.Fatal(err)
	}

	assessment, err := isolator.Assess(context.Background(), installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != RuntimeIsolationReady || !assessment.ProbeAuthorized || assessment.Blocker != "" {
		t.Fatalf("isolation assessment=%+v", assessment)
	}
	if assessment.Engine != RuntimeIsolationEngineBubblewrap || assessment.NetworkMode != "PRIVATE_NONE" {
		t.Fatalf("isolation boundary=%+v", assessment)
	}
	if len(assessment.RuntimePermissions) != 0 {
		t.Fatalf("unexpected runtime permissions=%v", assessment.RuntimePermissions)
	}

	plan, planned, err := isolator.PlanProbe(context.Background(), installed.InstallationID, sandboxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !planned.ProbeAuthorized || plan.Command != sandboxPath {
		t.Fatalf("probe plan=%+v assessment=%+v", plan, planned)
	}
	joined := strings.Join(plan.Args, " ")
	for _, required := range []string{"--unshare-all", "--clearenv", "--ro-bind", "/stagecore/plugin", "--proc /proc", "--dev /dev", "--tmpfs /tmp"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("probe args missing %q: %v", required, plan.Args)
		}
	}
	if strings.Contains(joined, "--share-net") {
		t.Fatalf("probe must not expose host network: %v", plan.Args)
	}
}

func TestRuntimeIsolationBlocksApprovedNetworkPermissionUntilBrokerExists(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, []string{"network.udp.send"})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	stager, reviewer := newActivationStagerForHarness(t, h)
	if _, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.send", PermissionDecisionApproved, "owner"); err != nil {
		t.Fatal(err)
	}
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	isolator, err := NewRuntimeIsolator(h.installer, reviewer, stager.assessor, sandboxPath)
	if err != nil {
		t.Fatal(err)
	}

	assessment, err := isolator.Assess(context.Background(), installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != RuntimeIsolationNotReady || assessment.ProbeAuthorized || assessment.Blocker != RuntimeIsolationBlockerNetworkBroker {
		t.Fatalf("network isolation assessment=%+v", assessment)
	}
	if len(assessment.RuntimePermissions) != 1 || assessment.RuntimePermissions[0] != "network.udp.send" {
		t.Fatalf("runtime permissions=%v", assessment.RuntimePermissions)
	}
}

func TestRuntimeIsolationFailsClosedWhenSandboxEngineIsUnavailable(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	stager, reviewer := newActivationStagerForHarness(t, h)
	missing := filepath.Join(t.TempDir(), "missing-bwrap")
	isolator, err := NewRuntimeIsolator(h.installer, reviewer, stager.assessor, missing)
	if err != nil {
		t.Fatal(err)
	}

	assessment, err := isolator.Assess(context.Background(), installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != RuntimeIsolationNotReady || assessment.ProbeAuthorized || assessment.Blocker != RuntimeIsolationBlockerSandboxUnavailable {
		t.Fatalf("missing sandbox assessment=%+v", assessment)
	}
}

func TestRuntimeIsolationProbePlanRejectsRelativeExecutablePath(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, nil)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	stager, reviewer := newActivationStagerForHarness(t, h)
	sandboxPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	isolator, err := NewRuntimeIsolator(h.installer, reviewer, stager.assessor, sandboxPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := isolator.PlanProbe(context.Background(), installed.InstallationID, "relative/plugin"); err == nil {
		t.Fatal("relative isolated runtime executable path should be rejected")
	}
}
