package store

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/visualengine"
)

func TestVisualEngineModeDefaultsExternalAndInheritsAcrossDraftFork(t *testing.T) {
	ctx := context.Background()
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	stageStore := New(handle.DB, clock.Real{})

	project, source, err := stageStore.CreateProject(ctx, CreateProjectParams{Name: "Visual Mode Inheritance", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	mode, err := stageStore.GetVisualEngineMode(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if mode != visualengine.EngineModeExternal {
		t.Fatalf("default mode=%q want=%q", mode, visualengine.EngineModeExternal)
	}
	if err := stageStore.SetVisualEngineMode(ctx, source.ID, visualengine.EngineModeNative, "owner"); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, source.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}

	draft, err := stageStore.EnsureProjectDraft(ctx, project.ID, "owner", "non-visual configuration edit")
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID == source.ID || draft.ParentRevisionID == nil || *draft.ParentRevisionID != source.ID {
		t.Fatalf("draft=%+v source=%s", draft, source.ID)
	}
	inherited, err := stageStore.GetVisualEngineMode(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inherited != visualengine.EngineModeNative {
		t.Fatalf("inherited mode=%q want=%q", inherited, visualengine.EngineModeNative)
	}
	sourceMode, err := stageStore.GetVisualEngineMode(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sourceMode != visualengine.EngineModeNative {
		t.Fatalf("source mode changed=%q", sourceMode)
	}
}
