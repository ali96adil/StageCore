package timingintelligence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time          { return c.now.UTC() }
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
		SectionFromCueID:  fixture.cues[0].ID,
		SectionToCueID:    fixture.cues[2].ID,
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
	if report.Section.Statistics.MedianUS != (105 * time.Second).Microseconds() {
		t.Fatalf("section median=%d want %d", report.Section.Statistics.MedianUS, (105 * time.Second).Microseconds())
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

func TestTimingCapturePausePreservesRawEventsAndReanchorsTraining(t *testing.T) {
	f := newTimingFixture(t)
	ctx := context.Background()
	service := New(f.store, f.clock)
	rehearsal, err := f.store.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: f.snapshot.ID, SessionType: domain.SessionRehearsal, Name: "Pause rehearsal",
		StartPosition: domain.SessionStartPosition{Version: 1, Kind: domain.SessionStartBeginning, Metadata: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	set := func(paused bool) {
		t.Helper()
		if err := service.SetTimingCapturePaused(ctx, f.projectID, rehearsal.ID, "operator", paused); err != nil {
			t.Fatal(err)
		}
	}
	f.executeCue(t, rehearsal.ID, f.cues[0].ID)
	set(true)
	set(true)
	f.clock.Advance(15 * time.Second)
	f.executeCue(t, rehearsal.ID, f.cues[1].ID)
	set(false)
	f.clock.Advance(20 * time.Second)
	f.executeCue(t, rehearsal.ID, f.cues[2].ID)
	f.clock.Advance(time.Second)
	f.executeCue(t, rehearsal.ID, f.cues[0].ID)
	f.clock.Advance(2 * time.Second)
	f.executeCue(t, rehearsal.ID, f.cues[1].ID)
	if err := f.store.EndSessionLifecycle(ctx, rehearsal.ID, domain.SessionLifecycleCompleted, "test"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetTimingCapturePaused(ctx, f.projectID, rehearsal.ID, "operator", true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ended session accepted mutation: %v", err)
	}
	records, err := f.store.ListTimingObservationRecords(ctx, f.projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 5 || records[0].TrainingExcluded || !records[1].TrainingExcluded || !records[2].TrainingExcluded || records[3].TrainingExcluded || records[4].TrainingExcluded {
		t.Fatalf("unexpected retained observations: %+v", records)
	}
	report, err := service.Report(ctx, f.projectID, ReportOptions{RuntimeSnapshotID: f.snapshot.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Transitions) != 1 || report.Transitions[0].Statistics.SampleCount != 1 || report.Transitions[0].Statistics.MedianUS != int64(2*time.Second/time.Microsecond) {
		t.Fatalf("training used interrupted observations: %+v", report.Transitions)
	}
	events, err := f.store.ListEvents(ctx, rehearsal.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.EventType == "timing.capture_paused" || event.EventType == "timing.capture_resumed" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected two journal changes, got %d", count)
	}
}

func TestRetrospectiveTimingSegmentsAreScopedOverlappingAndReversible(t *testing.T) {
	f := newTimingFixture(t)
	ctx := context.Background()
	first := f.rehearsal(t, "First", time.Minute, 40*time.Second, false)
	second := f.rehearsal(t, "Second", 80*time.Second, 50*time.Second, false)
	service := New(f.store, f.clock)
	options := ReportOptions{
		RuntimeSnapshotID: f.snapshot.ID, ReviewSessionID: first.ID,
		SectionFromCueID: f.cues[0].ID, SectionToCueID: f.cues[2].ID,
	}
	report := func() Report {
		t.Helper()
		got, err := service.Report(ctx, f.projectID, options)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	initial := report()
	if len(initial.SegmentReview.Points) != 3 || initial.Section.Statistics.SampleCount != 2 {
		t.Fatalf("initial=%+v review=%+v", initial, initial.SegmentReview)
	}
	points := initial.SegmentReview.Points
	set := func(sessionID string, segment store.TimingSegment) error {
		return service.SetTimingSegment(ctx, f.projectID, sessionID, "operator", segment)
	}
	single := store.TimingSegment{First: points[1].Sequence, Last: points[1].Sequence, Excluded: true}
	span := store.TimingSegment{First: points[1].Sequence, Last: points[2].Sequence, Excluded: true}
	if err := set(second.ID, single); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("foreign session accepted boundaries: %v", err)
	}
	if err := service.SetTimingSegment(ctx, "foreign-project", first.ID, "operator", single); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign project accepted segment: %v", err)
	}
	for _, segment := range []store.TimingSegment{single, span, span} {
		if err := set(first.ID, segment); err != nil {
			t.Fatal(err)
		}
	}
	single.Excluded = false
	if err := set(first.ID, single); err != nil {
		t.Fatal(err)
	}
	masked := report()
	if len(masked.SegmentReview.Excluded) != 1 || len(masked.SegmentReview.Points) != 3 ||
		masked.Section.Statistics.SampleCount != 1 ||
		findTransition(t, masked, f.cues[0].ID, f.cues[1].ID).MedianUS != (80*time.Second).Microseconds() {
		t.Fatalf("masked=%+v review=%+v", masked, masked.SegmentReview)
	}
	span.Excluded = false
	if err := set(first.ID, span); err != nil {
		t.Fatal(err)
	}
	restored := report()
	if len(restored.SegmentReview.Excluded) != 0 || restored.Section.Statistics.SampleCount != 2 {
		t.Fatalf("restored=%+v review=%+v", restored, restored.SegmentReview)
	}
	if _, err := f.store.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: f.snapshot.ID, SessionType: domain.SessionShow, Name: "Live Show",
		StartPosition: domain.SessionStartPosition{Version: 1, Kind: domain.SessionStartBeginning, Metadata: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	span.Excluded = true
	if err := set(first.ID, span); !errors.Is(err, domain.ErrShowConfigurationLocked) {
		t.Fatalf("live SHOW allowed segment edits: %v", err)
	}

}

func TestNewRehearsalTimingRequiresReversibleApproval(t *testing.T) {
	f := newTimingFixture(t)
	ctx := context.Background()
	service := New(f.store, f.clock)
	f.rehearsal(t, "Legacy automatic", time.Minute, 0, true)

	start := func(purpose domain.RehearsalPurpose, interval time.Duration) domain.Session {
		t.Helper()
		session, err := f.store.CreateSessionWithPurpose(ctx, f.snapshot.ID, domain.SessionRehearsal, string(purpose), purpose)
		if err != nil {
			t.Fatal(err)
		}
		f.executeCue(t, session.ID, f.cues[0].ID)
		f.clock.Advance(interval)
		f.executeCue(t, session.ID, f.cues[1].ID)
		return session
	}
	end := func(sessionID string) {
		t.Helper()
		if err := f.store.EndSessionLifecycle(ctx, sessionID, domain.SessionLifecycleStopped, "test"); err != nil {
			t.Fatal(err)
		}
	}
	report := func(sessionID string, wantCount int, effective bool) SessionCandidate {
		t.Helper()
		got, err := service.Report(ctx, f.projectID, ReportOptions{RuntimeSnapshotID: f.snapshot.ID})
		if err != nil {
			t.Fatal(err)
		}
		stats := findTransition(t, got, f.cues[0].ID, f.cues[1].ID)
		candidate := findCandidate(t, got, sessionID)
		if stats.SampleCount != wantCount || candidate.Effective != effective || candidate.ObservationCount != 2 {
			t.Fatalf("candidate=%+v statistics=%+v", candidate, stats)
		}
		return candidate
	}
	selectMode := func(sessionID string, mode store.TimingSelectionMode) {
		t.Helper()
		if _, err := service.SetSessionSelection(ctx, f.projectID, sessionID, mode, "operator"); err != nil {
			t.Fatal(err)
		}
	}

	preview := start(domain.RehearsalPreview, 75*time.Second)
	end(preview.ID)
	report(preview.ID, 1, false)

	capture := start(domain.RehearsalTimingCapture, 80*time.Second)
	selectMode(capture.ID, store.TimingSelectionInclude)
	pending := report(capture.ID, 1, false)
	if pending.SelectionMode != store.TimingSelectionInclude || pending.Reason != "approved, pending completed or stopped rehearsal" {
		t.Fatalf("active approval=%+v", pending)
	}
	end(capture.ID)
	report(capture.ID, 2, true)
	selectMode(capture.ID, store.TimingSelectionAuto)
	report(capture.ID, 1, false)
	selectMode(preview.ID, store.TimingSelectionInclude)
	report(preview.ID, 2, true)
	selectMode(preview.ID, store.TimingSelectionExclude)
	report(preview.ID, 1, false)
}

func TestTimingMutationsRollBackAfterCommitAuthorityEnds(t *testing.T) {
	f := newTimingFixture(t)
	ctx := context.Background()
	finished := f.rehearsal(t, "Finished rehearsal", time.Minute, time.Minute, false)
	active, err := f.store.CreateSession(ctx, f.snapshot.ID, domain.SessionRehearsal, "Active rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	service := New(f.store, f.clock)
	report, err := service.Report(ctx, f.projectID, ReportOptions{
		RuntimeSnapshotID: f.snapshot.ID, ReviewSessionID: finished.ID,
		SectionFromCueID: f.cues[0].ID, SectionToCueID: f.cues[2].ID,
	})
	if err != nil || len(report.SegmentReview.Points) == 0 {
		t.Fatalf("timing observations=%+v err=%v", report.SegmentReview, err)
	}
	point := report.SegmentReview.Points[0].Sequence
	denied := errors.New("timing authority ended at commit")
	guarded := capability.WithWriteAuthorization(ctx, func(context.Context, *sql.Tx) error {
		return denied
	})
	if _, err := service.SetSessionSelection(guarded, f.projectID, finished.ID, store.TimingSelectionExclude, "operator"); !errors.Is(err, denied) {
		t.Fatalf("selection err=%v", err)
	}
	if err := service.SetTimingCapturePaused(guarded, f.projectID, active.ID, "operator", true); !errors.Is(err, denied) {
		t.Fatalf("pause err=%v", err)
	}
	segment := store.TimingSegment{First: point, Last: point, Excluded: true}
	if err := service.SetTimingSegment(guarded, f.projectID, finished.ID, "operator", segment); !errors.Is(err, denied) {
		t.Fatalf("segment err=%v", err)
	}
	selections, err := f.store.ListTimingSessionSelections(ctx, f.projectID)
	if err != nil || len(selections) != 0 {
		t.Fatalf("denied selections persisted: %v err=%v", selections, err)
	}
	paused, err := f.store.TimingCapturePausedSessions(ctx, f.projectID)
	if err != nil || paused[active.ID] {
		t.Fatalf("denied pause persisted: %v err=%v", paused, err)
	}
	segments, err := f.store.ListTimingSegments(ctx, f.projectID)
	if err != nil || len(segments[finished.ID]) != 0 {
		t.Fatalf("denied segments persisted: %v err=%v", segments, err)
	}
}

func TestTimingApprovalRejectsConcurrentSelectionAndWrongSnapshot(t *testing.T) {
	f := newTimingFixture(t)
	ctx := context.Background()
	session := f.rehearsal(t, "Review race", time.Minute, time.Minute, false)
	service := New(f.store, f.clock)
	wrong := store.WithTimingSelectionReview(ctx, store.TimingSelectionReview{
		RuntimeSnapshotID: "other-snapshot", Mode: store.TimingSelectionAuto,
	})
	if _, err := service.SetSessionSelection(wrong, f.projectID, session.ID, store.TimingSelectionInclude, "reviewer"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("wrong snapshot err=%v", err)
	}
	reviewed := store.WithTimingSelectionReview(ctx, store.TimingSelectionReview{
		RuntimeSnapshotID: f.snapshot.ID, Mode: store.TimingSelectionAuto,
	})
	results := make(chan error, 2)
	for _, mode := range []store.TimingSelectionMode{store.TimingSelectionInclude, store.TimingSelectionExclude} {
		go func() {
			_, err := service.SetSessionSelection(reviewed, f.projectID, session.ID, mode, "reviewer")
			results <- err
		}()
	}
	var succeeded, conflicted int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrConflict):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("successful=%d conflicted=%d", succeeded, conflicted)
	}
}
