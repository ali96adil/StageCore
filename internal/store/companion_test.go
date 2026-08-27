package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestCompanionIdentitySurvivesMachineMetadataChange(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 17, 0, 0, 0, time.UTC)
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Fixed{Time: now})

	registered, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{
		DisplayName:  "Video Mac",
		Hostname:     "video-mac.local",
		Platform:     "darwin",
		Architecture: "arm64",
		Version:      "0.1.0",
		Capabilities: []string{"osc.send", "local.launch", "osc.send"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.ID == "" || registered.TrustState != domain.CompanionUntrusted || registered.Readiness != domain.CompanionReadinessUnknown {
		t.Fatalf("registered=%#v", registered)
	}
	if len(registered.Capabilities) != 2 || registered.Capabilities[0] != "local.launch" || registered.Capabilities[1] != "osc.send" {
		t.Fatalf("capabilities=%v", registered.Capabilities)
	}
	if err := s.SetCompanionTrustState(ctx, registered.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}

	updated, err := s.UpdateCompanionReport(ctx, registered.ID, store.CompanionReportParams{
		DisplayName:  "Video Main Mac",
		Hostname:     "renamed-video.local",
		Platform:     "darwin",
		Architecture: "arm64",
		Version:      "0.1.1",
		Capabilities: []string{"osc.send", "local.launch"},
		Readiness:    domain.CompanionReadinessSyncing,
		ConfigHash:   "role-config-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != registered.ID {
		t.Fatalf("companion identity changed: before=%s after=%s", registered.ID, updated.ID)
	}
	if updated.Hostname != "renamed-video.local" || updated.DisplayName != "Video Main Mac" || updated.TrustState != domain.CompanionTrusted {
		t.Fatalf("updated=%#v", updated)
	}
}

func TestMachineRoleAllowsOneActiveTrustedCompanionAndExplicitReplacement(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 17, 5, 0, 0, time.UTC)
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Fixed{Time: now})

	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Companion Show"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{
		RoleKey:              "VIDEO-MAIN",
		DisplayName:          "Main Video",
		RequiredCapabilities: []string{"osc.send"},
		Required:             true,
	})
	if err != nil {
		t.Fatal(err)
	}

	first := registerTrustedCompanion(t, ctx, s, "Mac A")
	second := registerTrustedCompanion(t, ctx, s, "Mac B")

	assigned, err := s.AssignMachineRole(ctx, role.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.State != domain.RoleAssigned || assigned.CompanionID != first.ID {
		t.Fatalf("assigned=%#v", assigned)
	}

	// Repeating the same assignment is idempotent and must not create a second row.
	repeated, err := s.AssignMachineRole(ctx, role.ID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != assigned.ID {
		t.Fatalf("duplicate assignment created: first=%s repeated=%s", assigned.ID, repeated.ID)
	}

	if _, err := s.AssignMachineRole(ctx, role.ID, second.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second active Companion error=%v want conflict", err)
	}
	if err := s.ReleaseRoleAssignment(ctx, assigned.ID); err != nil {
		t.Fatal(err)
	}

	replacement, err := s.AssignMachineRole(ctx, role.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == assigned.ID || replacement.CompanionID != second.ID || replacement.State != domain.RoleAssigned {
		t.Fatalf("replacement=%#v", replacement)
	}
	active, err := s.GetActiveRoleAssignment(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != replacement.ID {
		t.Fatalf("active=%#v replacement=%#v", active, replacement)
	}
}

func TestMachineRoleRejectsUntrustedCompanion(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 26, 17, 10, 0, 0, time.UTC)
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Fixed{Time: now})
	project, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Trust Gate"})
	if err != nil {
		t.Fatal(err)
	}
	role, err := s.CreateMachineRole(ctx, project.ID, store.CreateMachineRoleParams{RoleKey: "VIDEO-MAIN", Required: true})
	if err != nil {
		t.Fatal(err)
	}
	companion, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{DisplayName: "Untrusted Mac"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignMachineRole(ctx, role.ID, companion.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("untrusted assignment error=%v want conflict", err)
	}
}

func registerTrustedCompanion(t *testing.T, ctx context.Context, s *store.Store, name string) domain.Companion {
	t.Helper()
	companion, err := s.RegisterCompanion(ctx, store.RegisterCompanionParams{DisplayName: name, Platform: "darwin", Architecture: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetCompanionTrustState(ctx, companion.ID, domain.CompanionTrusted); err != nil {
		t.Fatal(err)
	}
	companion, err = s.GetCompanion(ctx, companion.ID)
	if err != nil {
		t.Fatal(err)
	}
	return companion
}
