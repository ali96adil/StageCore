package showcapsule

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/companion"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/vault"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

func TestVisualEngineRuntimeSnapshotAndShowCapsulePortability(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC)

	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	stageStore := store.New(handle.DB, clock.Fixed{Time: now})
	vaultRoot := t.TempDir()
	stageVault, err := vault.Open(vaultRoot, stageStore)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(stageStore, stageVault, clock.Fixed{Time: now})
	if err != nil {
		t.Fatal(err)
	}

	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{
		Name: "Visual Portability Show", Description: "F-026 Slice E2", CreatedBy: "owner", ChangeNote: "visual portability fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	role, err := stageStore.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "visual-engine-main", DisplayName: "Visual Engine Main",
		RequiredCapabilities: []string{visualengine.CapabilityPreload, visualengine.CapabilityOutputConfigure}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	targetConfig := json.RawMessage(fmt.Sprintf(`{"machine_role_id":%q}`, role.ID))
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: role.RoleKey, LogicalType: companion.MachineRoleLogicalType,
		TargetRef: role.RoleKey, ProjectConfig: targetConfig,
	}); err != nil {
		t.Fatal(err)
	}

	content := []byte("stagecore-f026-visual-portability-managed-media")
	managed, err := stageVault.ImportManaged(ctx, vault.ImportParams{
		ProjectID: project.ID, Name: "Backdrop", AssetPolicy: store.MediaPolicyArchiveRequired,
		OriginalFilename: "backdrop.mov",
	}, strings.NewReader(string(content)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.AddMachineRoleMediaRequirement(ctx, role.ID, managed.Version.ID, true); err != nil {
		t.Fatal(err)
	}

	outputParams := json.RawMessage(`{"contract_version":1,"output_id":"main","width":1920,"height":1080}`)
	preloadParams := json.RawMessage(fmt.Sprintf(
		`{"contract_version":1,"layer_id":"backdrop","content_version_id":%q,"content_hash":%q,"content_mode":"FIT","output_id":"main","z_index":0}`,
		managed.Version.ID, managed.Version.ContentHash,
	))
	if err := visualengine.ValidateCommand(visualengine.CapabilityOutputConfigure, outputParams); err != nil {
		t.Fatalf("fixture output configuration is not a valid Visual Engine command: %v", err)
	}
	if err := visualengine.ValidateCommand(visualengine.CapabilityPreload, preloadParams); err != nil {
		t.Fatalf("fixture preload is not a valid Visual Engine command: %v", err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "V1", Name: "Visual Opening", OrderIndex: 1, Enabled: true,
	}, []domain.Action{
		{OrderIndex: 1, TargetRef: role.RoleKey, CapabilityKey: visualengine.CapabilityOutputConfigure, Parameters: outputParams, Enabled: true},
		{OrderIndex: 2, TargetRef: role.RoleKey, CapabilityKey: visualengine.CapabilityPreload, Parameters: preloadParams, Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	runtimeSnapshot, manifest, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	assertPortableVisualManifest(t, manifest, role.ID, role.RoleKey, managed.Version.ID, managed.Version.ContentHash, targetConfig)

	exportRoot := t.TempDir()
	result, err := service.Export(ctx, project.ID, exportRoot, BuildOptions{
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Mode:              ExportSelfContained,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Manifest.Media) != 1 {
		t.Fatalf("capsule media requirements=%d, want 1", len(result.Manifest.Media))
	}
	media := result.Manifest.Media[0]
	if media.MachineRoleID != role.ID || media.RoleKey != role.RoleKey || media.ContentVersionID != managed.Version.ID || media.ContentSHA256 != managed.Version.ContentHash || !media.Required || !media.ContentIncluded {
		t.Fatalf("unexpected Visual Engine capsule media requirement: %+v", media)
	}
	if len(result.Manifest.Objects) != 1 || !result.Manifest.Objects[0].Included || result.Manifest.Objects[0].ContentSHA256 != managed.Version.ContentHash {
		t.Fatalf("managed Visual Engine object was not included: %+v", result.Manifest.Objects)
	}
	assertPortableVisualManifest(t, result.Manifest.RuntimeSnapshot.Manifest, role.ID, role.RoleKey, managed.Version.ID, managed.Version.ContentHash, targetConfig)

	verified, err := Verify(result.Path)
	if err != nil {
		t.Fatalf("verify Visual Engine capsule: %v", err)
	}
	assertPortableVisualManifest(t, verified.RuntimeSnapshot.Manifest, role.ID, role.RoleKey, managed.Version.ID, managed.Version.ContentHash, targetConfig)
	if len(verified.Media) != 1 || !verified.Media[0].ContentIncluded || verified.Media[0].ContentSHA256 != managed.Version.ContentHash {
		t.Fatalf("verified Visual Engine media identity=%+v", verified.Media)
	}
	objectPath := filepath.Join(result.Path, filepath.FromSlash(verified.Objects[0].ArchivePath))
	gotBytes, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != string(content) {
		t.Fatalf("portable Visual Engine bytes=%q, want %q", string(gotBytes), string(content))
	}

	manifestJSON, err := json.Marshal(verified.RuntimeSnapshot.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifestJSON), vaultRoot) || strings.Contains(string(manifestJSON), managed.Object.RelativePath) {
		t.Fatalf("Runtime Snapshot leaked Hub-local media path authority: %s", manifestJSON)
	}
}

func assertPortableVisualManifest(t *testing.T, manifest snapshot.Manifest, roleID, roleKey, contentVersionID, contentHash string, targetConfig json.RawMessage) {
	t.Helper()
	if len(manifest.Targets) != 1 {
		t.Fatalf("Visual Engine snapshot targets=%d, want 1", len(manifest.Targets))
	}
	target := manifest.Targets[0]
	if target.TargetRef != roleKey || target.LogicalType != companion.MachineRoleLogicalType || !jsonEqual(target.Configuration, targetConfig) {
		t.Fatalf("Visual Engine target configuration was not preserved: %+v config=%s", target, target.Configuration)
	}
	if len(manifest.RequiredMedia) != 1 {
		t.Fatalf("Visual Engine required_media=%d, want 1", len(manifest.RequiredMedia))
	}
	media := manifest.RequiredMedia[0]
	if media.MachineRoleID != roleID || media.RoleKey != roleKey || media.ContentVersionID != contentVersionID || media.ContentHash != contentHash || media.ChecksumAlgorithm != "SHA256" || !media.Required {
		t.Fatalf("unexpected Visual Engine Runtime Snapshot media identity: %+v", media)
	}
	if len(manifest.Cues) != 1 || len(manifest.Cues[0].Actions) != 2 {
		t.Fatalf("Visual Engine cue/actions were not preserved: %+v", manifest.Cues)
	}
	outputAction := manifest.Cues[0].Actions[0]
	preloadAction := manifest.Cues[0].Actions[1]
	if outputAction.CapabilityKey != visualengine.CapabilityOutputConfigure || preloadAction.CapabilityKey != visualengine.CapabilityPreload {
		t.Fatalf("unexpected Visual Engine capabilities: %q, %q", outputAction.CapabilityKey, preloadAction.CapabilityKey)
	}
	if err := visualengine.ValidateCommand(outputAction.CapabilityKey, outputAction.Parameters); err != nil {
		t.Fatalf("portable output configuration became invalid: %v", err)
	}
	if err := visualengine.ValidateCommand(preloadAction.CapabilityKey, preloadAction.Parameters); err != nil {
		t.Fatalf("portable preload became invalid: %v", err)
	}
	var preload struct {
		ContentVersionID string `json:"content_version_id"`
		ContentHash      string `json:"content_hash"`
	}
	if err := json.Unmarshal(preloadAction.Parameters, &preload); err != nil {
		t.Fatal(err)
	}
	if preload.ContentVersionID != contentVersionID || preload.ContentHash != contentHash {
		t.Fatalf("portable preload content identity=%+v", preload)
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
