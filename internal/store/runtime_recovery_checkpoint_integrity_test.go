package store_test

import (
	"context"
	"encoding/json"
	"testing"

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
	checkpoint, err := s.CreateSimulationCheckpoint(ctx, session.ID, 1, json.RawMessage(`{"version":1,"targets":{},"faults":[]}`))
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

	events, err := s.ListEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.EventType != "runtime.recovery.decision" {
			continue
		}
		var payload struct {
			Disposition recovery.Disposition `json:"disposition"`
			ReasonCode  string               `json:"reason_code"`
			CheckpointID string               `json:"checkpoint_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Disposition != recovery.DispositionAbort || payload.ReasonCode != recovery.ReasonSimulationRestartFailClosed || payload.CheckpointID != "" {
			t.Fatalf("corrupt checkpoint recovery event=%+v", payload)
		}
		return
	}
	t.Fatal("runtime.recovery.decision event not found")
}
