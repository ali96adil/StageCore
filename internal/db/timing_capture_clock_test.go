package db

import (
	"context"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestTimingCaptureKeepsInsertionOrderAcrossClockRegression(t *testing.T) {
	ctx := context.Background()
	h, err := Open(ctx, Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	base := time.Date(2026, 8, 30, 21, 0, 0, 0, time.UTC)
	clock := &mutableTimingClock{now: base}
	s := store.New(h.DB, clock)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-028 Clock Regression", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	firstCue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "1", Name: "First",
		OrderIndex: 0, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondCue, err := s.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Second",
		OrderIndex: 1, CueType: "STANDARD", Criticality: "NORMAL", Enabled: true,
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
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "clock-regression")
	if err != nil {
		t.Fatal(err)
	}

	clock.Set(base.Add(10 * time.Second))
	firstExecution, err := s.CreateCueExecution(ctx, session.ID, firstCue.ID, "corr-first", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, session.ID, firstCue.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, firstExecution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	// Move the wall clock backwards. Capture order must still follow the actual
	// inserted execution sequence and expose the negative interval as data.
	clock.Set(base.Add(5 * time.Second))
	secondExecution, err := s.CreateCueExecution(ctx, session.ID, secondCue.ID, "corr-second", "test.operator")
	if err != nil {
		t.Fatal(err)
	}

	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("timing events=%d want 2", len(events))
	}
	second := decodeTimingObservation(t, events[1])
	if second.CueExecutionID != secondExecution.ID {
		t.Fatalf("second execution=%s want %s", second.CueExecutionID, secondExecution.ID)
	}
	if second.PreviousCueExecutionID == nil || *second.PreviousCueExecutionID != firstExecution.ID {
		t.Fatalf("previous execution=%v want %s", second.PreviousCueExecutionID, firstExecution.ID)
	}
	if second.PreviousCueID == nil || *second.PreviousCueID != firstCue.ID {
		t.Fatalf("previous cue=%v want %s", second.PreviousCueID, firstCue.ID)
	}
	wantPreviousStart := base.Add(10 * time.Second).UnixMicro()
	if second.PreviousCueStartedAtUS == nil || *second.PreviousCueStartedAtUS != wantPreviousStart {
		t.Fatalf("previous start=%v want %d", second.PreviousCueStartedAtUS, wantPreviousStart)
	}
	wantInterval := int64(-5 * time.Second / time.Microsecond)
	if second.CueToCueElapsedUS == nil || *second.CueToCueElapsedUS != wantInterval {
		t.Fatalf("signed interval=%v want %d", second.CueToCueElapsedUS, wantInterval)
	}
}
