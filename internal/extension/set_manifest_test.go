package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/software"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
)

type extensionSetTestRig struct {
	store      *store.Store
	repository *software.Repository
	library    *Library
	installer  *Installer
	service    *ExtensionSetService
	close      func()
}

func TestExtensionSetExportRestorePortableAndFailClosed(t *testing.T) {
	ctx := context.Background()
	source := newExtensionSetTestRig(t, ctx)
	defer source.close()
	target := newExtensionSetTestRig(t, ctx)
	defer target.close()

	baseManifest := extensionSetAddonManifest(t, "example.base-addon", nil)
	sceneManifest := extensionSetAddonManifest(t, "example.scene-addon", []Dependency{{ExtensionID: "example.base-addon", MinVersion: "1.0.0", MaxVersion: "1.0.0"}})
	pluginManifest := extensionSetPluginManifest(t)

	sourceBase := extensionSetRegisterPackage(t, ctx, source, "example.base-addon", []byte("base payload"), baseManifest)
	sourceScene := extensionSetRegisterPackage(t, ctx, source, "example.scene-addon", []byte("scene payload"), sceneManifest)
	sourcePlugin := extensionSetRegisterPackage(t, ctx, source, "example.net-plugin", []byte("plugin payload"), pluginManifest)
	targetBase := extensionSetRegisterPackage(t, ctx, target, "example.base-addon", []byte("base payload"), baseManifest)
	targetScene := extensionSetRegisterPackage(t, ctx, target, "example.scene-addon", []byte("scene payload"), sceneManifest)
	targetPlugin := extensionSetRegisterPackage(t, ctx, target, "example.net-plugin", []byte("plugin payload"), pluginManifest)

	if sourceBase.PackageID == targetBase.PackageID || sourceScene.PackageID == targetScene.PackageID || sourcePlugin.PackageID == targetPlugin.PackageID {
		t.Fatal("test requires different local package IDs across Hubs")
	}
	if _, err := source.installer.InstallPlanned(ctx, sourceBase.PackageID, "source-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := source.installer.InstallPlanned(ctx, sourceScene.PackageID, "source-owner"); err != nil {
		t.Fatal(err)
	}
	pluginInstall, err := source.installer.InstallPlanned(ctx, sourcePlugin.PackageID, "source-owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.store.SetExtensionPermissionDecision(ctx, pluginInstall.InstallationID, "network.udp.send", store.ExtensionPermissionApproved, "source-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := source.store.SetExtensionRuntimeDesiredState(ctx, pluginInstall.InstallationID, store.ExtensionRuntimeDesiredEnabled, "source-owner"); err != nil {
		t.Fatal(err)
	}

	exported, raw, err := source.service.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Extensions) != 3 || strings.Contains(string(raw), "installation_id") || strings.Contains(string(raw), "package_id") || strings.Contains(string(raw), "permission") || strings.Contains(string(raw), "ENABLED") {
		t.Fatalf("unsafe or incomplete export: %s", raw)
	}

	plan, err := target.service.PlanRestore(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != ExtensionSetRestoreReady || plan.PermissionReviewsRestored || plan.RuntimeIntentRestored || plan.NewInstallDesiredState != store.ExtensionRuntimeDesiredDisabled {
		t.Fatalf("restore plan=%+v", plan)
	}
	baseOrder, sceneOrder := 0, 0
	for _, step := range plan.Steps {
		if step.PackageID == sourceBase.PackageID || step.PackageID == sourceScene.PackageID || step.PackageID == sourcePlugin.PackageID {
			t.Fatalf("restore plan leaked source-local package ID: %+v", step)
		}
		switch step.ExtensionID {
		case "example.base-addon":
			baseOrder = step.Order
		case "example.scene-addon":
			sceneOrder = step.Order
		}
	}
	if baseOrder == 0 || sceneOrder == 0 || baseOrder >= sceneOrder {
		t.Fatalf("dependency order base=%d scene=%d steps=%+v", baseOrder, sceneOrder, plan.Steps)
	}

	result, err := target.service.Restore(ctx, raw, "target-owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Installed) != 3 {
		t.Fatalf("installed=%+v", result.Installed)
	}
	var restoredPlugin Installation
	for _, installation := range result.Installed {
		if installation.ExtensionID == "example.net-plugin" {
			restoredPlugin = installation
		}
	}
	if restoredPlugin.InstallationID == "" {
		t.Fatal("restored plugin not found")
	}
	decisions, err := target.store.ListExtensionPermissionDecisions(ctx, restoredPlugin.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 0 {
		t.Fatalf("restore recreated permission approvals: %+v", decisions)
	}
	runtimeState, err := target.store.GetExtensionRuntimeLifecycle(ctx, restoredPlugin.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeState.DesiredState != store.ExtensionRuntimeDesiredDisabled || runtimeState.ObservedState != store.ExtensionRuntimeObservedStopped {
		t.Fatalf("restore recreated runtime intent: %+v", runtimeState)
	}

	noop, err := target.service.PlanRestore(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if noop.Status != ExtensionSetRestoreNoop {
		t.Fatalf("idempotent plan=%+v", noop)
	}
	for _, step := range noop.Steps {
		if step.Action != ExtensionSetRestoreAlreadyInstalled {
			t.Fatalf("idempotent step=%+v", step)
		}
	}
}

func TestExtensionSetRestoreBlocksTamperAndMissingRequiredDependency(t *testing.T) {
	ctx := context.Background()
	rig := newExtensionSetTestRig(t, ctx)
	defer rig.close()
	baseManifest := extensionSetAddonManifest(t, "example.base-addon", nil)
	sceneManifest := extensionSetAddonManifest(t, "example.scene-addon", []Dependency{{ExtensionID: "example.base-addon", MinVersion: "1.0.0"}})
	base := extensionSetRegisterPackage(t, ctx, rig, "example.base-addon", []byte("base payload"), baseManifest)
	scene := extensionSetRegisterPackage(t, ctx, rig, "example.scene-addon", []byte("scene payload"), sceneManifest)
	if _, err := rig.installer.InstallPlanned(ctx, base.PackageID, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.installer.InstallPlanned(ctx, scene.PackageID, "owner"); err != nil {
		t.Fatal(err)
	}
	_, raw, err := rig.service.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var tampered ExtensionSetManifest
	if err := json.Unmarshal(raw, &tampered); err != nil {
		t.Fatal(err)
	}
	for index := range tampered.Extensions {
		if tampered.Extensions[index].ExtensionID == "example.scene-addon" {
			tampered.Extensions[index].PayloadSHA256 = strings.Repeat("0", 64)
		}
	}
	tamperedRaw, _ := json.Marshal(tampered)
	tamperedPlan, err := rig.service.PlanRestore(ctx, tamperedRaw)
	if err != nil {
		t.Fatal(err)
	}
	if tamperedPlan.Status != ExtensionSetRestoreBlocked || !extensionSetHasBlocker(tamperedPlan, "EXACT_PACKAGE_NOT_AVAILABLE", "example.scene-addon") {
		t.Fatalf("tampered plan=%+v", tamperedPlan)
	}

	var clean ExtensionSetManifest
	if err := json.Unmarshal(raw, &clean); err != nil {
		t.Fatal(err)
	}
	var sceneEntry ExtensionSetEntry
	for _, entry := range clean.Extensions {
		if entry.ExtensionID == "example.scene-addon" {
			sceneEntry = entry
		}
	}
	missing := ExtensionSetManifest{Format: ExtensionSetFormatV1, SchemaVersion: ExtensionSetSchemaVersion, Extensions: []ExtensionSetEntry{sceneEntry}}
	missingRaw, _ := json.Marshal(missing)
	missingPlan, err := rig.service.PlanRestore(ctx, missingRaw)
	if err != nil {
		t.Fatal(err)
	}
	if missingPlan.Status != ExtensionSetRestoreBlocked || !extensionSetHasBlocker(missingPlan, "DEPENDENCY_MISSING_FROM_SET", "example.base-addon") {
		t.Fatalf("missing dependency plan=%+v", missingPlan)
	}
}

func TestParseExtensionSetManifestRejectsUnknownAndDuplicateEntries(t *testing.T) {
	hash := strings.Repeat("a", 64)
	unknown := `{"format":"stagecore-extension-set-v1"}`
	unknown = strings.ReplaceAll(unknown, `\"`, `"`)
	unknown = strings.TrimSuffix(unknown, "}") + `,"schema_version":1,"extensions":[],"unexpected":true}`
	if _, err := ParseExtensionSetManifest([]byte(unknown)); err == nil {
		t.Fatal("parser accepted unknown field")
	}
	entry := ExtensionSetEntry{ExtensionID: "example.addon", Version: "1.0.0", Kind: KindAddon, Source: SourceLocal, ManifestSHA256: hash, PayloadSHA256: hash, PayloadSizeBytes: 1, Platform: "linux", Architecture: "arm64"}
	duplicateRaw, err := json.Marshal(ExtensionSetManifest{Format: ExtensionSetFormatV1, SchemaVersion: ExtensionSetSchemaVersion, Extensions: []ExtensionSetEntry{entry, entry}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseExtensionSetManifest(duplicateRaw); err == nil {
		t.Fatal("parser accepted duplicate extension_id")
	}
}

func newExtensionSetTestRig(t *testing.T, ctx context.Context) *extensionSetTestRig {
	t.Helper()
	dataRoot := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	stageStore := store.New(h.DB, clock.Real{})
	v, err := vault.Open(filepath.Join(dataRoot, "vault"), stageStore)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	repository, err := software.New(v, stageStore, software.CurrentHubAPIVersion)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	library, err := NewLibrary(stageStore, repository)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	installer, err := NewInstaller(library, filepath.Join(dataRoot, "extensions"))
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	service, err := NewExtensionSetService(installer)
	if err != nil {
		_ = h.Close()
		t.Fatal(err)
	}
	return &extensionSetTestRig{store: stageStore, repository: repository, library: library, installer: installer, service: service, close: func() { _ = h.Close() }}
}

func extensionSetRegisterPackage(t *testing.T, ctx context.Context, rig *extensionSetTestRig, extensionID string, payload, manifest []byte) Package {
	t.Helper()
	softwarePackage, err := rig.repository.ImportPackage(ctx, software.ImportParams{
		ProductID: extensionID,
		Version: "1.0.0",
		Platform: "linux",
		Architecture: "arm64",
		MinAPIVersion: 1,
		MaxAPIVersion: 1,
		OriginalFilename: extensionID + "-1.0.0",
		SigningStatus: store.SoftwareSigningSigned,
		NotarizationStatus: store.SoftwareNotarizationNotApplicable,
		ReleaseChannel: store.SoftwareChannelRelease,
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	registered, err := rig.library.Register(ctx, softwarePackage.ID, manifest, "catalog")
	if err != nil {
		t.Fatal(err)
	}
	return registered
}

func extensionSetAddonManifest(t *testing.T, extensionID string, dependencies []Dependency) []byte {
	t.Helper()
	raw, err := json.Marshal(Manifest{
		SchemaVersion: 1,
		ExtensionID: extensionID,
		Version: "1.0.0",
		Kind: KindAddon,
		Source: SourceLocal,
		Name: LocalizedText{EN: "Test Addon", ArIQ: "إضافة اختبار"},
		Summary: LocalizedText{EN: "Test addon.", ArIQ: "إضافة للاختبار."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Dependencies: dependencies,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func extensionSetPluginManifest(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(Manifest{
		SchemaVersion: 1,
		ExtensionID: "example.net-plugin",
		Version: "1.0.0",
		Kind: KindPlugin,
		Source: SourceLocal,
		Name: LocalizedText{EN: "Network Plugin", ArIQ: "إضافة شبكة"},
		Summary: LocalizedText{EN: "Network test plugin.", ArIQ: "إضافة شبكة للاختبار."},
		Compatibility: Compatibility{APIMin: 1, APIMax: 1, Platforms: []string{"linux"}, Architectures: []string{"arm64"}},
		Permissions: []string{"network.udp.send"},
		Capabilities: []string{"osc.send"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func extensionSetHasBlocker(plan ExtensionSetRestorePlan, code, extensionID string) bool {
	for _, blocker := range plan.Blockers {
		if blocker.Code == code && blocker.ExtensionID == extensionID {
			return true
		}
	}
	return false
}
