package store_test

import (
	"context"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestNotesAndInterruptedRuntimeSurviveReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Real{})
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "Session Memory", CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "Opening", OrderIndex: 1,
		CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, []domain.Action{{OrderIndex: 0, ExecutionMode: "SEQUENTIAL", TargetRef: "LOCAL", CapabilityKey: "sim.test", PriorityClass: domain.PriorityP1, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "owner")
	if err != nil {
		t.Fatal(err)
	}
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "Act 1")
	if err != nil {
		t.Fatal(err)
	}
	cueExecution, err := s.CreateCueExecution(ctx, session.ID, cue.ID, "corr-1", "owner")
	if err != nil {
		t.Fatal(err)
	}
	actionExecution, err := s.CreateActionExecution(ctx, cueExecution.ID, cue.Actions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	category := "video"
	note, err := s.CreateNote(ctx, project.ID, store.CreateNoteParams{
		SessionID: &session.ID, CueID: &cue.ID, Category: category,
		Body: "Check blackout timing", CreatedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := s.SetNoteStatus(ctx, project.ID, note.ID, domain.NoteResolved)
	if err != nil || resolved.ResolvedAt == nil {
		t.Fatalf("resolve note=%+v err=%v", resolved, err)
	}
	reopened, err := s.SetNoteStatus(ctx, project.ID, note.ID, domain.NoteOpen)
	if err != nil || reopened.ResolvedAt != nil {
		t.Fatalf("reopen note=%+v err=%v", reopened, err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	h, err = db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	s = store.New(h.DB, clock.Real{})

	notes, err := s.ListNotes(ctx, project.ID, store.NoteFilter{Status: domain.NoteOpen, Category: category, SessionID: session.ID, CueID: cue.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].ID != note.ID || notes[0].Body != "Check blackout timing" {
		t.Fatalf("notes after reopen=%+v", notes)
	}
	sessions, err := s.ListSessionsForProject(ctx, project.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != session.ID || sessions[0].Status != domain.SessionActive {
		t.Fatalf("session before reconciliation=%+v", sessions)
	}
	count, err := s.ReconcileInterruptedRuntime(ctx)
	if err != nil || count != 1 {
		t.Fatalf("reconcile count=%d err=%v", count, err)
	}
	loadedSession, err := s.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedSession.Status != domain.SessionAborted || loadedSession.EndedAt == nil {
		t.Fatalf("interrupted session=%+v", loadedSession)
	}
	cueExecutions, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cueExecutions) != 1 || cueExecutions[0].Result != domain.ExecutionCancelled || cueExecutions[0].CompletedAt == nil {
		t.Fatalf("interrupted cue executions=%+v", cueExecutions)
	}
	actionExecutions, err := s.ListActionExecutions(ctx, cueExecution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actionExecutions) != 1 || actionExecutions[0].ID != actionExecution.ID || actionExecutions[0].Result != domain.ExecutionCancelled || actionExecutions[0].ErrorCode == nil || *actionExecutions[0].ErrorCode != "HUB_RESTART_INTERRUPTED" {
		t.Fatalf("interrupted action executions=%+v", actionExecutions)
	}
}
