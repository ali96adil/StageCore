package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

func TestVisualEngineConfigurationDefaultPersistenceAndFork(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-026 F2", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}

	defaultConfig, err := s.GetVisualEngineConfiguration(ctx, revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if defaultConfig.Mode != visualengine.ModeNative || defaultConfig.Explicit {
		t.Fatalf("default config=%+v", defaultConfig)
	}

	external, err := s.SetVisualEngineConfiguration(ctx, revision.ID, visualengine.ModeExternal, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if external.Mode != visualengine.ModeExternal || !external.Explicit || external.UpdatedBy != "operator" || !external.UpdatedAt.Equal(fixedTime) {
		t.Fatalf("external config=%+v", external)
	}
	if _, err := s.SetVisualEngineConfiguration(ctx, revision.ID, visualengine.EngineMode("INVALID"), "operator"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid mode err=%v", err)
	}

	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetVisualEngineConfiguration(ctx, revision.ID, visualengine.ModeNative, "operator"); !errors.Is(err, domain.ErrRevisionFrozen) {
		t.Fatalf("frozen revision mutation err=%v", err)
	}

	draft, err := s.EnsureProjectDraft(ctx, project.ID, "editor", "Visual Engine fork")
	if err != nil {
		t.Fatal(err)
	}
	forked, err := s.GetVisualEngineConfiguration(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if forked.Mode != visualengine.ModeExternal || !forked.Explicit || forked.UpdatedBy != "editor" {
		t.Fatalf("forked config=%+v", forked)
	}
}

func TestVisualEngineConfigurationSHOWLockAllowsReadAndRejectsMutation(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	if _, err := handle.DB.ExecContext(ctx, `
		INSERT INTO visual_engine_configurations (revision_id, engine_mode, updated_by, updated_at_us)
		VALUES (?, 'EXTERNAL', 'test', 1)`, runtimeSnapshot.RevisionID); err != nil {
		t.Fatal(err)
	}

	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "F-026 F2 lock")
	if err != nil {
		t.Fatal(err)
	}
	config, err := s.GetVisualEngineConfiguration(ctx, runtimeSnapshot.RevisionID)
	if err != nil || config.Mode != visualengine.ModeExternal {
		t.Fatalf("SHOW read config=%+v err=%v", config, err)
	}
	if _, err := s.SetVisualEngineConfiguration(ctx, runtimeSnapshot.RevisionID, visualengine.ModeNative, "operator"); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("SHOW store mutation err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE visual_engine_configurations SET engine_mode = 'NATIVE' WHERE revision_id = ?`, runtimeSnapshot.RevisionID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct SHOW update err=%v", err)
	}
	if _, err := handle.DB.ExecContext(ctx, `DELETE FROM visual_engine_configurations WHERE revision_id = ?`, runtimeSnapshot.RevisionID); err == nil || !strings.Contains(err.Error(), "SHOW_CONFIGURATION_LOCKED") {
		t.Fatalf("direct SHOW delete err=%v", err)
	}

	if err := s.EndSession(ctx, show.ID, domain.SessionCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `UPDATE visual_engine_configurations SET engine_mode = 'NATIVE' WHERE revision_id = ?`, runtimeSnapshot.RevisionID); err != nil {
		t.Fatalf("post-SHOW update err=%v", err)
	}
}
