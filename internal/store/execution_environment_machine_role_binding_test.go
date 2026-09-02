package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/executionenv"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestExecutionEnvironmentMachineRoleBindingIntegrityAndFork(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-025 Binding", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateExecutionEnvironmentManifest(ctx, revision.ID, executionEnvironmentFixture("video-main"), "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if created.MachineRoleID != nil {
		t.Fatalf("new environment unexpectedly bound to role %v", created.MachineRoleID)
	}

	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "video-main", DisplayName: "Main Video", Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roleID := role.ID
	bound, err := s.SetExecutionEnvironmentMachineRole(ctx, created.ID, &roleID)
	if err != nil {
		t.Fatal(err)
	}
	if bound.MachineRoleID == nil || *bound.MachineRoleID != role.ID {
		t.Fatalf("bound role=%v want %s", bound.MachineRoleID, role.ID)
	}
	if bound.ContentSHA256 != created.ContentSHA256 {
		t.Fatalf("binding changed manifest hash got=%s want=%s", bound.ContentSHA256, created.ContentSHA256)
	}
	var storedRoleID string
	if err := handle.DB.QueryRowContext(ctx, `SELECT machine_role_id FROM execution_environment_manifests WHERE environment_manifest_id = ?`, created.ID).Scan(&storedRoleID); err != nil {
		t.Fatal(err)
	}
	if storedRoleID != role.ID {
		t.Fatalf("stored role=%s want %s", storedRoleID, role.ID)
	}

	otherProject, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Other Project", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	otherRole, err := s.CreateMachineRole(ctx, otherProject.ID, store.CreateMachineRoleParams{
		RoleKey: "other-video", DisplayName: "Other Video", Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRoleID := otherRole.ID
	if _, err := s.SetExecutionEnvironmentMachineRole(ctx, created.ID, &otherRoleID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("cross-project Store bind err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE execution_environment_manifests SET machine_role_id = ? WHERE environment_manifest_id = ?`, otherRole.ID, created.ID); err == nil || !strings.Contains(err.Error(), "EXECUTION_ENVIRONMENT_MACHINE_ROLE_PROJECT_MISMATCH") {
		t.Fatalf("cross-project direct SQL bind err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE machine_roles SET project_id = ? WHERE machine_role_id = ?`, otherProject.ID, role.ID); err == nil || !strings.Contains(err.Error(), "EXECUTION_ENVIRONMENT_MACHINE_ROLE_PROJECT_MISMATCH") {
		t.Fatalf("bound role project move err=%v", err)
	}

	unbound, err := s.SetExecutionEnvironmentMachineRole(ctx, created.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if unbound.MachineRoleID != nil {
		t.Fatalf("unbound role=%v", unbound.MachineRoleID)
	}
	bound, err = s.SetExecutionEnvironmentMachineRole(ctx, created.ID, &roleID)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetExecutionEnvironmentMachineRole(ctx, created.ID, nil); !errors.Is(err, domain.ErrRevisionFrozen) {
		t.Fatalf("frozen revision bind mutation err=%v", err)
	}

	fork, err := s.EnsureProjectDraft(ctx, project.ID, "fork.actor", "continue F-025 binding")
	if err != nil {
		t.Fatal(err)
	}
	forkItems, err := s.ListExecutionEnvironmentManifests(ctx, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forkItems) != 1 {
		t.Fatalf("fork environments=%+v", forkItems)
	}
	cloned := forkItems[0]
	if cloned.MachineRoleID == nil || *cloned.MachineRoleID != role.ID {
		t.Fatalf("fork role=%v want %s", cloned.MachineRoleID, role.ID)
	}
	if cloned.ContentSHA256 != created.ContentSHA256 || cloned.Manifest.EnvironmentKey != created.Manifest.EnvironmentKey {
		t.Fatalf("fork changed canonical identity cloned=%+v source=%+v", cloned, created)
	}

	// Production triggers prevent this state. Drop only the role-project guard in
	// this isolated test DB to prove Store reads still fail closed if durable data
	// is corrupted outside normal StageCore write paths.
	if _, err := handle.DB.ExecContext(ctx, `DROP TRIGGER f025_bound_machine_role_project_update_guard`); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE machine_roles SET project_id = ? WHERE machine_role_id = ?`, otherProject.ID, role.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetExecutionEnvironmentManifest(ctx, created.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("corrupt source binding read err=%v", err)
	}
	if _, err := s.GetExecutionEnvironmentManifest(ctx, cloned.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("corrupt fork binding read err=%v", err)
	}
}

func TestExecutionEnvironmentMachineRoleBindingSHOWLock(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	project, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)

	manifest := executionEnvironmentFixture("video-show")
	canonical, err := executionenv.CanonicalBytes(manifest)
	if err != nil {
		t.Fatal(err)
	}
	contentHash, err := executionenv.ContentHash(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestID := "00000000-0000-7000-8000-000000000125"
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO execution_environment_manifests (
			environment_manifest_id, revision_id, environment_key, adapter_key, application_key,
			manifest_json, content_sha256, created_by, created_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		manifestID, runtimeSnapshot.RevisionID, manifest.EnvironmentKey, manifest.AdapterKey, manifest.Application.Key,
		string(canonical), contentHash, "test", 1,
	); err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "video-show", DisplayName: "SHOW Video", Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roleID := role.ID

	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "F-025 binding lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetExecutionEnvironmentMachineRole(ctx, manifestID, &roleID); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("SHOW Store bind err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE execution_environment_manifests SET machine_role_id = ? WHERE environment_manifest_id = ?`, role.ID, manifestID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("SHOW direct binding update err=%v", err)
	}
	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
}
