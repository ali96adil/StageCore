package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

type movingClock struct{ now time.Time }

func (m *movingClock) Now() time.Time { return m.now.UTC() }

func TestExpireCompanionHeartbeatMarksCompanionAndRoleOffline(t *testing.T) {
	ctx := context.Background()
	c := &movingClock{now: time.Date(2026, 8, 26, 17, 30, 0, 0, time.UTC)}
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, c)

	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Heartbeat Show"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{RoleKey: "VIDEO-MAIN", Required: true})
	if err != nil {
		t.Fatal(err)
	}
	companionState, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{DisplayName: "Heartbeat Mac"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCompanionTrustState(ctx, companionState.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	assignment, err := s.AssignMachineRole(ctx, role.ID, companionState.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRoleAssignmentState(ctx, assignment.ID, domain.RoleReady); err != nil {
		t.Fatal(err)
	}

	c.now = c.now.Add(6 * time.Second)
	expired, err := s.ExpireCompanionHeartbeats(ctx, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if expired != 1 {
		t.Fatalf("expired=%d want 1", expired)
	}
	companionState, err = s.GetCompanion(ctx, companionState.ID)
	if err != nil {
		t.Fatal(err)
	}
	if companionState.Readiness != domain.CompanionReadinessOffline {
		t.Fatalf("companion readiness=%s want OFFLINE", companionState.Readiness)
	}
	assignment, err = s.GetActiveRoleAssignment(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assignment.State != domain.RoleOffline {
		t.Fatalf("role state=%s want OFFLINE", assignment.State)
	}
}

func TestSetRoleAssignmentStateDoesNotReviveReleasedAssignment(t *testing.T) {
	ctx := context.Background()
	c := &movingClock{now: time.Date(2026, 8, 26, 17, 31, 0, 0, time.UTC)}
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, c)
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Release Show"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{RoleKey: "VIDEO-MAIN", Required: true})
	if err != nil {
		t.Fatal(err)
	}
	companionState, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{DisplayName: "Release Mac"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCompanionTrustState(ctx, companionState.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	assignment, err := s.AssignMachineRole(ctx, role.ID, companionState.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseRoleAssignment(ctx, assignment.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRoleAssignmentState(ctx, assignment.ID, domain.RoleReady); err == nil {
		t.Fatal("released assignment was revived")
	}
}
