package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestShowConfigurationLockProtectsProjectWithoutFreezingRuntime(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	project, runtimeSnapshot, cues := createSessionFoundationFixture(t, s)

	otherProject, _, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Other Project"})
	if err != nil {
		t.Fatal(err)
	}

	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "locked show")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := s.ShowConfigurationLockState(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Locked || lock.Version != store.ShowConfigurationLockVersion1 {
		t.Fatalf("lock=%+v", lock)
	}
	if lock.ActiveShowSessionID == nil || *lock.ActiveShowSessionID != show.ID || lock.RuntimeSnapshotID == nil || *lock.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("lock identity=%+v", lock)
	}
	if lock.ShowStartedAt == nil || !lock.ShowStartedAt.Equal(fixedTime) || lock.OverrideSupported || lock.UnlockAction != "SHOW_EXIT" {
		t.Fatalf("lock semantics=%+v", lock)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, project.ID); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("mutable guard err=%v", err)
	}

	if _, err := handle.DB.ExecContext(ctx, `UPDATE cues SET name = 'unsafe edit' WHERE cue_id = ?`, cues[0].ID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct cue mutation err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `INSERT INTO project_revisions (revision_id, project_id, revision_number, status, created_at_us) VALUES ('00000000-0000-7000-8000-000000000012', ?, 99, 'DRAFT', 1)`, project.ID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct revision mutation err=%v", err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "LOCKED-TARGET"}); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("locked alias err=%v", err)
	}

	// F-012 is project-scoped. A different Project remains editable.
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: otherProject.ID, LogicalName: "OTHER-TARGET"}); err != nil {
		t.Fatalf("other project edit: %v", err)
	}

	// Live runtime truth remains writable while configuration is locked.
	execution, err := s.CreateCueExecution(ctx, show.ID, cues[0].ID, "f012-runtime", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, show.ID, cues[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, execution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListEvents(ctx, show.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("SHOW runtime produced no event journal records")
	}

	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	unlocked, err := s.ShowConfigurationLockState(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unlocked.Locked || unlocked.ActiveShowSessionID != nil || unlocked.RuntimeSnapshotID != nil {
		t.Fatalf("unlocked state=%+v", unlocked)
	}
	if err := s.RequireProjectConfigurationMutable(ctx, project.ID); err != nil {
		t.Fatalf("post-show mutable guard: %v", err)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "POST-SHOW-TARGET"}); err != nil {
		t.Fatalf("post-show edit: %v", err)
	}
}

func TestRehearsalDoesNotActivateShowConfigurationLock(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	if _, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "rehearsal"); err != nil {
		t.Fatal(err)
	}
	lock, err := s.ShowConfigurationLockState(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Locked {
		t.Fatalf("REHEARSAL unexpectedly locked configuration: %+v", lock)
	}
	if _, err := s.CreateAlias(ctx, domain.ProjectDeviceAlias{ProjectID: project.ID, LogicalName: "REHEARSAL-TARGET"}); err != nil {
		t.Fatalf("rehearsal edit: %v", err)
	}
}
