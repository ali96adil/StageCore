package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/store"
)

func TestSimulationRangeStartTruthAndExplicitConfirmation(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, runtimeSnapshot, cues := createSessionFoundationFixture(t, s)

	firstRange, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID,
		SessionType: domain.SessionSimulation,
		Name:        "full-prefix range",
		StartPosition: domain.SessionStartPosition{
			Kind: domain.SessionStartRange,
			CueID: &cues[0].ID,
			Metadata: json.RawMessage(`{"end_cue_id":"` + cues[1].ID + `"}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstRange.NextCueID == nil || *firstRange.NextCueID != cues[0].ID || firstRange.StartPosition.CueID == nil || *firstRange.StartPosition.CueID != cues[0].ID {
		t.Fatalf("range position=%+v next=%v", firstRange.StartPosition, firstRange.NextCueID)
	}
	if firstRange.StateTruth.RestorationStatus != domain.SessionRestorationNotRequired || firstRange.StateTruth.ManualConfirmationRequired {
		t.Fatalf("first range truth=%+v", firstRange.StateTruth)
	}

	midRange, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID,
		SessionType: domain.SessionSimulation,
		Name:        "mid-show range",
		StartPosition: domain.SessionStartPosition{
			Kind: domain.SessionStartRange,
			CueID: &cues[1].ID,
			Metadata: json.RawMessage(`{"end_cue_id":"` + cues[1].ID + `"}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if midRange.StateTruth.RestorationStatus != domain.SessionRestorationManualConfirmationRequired || !midRange.StateTruth.ManualConfirmationRequired {
		t.Fatalf("mid-range truth=%+v", midRange.StateTruth)
	}
	if err := s.ConfirmSimulationStartState(ctx, midRange.ID); err != nil {
		t.Fatal(err)
	}
	confirmed, err := s.GetSessionFoundation(ctx, midRange.ID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.StateTruth.ManualConfirmationRequired || confirmed.StateTruth.RestorationStatus != domain.SessionRestorationUnavailable || confirmed.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("confirmed truth=%+v", confirmed.StateTruth)
	}
	events, err := s.ListEvents(ctx, midRange.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != "simulation.start_state.confirmed" {
		t.Fatalf("confirmation events=%+v", events)
	}

	if _, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID,
		SessionType: domain.SessionRehearsal,
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartRange, CueID: &cues[0].ID, Metadata: json.RawMessage(`{"end_cue_id":"` + cues[1].ID + `"}`)},
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("REHEARSAL RANGE err=%v want conflict", err)
	}
	if _, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID,
		SessionType: domain.SessionSimulation,
		StartPosition: domain.SessionStartPosition{Kind: domain.SessionStartScene},
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("SCENE err=%v want conflict", err)
	}
}

func TestSimulationRangeGuardAndTerminalBoundary(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, runtimeSnapshot, cues := createSessionFoundationFixture(t, s)
	session, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID,
		SessionType: domain.SessionSimulation,
		Name:        "single cue range",
		StartPosition: domain.SessionStartPosition{
			Kind: domain.SessionStartRange,
			CueID: &cues[0].ID,
			Metadata: json.RawMessage(`{"end_cue_id":"` + cues[0].ID + `"}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCueExecution(ctx, session.ID, cues[1].ID, "outside", "test"); err == nil {
		t.Fatal("cue outside RANGE unexpectedly accepted")
	}
	execution, err := s.CreateCueExecution(ctx, session.ID, cues[0].ID, "inside", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, execution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.SessionCompleted || loaded.LifecycleState != domain.SessionLifecycleCompleted || loaded.EndReason != "RANGE_END_REACHED" || loaded.NextCueID != nil {
		t.Fatalf("completed range=%+v", loaded)
	}
	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.EventType == "simulation.range.completed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("range completion event missing: %+v", events)
	}
}

func TestSimulationCheckpointPersistsAuthorityAndStartTruth(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	_, runtimeSnapshot, cues := createSessionFoundationFixture(t, s)
	source, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "source")
	if err != nil {
		t.Fatal(err)
	}
	execution, err := s.CreateCueExecution(ctx, source.ID, cues[0].ID, "checkpoint-progress", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionCurrentCue(ctx, source.ID, cues[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishCueExecution(ctx, execution.ID, domain.ExecutionCompleted); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := s.CreateSimulationCheckpoint(ctx, source.ID, 1, json.RawMessage(`{"version":1,"session_id":"source","runtime_snapshot_id":"`+runtimeSnapshot.ID+`","targets":[],"faults":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.RuntimeSnapshotID != runtimeSnapshot.ID || checkpoint.ProjectID != source.ProjectID || len(checkpoint.ContentHash) != 64 {
		t.Fatalf("checkpoint=%+v", checkpoint)
	}
	if checkpoint.CurrentCueID == nil || *checkpoint.CurrentCueID != cues[0].ID || checkpoint.NextCueID == nil || *checkpoint.NextCueID != cues[1].ID {
		t.Fatalf("checkpoint progress current=%v next=%v", checkpoint.CurrentCueID, checkpoint.NextCueID)
	}
	loadedCheckpoint, err := s.GetSimulationCheckpoint(ctx, checkpoint.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedCheckpoint.ContentHash != checkpoint.ContentHash {
		t.Fatalf("loaded checkpoint hash=%q want %q", loadedCheckpoint.ContentHash, checkpoint.ContentHash)
	}

	target, err := s.CreateSessionAtPosition(ctx, store.CreateSessionFoundationParams{
		SnapshotID: runtimeSnapshot.ID,
		SessionType: domain.SessionSimulation,
		Name:        "checkpoint start",
		StartPosition: domain.SessionStartPosition{
			Kind:     domain.SessionStartCheckpoint,
			Metadata: json.RawMessage(`{"checkpoint_id":"` + checkpoint.ID + `"}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if target.CurrentCueID == nil || *target.CurrentCueID != cues[0].ID || target.NextCueID == nil || *target.NextCueID != cues[1].ID {
		t.Fatalf("checkpoint target progress=%+v", target)
	}
	if target.StateTruth.RestorationStatus != domain.SessionRestorationRestorable || target.StateTruth.DesiredStateRef == nil || *target.StateTruth.DesiredStateRef != checkpoint.ID || target.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("checkpoint target truth=%+v", target.StateTruth)
	}
	if err := s.MarkSimulationCheckpointRestored(ctx, target.ID, checkpoint.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.GetSessionFoundation(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.StateTruth.VerifiedStateRef == nil || *restored.StateTruth.VerifiedStateRef != checkpoint.ID || restored.StateTruth.ManualConfirmationRequired {
		t.Fatalf("restored truth=%+v", restored.StateTruth)
	}
}
