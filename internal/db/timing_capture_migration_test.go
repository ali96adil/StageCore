package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/db/migrations"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
	"github.com/pressly/goose/v3"
)

type mutableTimingClock struct {
	now time.Time
}

func (c *mutableTimingClock) Now() time.Time {
	return c.now.UTC()
}

func (c *mutableTimingClock) Set(t time.Time) {
	c.now = t.UTC()
}

func TestTimingCaptureMigrationRecordsCanonicalRawObservations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), FileName)
	database, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	defer database.Close()
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := VerifyConnection(ctx, database); err != nil {
		t.Fatal(err)
	}

	migrationFS, err := fs.Sub(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	migrateTimingTestDB(t, database, migrationFS, 13)

	base := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
	clock := &mutableTimingClock{now: base}
	s := store.New(database, clock)
	_, revision, err := s.CreateProject(ctx, store.CreateProjectParams{Name: "F-028 Timing Capture", CreatedBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	cues := make([]domain.Cue, 0, 3)
	for i, name := range []string{"Cue One", "Cue Two", "Cue Three"} {
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

	// An execution that predates F-028 must remain historical truth. Migration
	// 14 does not invent a derived path observation for it.
	historical, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "pre-f028")
	if err != nil {
		t.Fatal(err)
	}
	clock.Set(base.Add(5 * time.Second))
	historicalExecution, err := s.CreateCueExecution(ctx, historical.ID, cues[0].ID, "historical-correlation", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, historical.ID, cues[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, historicalExecution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	migrateTimingTestDB(t, database, migrationFS, 14)
	var historicalTimingEvents int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_records WHERE session_id = ? AND event_type = ?`, historical.ID, contracts.CueTimingObservedEventType).Scan(&historicalTimingEvents); err != nil {
		t.Fatal(err)
	}
	if historicalTimingEvents != 0 {
		t.Fatalf("historical timing events=%d want 0", historicalTimingEvents)
	}

	clock.Set(base.Add(10 * time.Second))
	rehearsal, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "timing-v1")
	if err != nil {
		t.Fatal(err)
	}

	clock.Set(base.Add(12 * time.Second))
	startCommandID := reserveTimingCommand(t, ctx, s, runtimeSnapshot, base.Add(11*time.Second), "corr-start")
	startExecution, err := s.CreateCueExecution(ctx, rehearsal.ID, cues[0].ID, "corr-start", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, rehearsal.ID, cues[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, startExecution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	clock.Set(base.Add(27 * time.Second))
	jumpCommandID := reserveTimingCommand(t, ctx, s, runtimeSnapshot, base.Add(26*time.Second), "corr-jump")
	jumpExecution, err := s.CreateCueExecution(ctx, rehearsal.ID, cues[2].ID, "corr-jump", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, rehearsal.ID, cues[2].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, jumpExecution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	clock.Set(base.Add(32 * time.Second))
	repeatCommandID := reserveTimingCommand(t, ctx, s, runtimeSnapshot, base.Add(31*time.Second), "corr-repeat")
	repeatExecution, err := s.CreateCueExecution(ctx, rehearsal.ID, cues[2].ID, "corr-repeat", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, rehearsal.ID, cues[2].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, repeatExecution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	clock.Set(base.Add(40 * time.Second))
	backCommandID := reserveTimingCommand(t, ctx, s, runtimeSnapshot, base.Add(39*time.Second), "corr-back")
	backExecution, err := s.CreateCueExecution(ctx, rehearsal.ID, cues[0].ID, "corr-back", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, rehearsal.ID, cues[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, backExecution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	events, err := s.ListEvents(ctx, rehearsal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("timing events=%d want 4", len(events))
	}
	for _, event := range events {
		if event.EventType != contracts.CueTimingObservedEventType {
			t.Fatalf("event type=%s", event.EventType)
		}
		if event.Source != "hub.timing_capture" || event.Priority != "P2" {
			t.Fatalf("event source=%s priority=%s", event.Source, event.Priority)
		}
	}

	start := decodeTimingObservation(t, events[0])
	if events[0].CausationID != startCommandID {
		t.Fatalf("start causation=%s want %s", events[0].CausationID, startCommandID)
	}
	if start.CaptureVersion != contracts.CueTimingCaptureVersion1 || start.Quality != contracts.CueTimingQualityRawUnassessed {
		t.Fatalf("start capture=%+v", start)
	}
	if start.Path.Kind != contracts.CueTimingPathStart || start.Path.FromCueID != nil || start.Path.ToCueID != cues[0].ID {
		t.Fatalf("start path=%+v", start.Path)
	}
	if start.RequestIssuedAtUS == nil || start.RequestToStartUS == nil || *start.RequestToStartUS != int64(time.Second/time.Microsecond) {
		t.Fatalf("start request timing=%+v", start)
	}
	if start.SessionElapsedUS != int64(2*time.Second/time.Microsecond) || start.PreviousCueExecutionID != nil || start.CueToCueElapsedUS != nil {
		t.Fatalf("start elapsed=%+v", start)
	}
	if start.Clock.Basis != contracts.CueTimingClockHubUTCWall || start.Clock.Health != contracts.CueTimingClockUnassessed || start.Clock.IntervalScope != contracts.CueTimingIntervalSingleHub || start.Clock.RequestBasis != contracts.CueTimingRequestEnvelopeUTC {
		t.Fatalf("start clock=%+v", start.Clock)
	}

	jump := decodeTimingObservation(t, events[1])
	if events[1].CausationID != jumpCommandID || jump.Path.Kind != contracts.CueTimingPathForwardJump {
		t.Fatalf("jump event=%+v observation=%+v", events[1], jump)
	}
	if len(jump.Path.SkippedCueIDs) != 1 || jump.Path.SkippedCueIDs[0] != cues[1].ID {
		t.Fatalf("jump skipped=%v", jump.Path.SkippedCueIDs)
	}
	if jump.PreviousCueExecutionID == nil || *jump.PreviousCueExecutionID != startExecution.ID || jump.PreviousCueID == nil || *jump.PreviousCueID != cues[0].ID {
		t.Fatalf("jump previous=%+v", jump)
	}
	if jump.CueToCueElapsedUS == nil || *jump.CueToCueElapsedUS != int64(15*time.Second/time.Microsecond) {
		t.Fatalf("jump interval=%v", jump.CueToCueElapsedUS)
	}

	repeat := decodeTimingObservation(t, events[2])
	if events[2].CausationID != repeatCommandID || repeat.Path.Kind != contracts.CueTimingPathRepeat || len(repeat.Path.SkippedCueIDs) != 0 {
		t.Fatalf("repeat=%+v", repeat)
	}

	back := decodeTimingObservation(t, events[3])
	if events[3].CausationID != backCommandID || back.Path.Kind != contracts.CueTimingPathBackJump || len(back.Path.SkippedCueIDs) != 0 {
		t.Fatalf("back=%+v", back)
	}

	clock.Set(base.Add(50 * time.Second))
	showSession, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "show-start-at-cue")
	if err != nil {
		t.Fatal(err)
	}
	showExecution, err := s.CreateCueExecution(ctx, showSession.ID, cues[2].ID, "show-corr", "test.operator")
	if err != nil {
		t.Fatal(err)
	}
	showEvents, err := s.ListEvents(ctx, showSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(showEvents) != 1 {
		t.Fatalf("show timing events=%d want 1", len(showEvents))
	}
	showStart := decodeTimingObservation(t, showEvents[0])
	if showStart.Path.Kind != contracts.CueTimingPathStartAtCue || len(showStart.Path.SkippedCueIDs) != 2 || showStart.CueExecutionID != showExecution.ID {
		t.Fatalf("show start=%+v", showStart)
	}

	clock.Set(base.Add(60 * time.Second))
	simulation, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "simulation-no-f028")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueExecution(ctx, simulation.ID, cues[0].ID, "simulation-corr", "test.operator"); err != nil {
		t.Fatal(err)
	}
	simulationEvents, err := s.ListEvents(ctx, simulation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(simulationEvents) != 0 {
		t.Fatalf("simulation timing events=%v", simulationEvents)
	}
}

func migrateTimingTestDB(t *testing.T, database *sql.DB, migrationFS fs.FS, version int64) {
	t.Helper()
	migrationMu.Lock()
	defer migrationMu.Unlock()
	goose.SetBaseFS(migrationFS)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(database, ".", version); err != nil {
		t.Fatal(err)
	}
}

func reserveTimingCommand(t *testing.T, ctx context.Context, s *store.Store, runtimeSnapshot domain.RuntimeSnapshot, issuedAt time.Time, correlationID string) string {
	t.Helper()
	commandID, err := stageid.New()
	if err != nil {
		t.Fatal(err)
	}
	_, reserved, err := s.ReserveCommand(ctx, contracts.CommandEnvelope{
		CommandID:         commandID,
		CommandType:       "cue.go",
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          issuedAt,
		ProjectID:         runtimeSnapshot.ProjectID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer:            "test.operator",
		CorrelationID:     correlationID,
		Priority:          "P1",
		Payload:           json.RawMessage(`{}`),
	})
	if err != nil || !reserved {
		t.Fatalf("reserve timing command reserved=%v err=%v", reserved, err)
	}
	return commandID
}

func decodeTimingObservation(t *testing.T, event contracts.EventEnvelope) contracts.CueTimingObservation {
	t.Helper()
	var observation contracts.CueTimingObservation
	if err := json.Unmarshal(event.Payload, &observation); err != nil {
		t.Fatalf("decode timing observation: %v payload=%s", err, event.Payload)
	}
	return observation
}
