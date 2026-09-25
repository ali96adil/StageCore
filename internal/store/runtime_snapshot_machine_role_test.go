package store_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestPublishedRuntimeSnapshotAdvancesMachineRoleRequirement(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)

	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Snapshot Role Advance", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey: "MAC-LOCAL-ECHO", DisplayName: "Mac Local Echo",
		RequiredCapabilities: []string{"local.echo"}, Required: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	first, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMachineRole(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequiredRuntimeSnapshotID == nil || *got.RequiredRuntimeSnapshotID != first.ID {
		t.Fatalf("first publish role snapshot=%v want %s", got.RequiredRuntimeSnapshotID, first.ID)
	}

	second, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.GetMachineRole(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequiredRuntimeSnapshotID == nil || *got.RequiredRuntimeSnapshotID != second.ID {
		t.Fatalf("second publish role snapshot=%v want %s", got.RequiredRuntimeSnapshotID, second.ID)
	}
}
