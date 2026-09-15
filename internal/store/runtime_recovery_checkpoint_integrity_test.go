package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
	"github.com/ali96adil/StageCore/internal/recovery"
)

func TestHubRestartIgnoresCorruptSimulationCheckpointAndFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "corrupt-checkpoint")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.CreateSimulationCheckpoint(ctx, session.ID, domain.SimulationCheckpointStateContractVersion1, recoveryCheckpointTwinState(t, runtimeSnapshot.ID, "corrupt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE simulation_checkpoints
		SET twin_state_json = ?
		WHERE checkpoint_id = ?`, `{"version":1,"targets":{"tampered":true}}`, checkpoint.ID); err != nil {
		t.Fatal(err)
	}

	count, err := s.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled Sessions=%d want 1", count)
	}
	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateTruth.RestorationStatus != domain.SessionRestorationUnavailable || loaded.StateTruth.ManualConfirmationRequired {
		t.Fatalf("corrupt checkpoint recovery truth=%+v", loaded.StateTruth)
	}
	if loaded.StateTruth.DesiredStateRef != nil || loaded.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("corrupt checkpoint must not become state authority: %+v", loaded.StateTruth)
	}
	assertFailClosedSimulationRecoveryEvent(t, s, session.ID, "")
}

func TestHubRestartFallsBackToLatestValidSimulationCheckpoint(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "checkpoint-fallback")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := s.CreateSimulationCheckpoint(ctx, session.ID, domain.SimulationCheckpointStateContractVersion1, recoveryCheckpointTwinState(t, runtimeSnapshot.ID, "valid"))
	if err != nil {
		t.Fatal(err)
	}
	corrupt, err := s.CreateSimulationCheckpoint(ctx, session.ID, domain.SimulationCheckpointStateContractVersion1, recoveryCheckpointTwinState(t, runtimeSnapshot.ID, "newer"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE simulation_checkpoints
		SET twin_state_json = ?, captured_at_us = captured_at_us + 1
		WHERE checkpoint_id = ?`, `{"version":1,"targets":{"tampered":true}}`, corrupt.ID); err != nil {
		t.Fatal(err)
	}

	count, err := s.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled Sessions=%d want 1", count)
	}
	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateTruth.DesiredStateRef == nil || *loaded.StateTruth.DesiredStateRef != valid.ID {
		t.Fatalf("desired recovery checkpoint=%v want %s", loaded.StateTruth.DesiredStateRef, valid.ID)
	}
	if loaded.StateTruth.RestorationStatus != domain.SessionRestorationManualConfirmationRequired || !loaded.StateTruth.ManualConfirmationRequired {
		t.Fatalf("fallback recovery truth=%+v", loaded.StateTruth)
	}

	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.EventType != "runtime.recovery.decision" {
			continue
		}
		var payload struct {
			Disposition  recovery.Disposition `json:"disposition"`
			CheckpointID string               `json:"checkpoint_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Disposition != recovery.DispositionManualConfirmation || payload.CheckpointID != valid.ID {
			t.Fatalf("fallback recovery event=%+v", payload)
		}
		return
	}
	t.Fatal("runtime.recovery.decision event not found")
}

func TestHubRestartUnsupportedSimulationCheckpointVersionFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "unsupported-checkpoint-version")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.CreateSimulationCheckpoint(ctx, session.ID, domain.SimulationCheckpointStateContractVersion1+1, json.RawMessage(`{"version":2,"runtime_snapshot_id":"`+runtimeSnapshot.ID+`","targets":[],"faults":[]}`))
	if err != nil {
		t.Fatal(err)
	}

	count, err := s.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled Sessions=%d want 1", count)
	}
	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateTruth.RestorationStatus != domain.SessionRestorationUnavailable || loaded.StateTruth.ManualConfirmationRequired {
		t.Fatalf("unsupported checkpoint recovery truth=%+v", loaded.StateTruth)
	}
	if loaded.StateTruth.DesiredStateRef != nil || loaded.StateTruth.VerifiedStateRef != nil {
		t.Fatalf("unsupported checkpoint version must not become recovery authority: %+v", loaded.StateTruth)
	}
	assertFailClosedSimulationRecoveryEvent(t, s, session.ID, checkpoint.ID)
}

func TestHubRestartCheckpointWithWrongEmbeddedSnapshotFailsClosed(t *testing.T) {
	ctx := context.Background()
	s, _ := newStore(t)
	runtimeSnapshot, _ := createInternalRestartFixture(t, s, "INTERNAL")
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "wrong-embedded-snapshot")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.CreateSimulationCheckpoint(ctx, session.ID, domain.SimulationCheckpointStateContractVersion1, recoveryCheckpointTwinState(t, "different-runtime-snapshot", "wrong-snapshot"))
	if err != nil {
		t.Fatal(err)
	}

	count, err := s.ReconcileInterruptedRuntimeForHub(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reconciled Sessions=%d want 1", count)
	}
	loaded, err := s.GetSessionFoundation(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateTruth.RestorationStatus != domain.SessionRestorationUnavailable || loaded.StateTruth.DesiredStateRef != nil {
		t.Fatalf("wrong embedded snapshot recovery truth=%+v", loaded.StateTruth)
	}
	assertFailClosedSimulationRecoveryEvent(t, s, session.ID, checkpoint.ID)
}

func assertFailClosedSimulationRecoveryEvent(t *testing.T, s interface {
	ListEvents(context.Context, string) ([]contracts.EventEnvelope, error)
}, sessionID, rejectedCheckpointID string) {
	t.Helper()
	events, err := s.ListEvents(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.EventType != "runtime.recovery.decision" {
			continue
		}
		var payload struct {
			Disposition  recovery.Disposition `json:"disposition"`
			ReasonCode   string               `json:"reason_code"`
			CheckpointID string               `json:"checkpoint_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Disposition != recovery.DispositionAbort || payload.ReasonCode != recovery.ReasonSimulationRestartFailClosed || payload.CheckpointID != "" {
			t.Fatalf("fail-closed simulation recovery event=%+v rejected_checkpoint=%s", payload, rejectedCheckpointID)
		}
		return
	}
	t.Fatal("runtime.recovery.decision event not found")
}
