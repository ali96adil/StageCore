package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func createSessionFoundationFixture(t *testing.T, s *store.Store) (domain.Project, domain.RuntimeSnapshot, []domain.Cue) {
	t.Helper()
	ctx := context.Background()
	project, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-027 Foundation"})
	if err != nil {
		t.Fatal(err)
	}
	cues := make([]domain.Cue, 0, 2)
	for i, name := range []string{"First", "Second"} {
		cue, err := s.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: string(rune('1' + i)), Name: name,
			OrderIndex: i, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		cues = append(cues, cue)
	}
	if err := s.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(s).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	return project, runtimeSnapshot, cues
}

func TestSessionFoundationRecordsExplicitStartSemantics(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, runtimeSnapshot, cues := createSessionFoundationFixture(t, s)

	legacySession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "legacy-compatible start")
	if err != nil {
		t.Fatal(err)
	}
	beginning, err := s.GetSessionFoundation(ctx, legacySession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if beginning.ContractVersion != domain.SessionContractVersion1 || beginning.Type != domain.SessionRehearsal {
		t.Fatalf("beginning contract/type=%+v", beginning)
	}
	if beginning.StartPosition.Kind != domain.SessionStartBeginning || beginning.StartPosition.CueID != nil {
		t.Fatalf("beginning start=%+v", beginning.StartPosition)
	}
	if beginning.CurrentCueID != nil || beginning.LastCompletedCueID != nil || beginning.NextCueID == nil || *beginning.NextCueID != cues[0].ID {
		t.Fatalf("beginning progress current=%v last=%v next=%v", beginning.CurrentCueID, beginning.LastCompletedCueID, beginning.NextCueID)
	}
	if beginning.StateTruth.RestorationStatus != domain.SessionRestorationNotRequired || beginning.StateTruth.ManualConfirmationRequired {
		t.Fatalf("beginning truth=%+v", beginning.StateTruth)
	}

	selected, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID, SessionType: domain.SessionRehearsal, Name: "selected cue",
		StartPosition: domain.SessionStartPosition{Version: 1, Kind: domain.SessionStartCue, CueID: &cues[1].ID, Metadata: json.RawMessage(`{"source":"test"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selected.StartPosition.Kind != domain.SessionStartCue || selected.StartPosition.CueID == nil || *selected.StartPosition.CueID != cues[1].ID {
		t.Fatalf("selected start=%+v", selected.StartPosition)
	}
	if selected.CurrentCueID != nil || selected.LastCompletedCueID != nil || selected.NextCueID == nil || *selected.NextCueID != cues[1].ID {
		t.Fatalf("selected progress current=%v last=%v next=%v", selected.CurrentCueID, selected.LastCompletedCueID, selected.NextCueID)
	}
	if selected.StateTruth.RestorationStatus != domain.SessionRestorationManualConfirmationRequired || !selected.StateTruth.ManualConfirmationRequired || selected.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("selected truth=%+v", selected.StateTruth)
	}
	if _, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID, SessionType: domain.SessionShow,
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartCue, CueID: &cues[1].ID},
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("SHOW selected-cue err=%v, want conflict", err)
	}
}

func TestSessionFoundationProgressPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	h, err := db.Open(ctx, db.Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(h.DB, clock.Fixed{Time: fixedTime})
	_, runtimeSnapshot, cues := createSessionFoundationFixture(t, s)
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "progress")
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.CreateCueExecution(ctx, session.ID, cues[0].ID, "corr-1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, session.ID, cues[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, execution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
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
	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeSnapshotID != runtimeSnapshot.ID {
		t.Fatalf("snapshot=%s want %s", loaded.RuntimeSnapshotID, runtimeSnapshot.ID)
	}
	if loaded.CurrentCueID == nil || *loaded.CurrentCueID != cues[0].ID || loaded.LastCompletedCueID == nil || *loaded.LastCompletedCueID != cues[0].ID {
		t.Fatalf("persisted current/last current=%v last=%v", loaded.CurrentCueID, loaded.LastCompletedCueID)
	}
	if loaded.NextCueID == nil || *loaded.NextCueID != cues[1].ID {
		t.Fatalf("persisted next=%v want %s", loaded.NextCueID, cues[1].ID)
	}
}

func TestRestartReconciliationSuspendsRehearsalWithoutReplay(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	project, runtimeSnapshot, cues := createSessionFoundationFixture(t, s)
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "interrupted")
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.CreateCueExecution(ctx, session.ID, cues[0].ID, "corr-restart", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, session.ID, cues[0].ID); err != nil {
		t.Fatal(err)
	}
	command := contracts.CommandEnvelope{
		CommandID: "00000000-0000-7000-8000-000000000027", CommandType: "cue.go", SchemaVersion: contracts.SchemaVersion1,
		IssuedAt: fixedTime, ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer: "test", CorrelationID: "00000000-0000-7000-8000-000000000027", Priority: "P1",
		IdempotencyKey: "00000000-0000-7000-8000-000000000027", Payload: json.RawMessage(`{}`),
	}
	if _, reserved, err := s.ReserveCommand(ctx, command); err != nil || !reserved {
		t.Fatalf("reserve command reserved=%v err=%v", reserved, err)
	}
	count, err := s.ReconcileInterruptedRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled sessions=%d want 1", count)
	}

	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.SessionAborted || loaded.LifecycleState != domain.SessionLifecycleSuspended {
		t.Fatalf("reconciled status=%s lifecycle=%s", loaded.Status, loaded.LifecycleState)
	}
	if loaded.EndReason != "HUB_RESTART_INTERRUPTED" || loaded.EndedAt == nil {
		t.Fatalf("reconciled end reason/time=%q %v", loaded.EndReason, loaded.EndedAt)
	}
	if loaded.CurrentCueID == nil || *loaded.CurrentCueID != cues[0].ID || loaded.NextCueID == nil || *loaded.NextCueID != cues[0].ID {
		t.Fatalf("interrupted current/next=%v/%v; interrupted cue must remain the candidate", loaded.CurrentCueID, loaded.NextCueID)
	}
	if loaded.StateTruth.RestorationStatus != domain.SessionRestorationManualConfirmationRequired || !loaded.StateTruth.ManualConfirmationRequired || loaded.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("reconciled truth=%+v", loaded.StateTruth)
	}
	executions, err := s.ListCueExecutions(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != 1 || executions[0].ID != execution.ID || executions[0].Result != domain.ExecutionCancelled {
		t.Fatalf("reconciled execution=%+v", executions)
	}
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != "rehearsal.suspended" || !events[0].OccurredAt.Equal(fixedTime) {
		t.Fatalf("recovery events=%+v", events)
	}
	if record, reserved, err := s.ReserveCommand(ctx, command); err != nil || reserved || record.Status != string(contracts.CommandAccepted) {
		t.Fatalf("duplicate command after restart reserved=%v record=%+v err=%v", reserved, record, err)
	}
}

func TestRestartReconciliationKeepsShowStrictAndLifecycleStatesDistinct(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	show, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "show")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcileInterruptedRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetSessionFoundation(ctx, show.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LifecycleState != domain.SessionLifecycleAborted || !loaded.StateTruth.ManualConfirmationRequired {
		t.Fatalf("SHOW reconciliation=%+v", loaded)
	}

	rehearsal, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "stopped")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EndSessionLifecycle(ctx, rehearsal.ID, domain.SessionLifecycleStopped, "operator stopped"); err != nil {
		t.Fatal(err)
	}
	stopped, err := s.GetSessionFoundation(ctx, rehearsal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != domain.SessionCompleted || stopped.LifecycleState != domain.SessionLifecycleStopped || stopped.EndReason != "operator stopped" {
		t.Fatalf("stopped session=%+v", stopped)
	}
	if err := s.EndSessionLifecycle(ctx, rehearsal.ID, domain.SessionLifecycleCompleted, ""); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second terminal transition err=%v want conflict", err)
	}
}
