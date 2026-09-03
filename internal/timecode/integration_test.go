package timecode

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/cueengine"
	"github.com/ali96adil/StageCore/internal/db"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/preflight"
	"github.com/ali96adil/StageCore/internal/snapshot"
	"github.com/ali96adil/StageCore/internal/store"
)

func newIntegrationStore(t *testing.T) (*store.Store, *db.Handle) {
	t.Helper()
	h, err := db.Open(context.Background(), db.Config{DataRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return store.New(h.DB, clock.Real{}), h
}

func TestInternalRuntimeSnapshotBindingFiresOnceAndSurvivesRuntimeRestart(t *testing.T) {
	ctx := context.Background()
	stageStore, handle := newIntegrationStore(t)
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Timecode Show"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:   project.ID,
		LogicalName: "SHOW-CLOCK",
		LogicalType: SourceTargetLogicalType,
		ProjectConfig: json.RawMessage(`{
			"source_id":"show-clock",
			"kind":"INTERNAL",
			"rate":"30",
			"offset_frames":0,
			"start_timecode":"00:00:00:00"
		}`),
	}); err != nil {
		t.Fatal(err)
	}
	cue, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID:   revision.ID,
		DisplayLabel: "1",
		Name:         "Intro",
		OrderIndex:   0,
		Enabled:      true,
		ExecutionPolicy: json.RawMessage(`{
			"timecode":{
				"binding_id":"intro-at-10",
				"at":"00:00:00:10",
				"expiry_frames":2,
				"enabled":true
			}
		}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stageStore.CreateCueWithActions(ctx, domain.Cue{
		RevisionID: revision.ID, DisplayLabel: "2", Name: "Second", OrderIndex: 1, Enabled: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, manifest, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ResolveManifestConfiguration(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.SourceID != "show-clock" || cfg.Source.Kind != SourceInternal || len(cfg.Bindings) != 1 || cfg.Bindings[0].CueID != cue.ID || cfg.Bindings[0].TargetFrame != 10 {
		t.Fatalf("unexpected immutable timecode configuration: %#v", cfg)
	}

	// Live Project configuration may move on after publication. The authoritative
	// runtime must continue to use the source sealed into this exact Snapshot.
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID:   project.ID,
		LogicalName: "FUTURE-CLOCK",
		LogicalType: SourceTargetLogicalType,
		ProjectConfig: json.RawMessage(`{
			"source_id":"future-clock",
			"kind":"INTERNAL",
			"rate":"25"
		}`),
	}); err != nil {
		t.Fatal(err)
	}

	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionShow, "Show")
	if err != nil {
		t.Fatal(err)
	}
	engine := cueengine.New(stageStore)
	runtime := NewRuntimeService(stageStore, engine)
	before, err := runtime.Summary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Configuration.Source.SourceID != "show-clock" {
		t.Fatalf("published source drifted to %q", before.Configuration.Source.SourceID)
	}

	start := time.Now().UTC()
	if err := runtime.StartInternal(ctx, session.ID, start); err != nil {
		t.Fatal(err)
	}
	locked, err := runtime.Summary(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !locked.ShowLocked {
		t.Fatal("SHOW timecode source must be locked before the first automatic observation")
	}
	first, err := runtime.PollInternal(ctx, session.ID, start.Add(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if first.Health.State != HealthHealthy || len(first.Commands) != 0 {
		t.Fatalf("unexpected pre-trigger observation: %#v", first)
	}
	triggered, err := runtime.PollInternal(ctx, session.ID, start.Add(334*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(triggered.Commands) != 1 || triggered.Commands[0].Status != contracts.CommandCompleted {
		t.Fatalf("timecode command result=%#v", triggered.Commands)
	}
	if len(triggered.Decisions) != 1 || triggered.Decisions[0].State != DecisionFire || triggered.Decisions[0].Command == nil {
		t.Fatalf("timecode decision=%#v", triggered.Decisions)
	}
	commandID := triggered.Decisions[0].Command.CommandID
	updatedSession, err := stageStore.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedSession.CurrentCueID == nil || *updatedSession.CurrentCueID != cue.ID {
		t.Fatalf("current cue=%v want %s", updatedSession.CurrentCueID, cue.ID)
	}
	assertRuntimeCounts(t, ctx, stageStore, handle, session.ID, 1, 1)

	// Recreate both the cue engine and the timecode runtime. A duplicate command
	// identity is still terminal in command_records and cannot create a second cue execution.
	engineAfterRestart := cueengine.New(stageStore)
	duplicatePayload, _ := json.Marshal(cueengine.CueGoPayload{RequestedNextCueID: &cue.ID})
	duplicate := engineAfterRestart.ExecuteCueGo(ctx, session.ID, contracts.CommandEnvelope{
		CommandID:         commandID,
		CommandType:       cueengine.CueGoCommandType,
		SchemaVersion:     contracts.SchemaVersion1,
		IssuedAt:          time.Now().UTC(),
		ProjectID:         project.ID,
		RuntimeSnapshotID: runtimeSnapshot.ID,
		Issuer:            "timecode:show-clock",
		Priority:          "P1",
		IdempotencyKey:    commandID,
		Payload:           duplicatePayload,
	})
	if duplicate.Status != contracts.CommandCompleted {
		t.Fatalf("durable duplicate result=%#v", duplicate)
	}
	assertRuntimeCounts(t, ctx, stageStore, handle, session.ID, 1, 1)

	// A fresh timecode scheduler crossing the same frame is also inhibited by the
	// persisted ordered-cue authority because Intro is no longer the next cue.
	runtimeAfterRestart := NewRuntimeService(stageStore, engineAfterRestart)
	restartAt := time.Now().UTC()
	if err := runtimeAfterRestart.StartInternal(ctx, session.ID, restartAt); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeAfterRestart.PollInternal(ctx, session.ID, restartAt.Add(300*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	replayed, err := runtimeAfterRestart.PollInternal(ctx, session.ID, restartAt.Add(334*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed.Commands) != 0 || len(replayed.Decisions) != 1 || replayed.Decisions[0].State != DecisionInhibited {
		t.Fatalf("restart replay must be inhibited: %#v", replayed)
	}
	assertRuntimeCounts(t, ctx, stageStore, handle, session.ID, 1, 1)
	assertEventCount(t, ctx, handle, session.ID, "timecode.binding.triggered", 1)
	assertEventCount(t, ctx, handle, session.ID, "timecode.binding.result", 1)
	assertEventCount(t, ctx, handle, session.ID, "timecode.binding.inhibited", 1)
}

type staticPreflightBase struct {
	report preflight.Report
}

func (b staticPreflightBase) Evaluate(context.Context, string, string) (preflight.Report, error) {
	return b.report, nil
}

func TestMTCPreflightTracksLiveHealth(t *testing.T) {
	ctx := context.Background()
	stageStore, _ := newIntegrationStore(t)
	project, runtimeSnapshot := createTimecodeSnapshot(t, ctx, stageStore, SourceMTC, "30", "mtc-main")
	runtime := NewRuntimeService(stageStore, nil)
	service := NewPreflightService(staticPreflightBase{report: preflight.Report{
		Status: preflight.Pass, ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID, EvaluatedAt: time.Now().UTC(), Checks: []preflight.Check{},
	}}, runtime)

	missing, err := service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Status != preflight.Warn || checkStatus(missing, "timecode.source.mtc") != preflight.Warn {
		t.Fatalf("missing MTC preflight=%#v", missing)
	}

	session, err := stageStore.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionRehearsal, "MTC rehearsal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.IngestFrame(ctx, session.ID, "mtc-main", SourceMTC, Rate30, 0, time.Now().UTC(), 0, false); err != nil {
		t.Fatal(err)
	}
	healthy, err := service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if healthy.Status != preflight.Pass || checkStatus(healthy, "timecode.source.mtc") != preflight.Pass {
		t.Fatalf("healthy MTC preflight=%#v", healthy)
	}
}

func TestPreflightBlocksUnrepresentableMTCRate(t *testing.T) {
	ctx := context.Background()
	stageStore, _ := newIntegrationStore(t)
	project, runtimeSnapshot := createTimecodeSnapshot(t, ctx, stageStore, SourceMTC, "23.976", "mtc-film")
	service := NewPreflightService(staticPreflightBase{report: preflight.Report{
		Status: preflight.Pass, ProjectID: project.ID, RuntimeSnapshotID: runtimeSnapshot.ID, EvaluatedAt: time.Now().UTC(), Checks: []preflight.Check{},
	}}, NewRuntimeService(stageStore, nil))

	report, err := service.Evaluate(ctx, project.ID, runtimeSnapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != preflight.Block || checkStatus(report, "timecode.source.mtc.rate") != preflight.Block {
		t.Fatalf("unrepresentable MTC preflight=%#v", report)
	}
}

func createTimecodeSnapshot(t *testing.T, ctx context.Context, stageStore *store.Store, kind SourceKind, rate, sourceID string) (domain.Project, domain.RuntimeSnapshot) {
	t.Helper()
	project, revision, err := stageStore.CreateProject(ctx, store.CreateProjectParams{Name: "Timecode Preflight"})
	if err != nil {
		t.Fatal(err)
	}
	configuration, _ := json.Marshal(map[string]any{
		"source_id": sourceID,
		"kind":      string(kind),
		"rate":      rate,
	})
	if _, err := stageStore.CreateAlias(ctx, domain.ProjectDeviceAlias{
		ProjectID: project.ID, LogicalName: "TIMECODE", LogicalType: SourceTargetLogicalType, ProjectConfig: configuration,
	}); err != nil {
		t.Fatal(err)
	}
	if err := stageStore.SetRevisionStatus(ctx, revision.ID, domain.RevisionValidated); err != nil {
		t.Fatal(err)
	}
	runtimeSnapshot, _, err := snapshot.NewBuilder(stageStore).Create(ctx, revision.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	return project, runtimeSnapshot
}

func assertRuntimeCounts(t *testing.T, ctx context.Context, stageStore *store.Store, handle *db.Handle, sessionID string, cueExecutions, commandRecords int) {
	t.Helper()
	executions, err := stageStore.ListCueExecutions(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(executions) != cueExecutions {
		t.Fatalf("cue executions=%d want %d", len(executions), cueExecutions)
	}
	var commands int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM command_records`).Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if commands != commandRecords {
		t.Fatalf("command records=%d want %d", commands, commandRecords)
	}
}

func assertEventCount(t *testing.T, ctx context.Context, handle *db.Handle, sessionID, eventType string, want int) {
	t.Helper()
	var count int
	if err := handle.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_records WHERE session_id = ? AND event_type = ?`, sessionID, eventType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("event %s count=%d want %d", eventType, count, want)
	}
}

func checkStatus(report preflight.Report, key string) preflight.Status {
	for _, check := range report.Checks {
		if check.Key == key {
			return check.Status
		}
	}
	return ""
}
