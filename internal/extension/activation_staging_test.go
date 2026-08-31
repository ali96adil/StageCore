package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ali96adil/StageCore/internal/pluginpermissions"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
)

func registerActivationStagingPackage(t *testing.T, h *dependencyTestHarness, permissions []string) Package {
	t.Helper()
	machine := readinessHostMachine(t)
	softwarePackage, err := h.repository.ImportPackage(h.ctx, software.ImportParams{
		ProductID: "stagecore.staging-test",
		Version: "1.0.0",
		Platform: "linux",
		Architecture: runtime.GOARCH,
		MinAPIVersion: 1,
		MaxAPIVersion: 1,
		OriginalFilename: "stagecore-staging-test",
		SigningStatus: store.SoftwareSigningSigned,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader(minimalELF64(machine)))
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		ExtensionID: "stagecore.staging-test",
		Version: "1.0.0",
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: "Activation Staging Test", ArIQ: "اختبار تجهيز التفعيل"},
		Summary: LocalizedText{EN: "Exercises safe activation staging.", ArIQ: "يختبر تجهيز التفعيل الآمن."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{runtime.GOARCH}},
		Permissions: append([]string(nil), permissions...),
		Capabilities: []string{"test.execute"},
		Runtime: &RuntimeContract{
			Protocol: RuntimeProtocolPluginV1,
			Artifact: RuntimeArtifactNativeExecutable,
			CapabilityPermissions: map[string][]string{"test.execute": append([]string(nil), permissions...)},
		},
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

func newActivationStagerForHarness(t *testing.T, h *dependencyTestHarness) (*ActivationStager, *PermissionReviewer) {
	t.Helper()
	reviewer, err := NewPermissionReviewer(h.installer)
	if err != nil {
		t.Fatal(err)
	}
	assessor, err := NewReadinessAssessor(h.installer, reviewer)
	if err != nil {
		t.Fatal(err)
	}
	stager, err := NewActivationStager(h.installer, reviewer, assessor)
	if err != nil {
		t.Fatal(err)
	}
	return stager, reviewer
}

func TestActivationStagingVerifiesReadOnlyCopyAndCleansUp(t *testing.T) {
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

	result, err := stager.Check(context.Background(), installed.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "STAGING_VERIFIED" || result.ExecutionAuthorized || result.ExecutionBlocker != ActivationExecutionIsolationRequired {
		t.Fatalf("staging result=%+v", result)
	}
	if result.Platform != "linux" || result.Architecture != runtime.GOARCH {
		t.Fatalf("staging host=%s/%s want linux/%s", result.Platform, result.Architecture, runtime.GOARCH)
	}
	if len(result.RuntimePermissions) != 1 || result.RuntimePermissions[0] != "network.udp.send" {
		t.Fatalf("runtime permissions=%v", result.RuntimePermissions)
	}
	entries, err := os.ReadDir(stager.stageRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("activation staging files were not cleaned up: %v", entries)
	}
	installedPath := filepath.Join(h.installer.installedRoot, filepath.FromSlash(installed.PayloadRelativePath))
	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != installedPayloadMode || info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("immutable installed payload mode=%#o", info.Mode().Perm())
	}
	permissionService, err := pluginpermissions.New(h.handle.DB)
	if err != nil {
		t.Fatal(err)
	}
	granted, err := permissionService.Granted(h.ctx, pkg.Manifest.ExtensionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(granted) != 0 {
		t.Fatalf("activation staging leaked durable runtime grants: %v", granted)
	}
}

func TestActivationStagingRequiresReadinessBeforeCreatingCopy(t *testing.T) {
	h := newDependencyTestHarness(t)
	pkg := registerActivationStagingPackage(t, h, []string{"network.udp.send"})
	installed, err := h.installer.InstallPlanned(h.ctx, pkg.PackageID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	stager, _ := newActivationStagerForHarness(t, h)
	_, err = stager.Check(context.Background(), installed.InstallationID)
	if !errors.Is(err, ErrActivationNotReady) {
		t.Fatalf("staging err=%v want activation not ready", err)
	}
	var notReady *ActivationNotReadyError
	if !errors.As(err, &notReady) || notReady.Assessment.Status != ReadinessNotReady {
		t.Fatalf("activation not-ready assessment=%+v err=%v", notReady, err)
	}
	entries, readErr := os.ReadDir(stager.stageRoot)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("not-ready staging created runtime files: %v", entries)
	}
}

func TestActivationStagerCleansOnlyManagedStaleFilesAndFailsClosedOnUnexpectedEntries(t *testing.T) {
	h := newDependencyTestHarness(t)
	stager, _ := newActivationStagerForHarness(t, h)
	stale := filepath.Join(stager.stageRoot, "stage-stale.bin")
	if err := os.WriteFile(stale, []byte("stale"), activationStagingFileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := NewActivationStager(h.installer, stager.reviewer, stager.assessor); err != nil {
		t.Fatalf("managed stale staging file was not cleaned: %v", err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staging file still exists: %v", err)
	}
	unexpected := filepath.Join(stager.stageRoot, "do-not-touch")
	if err := os.WriteFile(unexpected, []byte("unexpected"), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := NewActivationStager(h.installer, stager.reviewer, stager.assessor); err == nil {
		t.Fatal("unexpected activation staging entry should fail closed")
	}
}
