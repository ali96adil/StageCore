package sessionmemory

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSessionMemoryExposesSuspendedLifecycleAndStateTruth(t *testing.T) {
	ctx := context.Background()
	h, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Suspended Memory"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Cue", OrderIndex: 0,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "resume later")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, session.ID, cue.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcileInterruptedRuntime(ctx); err != nil {
		t.Fatal(err)
	}

	memory := New(s)
	items, err := memory.List(ctx, project.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("session memory=%+v", items)
	}
	got := items[0]
	if got.Status != domain.SessionAborted || got.LifecycleState != domain.SessionLifecycleSuspended {
		t.Fatalf("coarse/lifecycle=%s/%s", got.Status, got.LifecycleState)
	}
	if got.StartPosition.Kind != domain.SessionStartBeginning || got.NextCueID == nil || *got.NextCueID != cue.ID {
		t.Fatalf("start/progress=%+v next=%v", got.StartPosition, got.NextCueID)
	}
	if got.StateTruth.RestorationStatus != domain.SessionRestorationManualConfirmationRequired || !got.StateTruth.ManualConfirmationRequired {
		t.Fatalf("state truth=%+v", got.StateTruth)
	}
}
