package extension

import (
	"bytes"
	"debug/elf"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
)

func readinessCheckByID(t *testing.T, assessment ReadinessAssessment, id string) ReadinessCheck {
	t.Helper()
	for _, check := range assessment.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("readiness check %q missing from %+v", id, assessment.Checks)
	return ReadinessCheck{}
}

func newReadinessAssessorForHarness(t *testing.T, h *dependencyTestHarness) *ReadinessAssessor {
	t.Helper()
	reviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	assessor, err := NewReadinessAssessor(h.installer, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	return assessor
}

func registerReadinessPackage(t *testing.T, h *dependencyTestHarness, extensionID string, permissions []string, dependencies []Dependency, productionReady, withRuntime bool) Package {
	t.Helper()
	signing := store.SoftwareSigningSigned
	channel := store.SoftwareChannelRelease
	if !productionReady {
		signing = store.SoftwareSigningUnsigned
		channel = store.SoftwareChannelDevelopment
	}
	softwarePackage, err := h.repository.ImportPackage(h.ctx, software.ImportParams{
		ProductID: extensionID,
		Version: "1.0.0",
		Platform: "linux",
		Architecture: "arm64",
		MinAPIVersion: 1,
		MaxAPIVersion: 1,
		OriginalFilename: extensionID + "-1.0.0",
		SigningStatus: signing,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: channel,
	}, bytes.NewReader(minimalELF64(elf.EM_AARCH64)))
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: extensionID,
		Version: "1.0.0",
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: extensionID, ArIQ: "إضافة " + extensionID},
		Summary: LocalizedText{EN: "Readiness test extension.", ArIQ: "إضافة تجريبية لاختبار الجاهزية."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Permissions: permissions,
		Dependencies: dependencies,
	}
	if withRuntime {
		manifest.Capabilities = []string{"test.execute"}
		manifest.Runtime = testPluginRuntime(permissions)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := h.library.Register(h.ctx, softwarePackage.ID, raw, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestReadinessReadyOnlyAfterPermissionApproval(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerReadinessPackage(t, h, "ready.permission-plugin", []string{"network.udp.send"}, nil, true, true)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	assessor := newReadinessAssessorForHarness(t, h)

	pending, err := assessor.Assess(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != ReadinessNotReady {
		t.Fatalf("pending readiness=%s want NOT_READY", pending.Status)
	}
	permissionCheck := readinessCheckByID(t, pending, "permission_review")
	if permissionCheck.Status != ReadinessCheckBlocked || permissionCheck.Code != "PERMISSION_REVIEW_PENDING" {
		t.Fatalf("permission check=%+v", permissionCheck)
	}

	reviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewer.Decide(h.ctx, installed.InstallationID, "network.udp.send", PermissionDecisionApproved, "owner"); err != nil {
		t.Fatal(err)
	}
	ready, err := assessor.Assess(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != ReadinessReadyForActivation {
		t.Fatalf("ready status=%s checks=%+v", ready.Status, ready.Checks)
	}
	for _, id := range []string{"installed_integrity", "package_compatibility", "package_trust", "runtime_artifact", "dependencies", "permission_review"} {
		if check := readinessCheckByID(t, ready, id); check.Status != ReadinessCheckPass {
			t.Fatalf("%s check=%+v", id, check)
		}
	}
	runtime := readinessCheckByID(t, ready, "runtime_health")
	if runtime.Status != ReadinessCheckNotApplicable || runtime.Code != "ACTIVATION_NOT_IMPLEMENTED" {
		t.Fatalf("runtime check=%+v", runtime)
	}
}

func TestReadinessBlocksPluginWithoutRuntimeContract(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerReadinessPackage(t, h, "ready.legacy-plugin", nil, nil, true, false)
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := newReadinessAssessorForHarness(t, h).Assess(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != ReadinessNotReady {
		t.Fatalf("legacy plugin readiness=%s", assessment.Status)
	}
	check := readinessCheckByID(t, assessment, "runtime_artifact")
	if check.Status != ReadinessCheckBlocked || check.Code != "RUNTIME_CONTRACT_MISSING" {
		t.Fatalf("runtime artifact check=%+v", check)
	}
}

func TestReadinessBlocksMissingRequiredDependencyThenRecovers(t *testing.T) {
	h := newDependencyTestHarness(t)
	dependency := registerReadinessPackage(t, h, "ready.required-dependency", nil, nil, true, true)
	root := registerReadinessPackage(t, h, "ready.root-plugin", nil, []Dependency{{ExtensionID: dependency.Manifest.ExtensionID, MinVersion: "1.0.0"}}, true, true)

	installedRoot, err := h.installer.Install(h.ctx, root.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	assessor := newReadinessAssessorForHarness(t, h)
	blocked, err := assessor.Assess(h.ctx, installedRoot.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status != ReadinessNotReady {
		t.Fatalf("blocked status=%s", blocked.Status)
	}
	dependencyCheck := readinessCheckByID(t, blocked, "dependencies")
	if dependencyCheck.Status != ReadinessCheckBlocked || dependencyCheck.Code != "REQUIRED_DEPENDENCIES_MISSING" {
		t.Fatalf("dependency check=%+v", dependencyCheck)
	}

	if _, err := h.installer.InstallPlanned(h.ctx, dependency.PackageID, "owner"); err != nil {
		t.Fatal(err)
	}
	ready, err := assessor.Assess(h.ctx, installedRoot.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != ReadinessReadyForActivation {
		t.Fatalf("recovered status=%s checks=%+v", ready.Status, ready.Checks)
	}
}

func TestReadinessKeepsOptionalDependencyAsAdvisory(t *testing.T) {
	h := newDependencyTestHarness(t)
	_ = registerReadinessPackage(t, h, "ready.optional-dependency", nil, nil, true, true)
	root := registerReadinessPackage(t, h, "ready.optional-root", nil, []Dependency{{ExtensionID: "ready.optional-dependency", MinVersion: "1.0.0", Optional: true}}, true, true)
	installed, err := h.installer.InstallPlanned(h.ctx, root.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := newReadinessAssessorForHarness(t, h).Assess(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != ReadinessReadyForActivation {
		t.Fatalf("optional dependency blocked readiness: %+v", assessment)
	}
	if len(assessment.Advisories) != 1 || assessment.Advisories[0].ExtensionID != "ready.optional-dependency" {
		t.Fatalf("advisories=%+v", assessment.Advisories)
	}
}

func TestReadinessBlocksNonProductionPackageAndDetectsTamper(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerReadinessPackage(t, h, "ready.local-development", nil, nil, false, true)
	installed, err := h.installer.Install(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	assessor := newReadinessAssessorForHarness(t, h)
	assessment, err := assessor.Assess(h.ctx, installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Status != ReadinessNotReady {
		t.Fatalf("development package readiness=%s", assessment.Status)
	}
	trustCheck := readinessCheckByID(t, assessment, "package_trust")
	if trustCheck.Status != ReadinessCheckBlocked || trustCheck.Code != "PACKAGE_NOT_PRODUCTION_READY" {
		t.Fatalf("trust check=%+v", trustCheck)
	}

	installedPath := filepath.Join(h.installer.installedRoot, filepath.FromSlash(installed.PayloadRelativePath))
	if err := os.Chmod(installedPath, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := assessor.Assess(h.ctx, installed.InstallationID); !errors.Is(err, ErrInstalledPayloadIntegrity) {
		t.Fatalf("tampered readiness err=%v", err)
	}
}
