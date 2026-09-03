package timingintelligence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time { return c.now.UTC() }
func (c *mutableClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

type timingFixture struct {
	store     *store.Store
	clock     *mutableClock
	projectID string
	snapshot  domain.RuntimeSnapshot
	cues      []domain.Cue
}

func newTimingFixture(t *testing.T) timingFixture {
	t.Helper()
	ctx := context.Background()
	stageClock := &mutableClock{now: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)}
	handle, err := db.Open(ctx, db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	stageStore := store.New(handle.DB, stageClock)
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Timing Show", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	cues := make([]domain.Cue, 0, 3)
	for i, name := range []string{"House Open", "Elevator Down", "Blackout"} {
		cue, createErr := stageStore.CreateCueWithActions(ctx, domain.Cue{
			RevisionID: revision.ID, DisplayLabel: string(rune('1' + i)), Name: name,
			OrderIndex: i, Enabled: true, ExecutionPolicy: json.RawMessage(`{}`),
		}, nil)
		if createErr != nil {
			t.Fatal(createErr)
		}
		cues = append(cues, cue)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	return timingFixture{store: stageStore, clock: stageClock, projectID: project.ID, snapshot: runtimeSnapshot, cues: cues}
}

func (f timingFixture) rehearsal(t *testing.T, name string, firstToSecond, secondToThird time.Duration, partial bool) domain.Session {
	t.Helper()
	ctx := context.Background()
	session, err := f.store.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: f.snapshot.ID, SessionType: domain.SessionRehearsal, Name: name,
		StartPosition: domain.SessionStartPosition{Version: 1, Kind: domain.SessionStartBeginning, Metadata: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.executeCue(t, session.ID, f.cues[0].ID)
	f.clock.Advance(firstToSecond)
	f.executeCue(t, session.ID, f.cues[1].ID)
	if partial {
		if err := f.store.EndSessionLifecycle(ctx, session.ID, domain.SessionLifecycleStopped, "partial rehearsal complete"); err != nil {
			t.Fatal(err)
		}
		updated, err := f.store.GetSessionFoundation(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(5 * time.Minute)
		return updated
	}
	f.clock.Advance(secondToThird)
	f.executeCue(t, session.ID, f.cues[2].ID)
	if err := f.store.EndSessionLifecycle(ctx, session.ID, domain.SessionLifecycleCompleted, "rehearsal complete"); err != nil {
		t.Fatal(err)
	}
	updated, err := f.store.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(5 * time.Minute)
	return updated
}

func (f timingFixture) executeCue(t *testing.T, sessionID, cueID string) {
	t.Helper()
	ctx := context.Background()
	execution, err := f.store.CreateCueExecution(ctx, sessionID, cueID, "test:"+cueID, "test.timing")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.FinishCueExecution(ctx, execution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}
}

func TestReportUsesTrustedSequentialIntervalsAndPartialRehearsals(t *testing.T) {
	fixture := newTimingFixture(t)
	fixture.rehearsal(t, "R1", 60*time.Second, 40*time.Second, false)
	fixture.rehearsal(t, "R2", 65*time.Second, 45*time.Second, false)
	partial := fixture.rehearsal(t, "R3 partial", 70*time.Second, 0, true)

	service := New(fixture.store, fixture.clock)
	report, err := service.Report(context.Background(), fixture.projectID, ReportOptions{
		RuntimeSnapshotID: fixture.snapshot.ID,
		SectionFromCueID: fixture.cues[0].ID,
		SectionToCueID: fixture.cues[2].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.AdvisoryOnly {
		t.Fatal("timing intelligence must remain advisory-only")
	}
	first := findTransition(t, report, fixture.cues[0].ID, fixture.cues[1].ID)
	if first.SampleCount != 3 || first.TrustedSessionCount != 3 {
		t.Fatalf("first transition samples=%d sessions=%d", first.SampleCount, first.TrustedSessionCount)
	}
	if first.MedianUS != (65*time.Second).Microseconds() || first.Confidence != ConfidenceMedium {
		t.Fatalf("first transition=%+v", first)
	}
	second := findTransition(t, report, fixture.cues[1].ID, fixture.cues[2].ID)
	if second.SampleCount != 2 || second.Confidence != ConfidenceLow {
		t.Fatalf("second transition=%+v", second)
	}
	if report.Section == nil || report.Section.Statistics.SampleCount != 2 {
		t.Fatalf("section=%+v", report.Section)
	}
	if report.Section.Statistics.MedianUS != (105*time.Second).Microseconds() {
		t.Fatalf("section median=%d want %d", report.Section.Statistics.MedianUS, (105*time.Second).Microseconds())
	}
	candidate := findCandidate(t, report, partial.ID)
	if !candidate.Effective || candidate.LifecycleState != domain.SessionLifecycleStopped {
		t.Fatalf("partial candidate=%+v", candidate)
	}
}

func TestExplicitExcludeChangesTrainingSetWithoutDeletingHistory(t *testing.T) {
	fixture := newTimingFixture(t)
	fixture.rehearsal(t, "R1", 60*time.Second, 0, true)
	fixture.rehearsal(t, "R2", 65*time.Second, 0, true)
	third := fixture.rehearsal(t, "R3", 70*time.Second, 0, true)
	service := New(fixture.store, fixture.clock)

	selection, err := service.SetSessionSelection(context.Background(), fixture.projectID, third.ID, store.TimingSelectionExclude, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Mode != store.TimingSelectionExclude {
		t.Fatalf("selection=%+v", selection)
	}
	report, err := service.Report(context.Background(), fixture.projectID, ReportOptions{RuntimeSnapshotID: fixture.snapshot.ID})
	if err != nil {
		t.Fatal(err)
	}
	stats := findTransition(t, report, fixture.cues[0].ID, fixture.cues[1].ID)
	if stats.SampleCount != 2 || stats.MedianUS != (62500*time.Millisecond).Microseconds() || stats.Confidence != ConfidenceLow {
		t.Fatalf("excluded stats=%+v", stats)
	}
	candidate := findCandidate(t, report, third.ID)
	if candidate.Effective || candidate.SelectionMode != store.TimingSelectionExclude {
		t.Fatalf("excluded candidate=%+v", candidate)
	}
	events, err := fixture.store.ListEvents(context.Background(), third.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundTiming := false
	for _, event := range events {
		if event.EventType == "cue.timing_observed" {
			foundTiming = true
		}
	}
	if !foundTiming {
		t.Fatal("selection must not delete canonical timing history")
	}

	if _, err := service.SetSessionSelection(context.Background(), fixture.projectID, third.ID, store.TimingSelectionAuto, "operator"); err != nil {
		t.Fatal(err)
	}
	restored, err := service.Report(context.Background(), fixture.projectID, ReportOptions{RuntimeSnapshotID: fixture.snapshot.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := findTransition(t, restored, fixture.cues[0].ID, fixture.cues[1].ID).SampleCount; got != 3 {
		t.Fatalf("AUTO restored samples=%d want 3", got)
	}
}

func TestLiveProjectionUsesAuthoritativeNextCueAndContextLeadTime(t *testing.T) {
	fixture := newTimingFixture(t)
	fixture.rehearsal(t, "R1", 60*time.Second, 0, true)
	fixture.rehearsal(t, "R2", 65*time.Second, 0, true)
	fixture.rehearsal(t, "R3", 70*time.Second, 0, true)

	if _, err := fixture.store.CreateNote(context.Background(), fixture.projectID, store.CreateNoteParams{
		CueID: &fixture.cues[1].ID, Category: "timing", Body: "Prepare elevator operator", CreatedBy: "stage-manager",
	}); err != nil {
		t.Fatal(err)
	}
	show, err := fixture.store.CreateSessionAtPosition(context.Background(), store.CreateSessionFoundationParams{
		SnapshotID: fixture.snapshot.ID, SessionType: domain.SessionShow, Name: "Live Show",
		StartPosition: domain.SessionStartPosition{Version: 1, Kind: domain.SessionStartBeginning, Metadata: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.executeCue(t, show.ID, fixture.cues[0].ID)
	fixture.clock.Advance(40 * time.Second)

	service := New(fixture.store, fixture.clock)
	report, err := service.Report(context.Background(), fixture.projectID, ReportOptions{SessionID: show.ID, LeadTime: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	projection := report.Projection
	if projection == nil || projection.NextCue == nil || projection.NextCue.CueID != fixture.cues[1].ID {
		t.Fatalf("projection next=%+v", projection)
	}
	if projection.CurrentCue == nil || projection.CurrentCue.CueID != fixture.cues[0].ID {
		t.Fatalf("projection current=%+v", projection)
	}
	if projection.Confidence != ConfidenceMedium || projection.Pace != PaceEarly {
		t.Fatalf("projection confidence/pace=%+v", projection)
	}
	if projection.ExpectedAt == nil || projection.CurrentCueAt == nil || projection.ExpectedAt.Sub(*projection.CurrentCueAt) != 65*time.Second {
		t.Fatalf("projection expected=%+v", projection)
	}
	if !projection.ContextNotesDue || len(projection.ContextNotes) != 1 || projection.ContextNotes[0].Body != "Prepare elevator operator" {
		t.Fatalf("projection notes=%+v", projection)
	}
	if !projection.AdvisoryOnly {
		t.Fatal("live projection must be advisory-only")
	}
}

func TestSelectionMutationFailsClosedDuringActiveShow(t *testing.T) {
	fixture := newTimingFixture(t)
	rehearsal := fixture.rehearsal(t, "R1", 60*time.Second, 0, true)
	if _, err := fixture.store.CreateSessionAtPosition(context.Background(), store.CreateSessionFoundationParams{
		SnapshotID: fixture.snapshot.ID, SessionType: domain.SessionShow, Name: "Live Show",
		StartPosition: domain.SessionStartPosition{Version: 1, Kind: domain.SessionStartBeginning, Metadata: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	service := New(fixture.store, fixture.clock)
	if _, err := service.SetSessionSelection(context.Background(), fixture.projectID, rehearsal.ID, store.TimingSelectionExclude, "operator"); err == nil {
		t.Fatal("expected active SHOW to block timing training-set mutation")
	}
}

func findTransition(t *testing.T, report Report, fromID, toID string) IntervalStatistics {
	t.Helper()
	for _, transition := range report.Transitions {
		if transition.From.CueID == fromID && transition.To.CueID == toID {
			return transition.Statistics
		}
	}
	t.Fatalf("transition %s -> %s not found", fromID, toID)
	return IntervalStatistics{}
}

func findCandidate(t *testing.T, report Report, sessionID string) SessionCandidate {
	t.Helper()
	for _, candidate := range report.Sessions {
		if candidate.SessionID == sessionID {
			return candidate
		}
	}
	t.Fatalf("candidate %s not found", sessionID)
	return SessionCandidate{}
}
