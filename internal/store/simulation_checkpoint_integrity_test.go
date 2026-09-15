package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ali96adil/StageCore/internal/domain"
)

func TestSimulationCheckpointRejectsTamperedTwinState(t *testing.T) {
	ctx := context.Background()
	s, handle := newStore(t)
	_, runtimeSnapshot, _ := createSessionFoundationFixture(t, s)
	session, err := s.CreateSession(ctx, runtimeSnapshot.ID, domain.SessionSimulation, "integrity-source")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.CreateSimulationCheckpoint(ctx, session.ID, 1, json.RawMessage(`{"version":1,"runtime_snapshot_id":"`+runtimeSnapshot.ID+`","targets":[],"faults":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.DB.ExecContext(ctx, `
		UPDATE simulation_checkpoints
		SET twin_state_json = ?
		WHERE checkpoint_id = ?`, `{"version":1,"targets":[{"tampered":true}]}`, checkpoint.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetSimulationCheckpoint(ctx, checkpoint.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("tampered checkpoint err=%v want conflict", err)
	}
}
