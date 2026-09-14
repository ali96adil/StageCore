package simulator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/domain"
)

type checkpointMemoryStore struct {
	sessions map[string]domain.Session
	items    map[string]domain.SimulationCheckpoint
	marked   bool
	markErr  error
}

func (s *checkpointMemoryStore) GetSessionFoundation(_ context.Context, id string) (domain.Session, error) {
	value, ok := s.sessions[id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return value, nil
}

func (s *checkpointMemoryStore) CreateSimulationCheckpoint(_ context.Context, sessionID string, version int, state json.RawMessage) (domain.SimulationCheckpoint, error) {
	session, ok := s.sessions[sessionID]
	if !ok {
		return domain.SimulationCheckpoint{}, domain.ErrNotFound
	}
	checkpoint := domain.SimulationCheckpoint{
		ID:                   "checkpoint-1",
		SourceSessionID:      sessionID,
		ProjectID:            session.ProjectID,
		RuntimeSnapshotID:    session.RuntimeSnapshotID,
		StateContractVersion: version,
		CapturedAt:           time.Unix(1, 0).UTC(),
		CurrentCueID:         session.CurrentCueID,
		LastCompletedCueID:   session.LastCompletedCueID,
		NextCueID:            session.NextCueID,
		TwinState:            append(json.RawMessage(nil), state...),
		ContentHash:          "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	s.items[checkpoint.ID] = checkpoint
	return checkpoint, nil
}

func (s *checkpointMemoryStore) GetSimulationCheckpoint(_ context.Context, id string) (domain.SimulationCheckpoint, error) {
	value, ok := s.items[id]
	if !ok {
		return domain.SimulationCheckpoint{}, domain.ErrNotFound
	}
	return value, nil
}

func (s *checkpointMemoryStore) MarkSimulationCheckpointRestored(_ context.Context, sessionID, checkpointID string) error {
	if s.markErr != nil {
		return s.markErr
	}
	if _, ok := s.sessions[sessionID]; !ok {
		return domain.ErrNotFound
	}
	if _, ok := s.items[checkpointID]; !ok {
		return domain.ErrNotFound
	}
	s.marked = true
	return nil
}

func activeSimulation(id string) domain.Session {
	return domain.Session{
		ID:                id,
		ProjectID:         "project-1",
		RuntimeSnapshotID: "snapshot-1",
		Type:              domain.SessionSimulation,
		Status:            domain.SessionActive,
		LifecycleState:    domain.SessionLifecycleActive,
	}
}

func TestCheckpointManagerCaptureRestoreDoesNotReplayExecution(t *testing.T) {
	ctx := context.Background()
	source := activeSimulation("source-session")
	target := activeSimulation("target-session")
	store := &checkpointMemoryStore{
		sessions: map[string]domain.Session{source.ID: source, target.ID: target},
		items:    map[string]domain.SimulationCheckpoint{},
	}
	twin := NewDigitalTwin()
	if err := twin.BindSession(source); err != nil {
		t.Fatal(err)
	}
	result := twin.Execute(ctx, capability.Request{
		SessionID:         source.ID,
		ProjectID:         source.ProjectID,
		RuntimeSnapshotID: source.RuntimeSnapshotID,
		Capability:        "sim.test",
		Target:            &capability.Target{Ref: "tablet.3", LogicalType: "tablet"},
		Parameters:        json.RawMessage(`{"simulation":{"behavior":"COMPLETE"}}`),
	})
	if result.Result != domain.ExecutionCompleted {
		t.Fatalf("source execution=%#v", result)
	}
	if err := twin.SetTargetOnline(source.ID, "tablet.3", false); err != nil {
		t.Fatal(err)
	}

	manager := NewCheckpointManager(store, twin)
	checkpoint, err := manager.Capture(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.StateContractVersion != DigitalTwinStateContractVersion1 {
		t.Fatalf("checkpoint=%+v", checkpoint)
	}
	if _, err := manager.Restore(ctx, target.ID, checkpoint.ID); err != nil {
		t.Fatal(err)
	}
	if !store.marked {
		t.Fatal("checkpoint restoration was not persisted")
	}
	restored := twin.Snapshot(target.ID)
	if restored.RuntimeSnapshotID != target.RuntimeSnapshotID || len(restored.Targets) != 1 {
		t.Fatalf("restored snapshot=%#v", restored)
	}
	state := restored.Targets[0]
	if state.ExecutionCount != 1 {
		t.Fatalf("execution count=%d; restore must not replay historical command", state.ExecutionCount)
	}
	if state.Online || !state.StateTruth.Restorable || state.StateTruth.RestorationReason != "checkpoint_restored" {
		t.Fatalf("restored target=%#v", state)
	}
}

func TestCheckpointManagerRejectsCrossSnapshotRestore(t *testing.T) {
	ctx := context.Background()
	source := activeSimulation("source-session")
	target := activeSimulation("target-session")
	target.RuntimeSnapshotID = "snapshot-2"
	store := &checkpointMemoryStore{
		sessions: map[string]domain.Session{source.ID: source, target.ID: target},
		items:    map[string]domain.SimulationCheckpoint{},
	}
	twin := NewDigitalTwin()
	manager := NewCheckpointManager(store, twin)
	checkpoint, err := manager.Capture(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Restore(ctx, target.ID, checkpoint.ID); err == nil {
		t.Fatal("cross-snapshot checkpoint restore unexpectedly succeeded")
	}
	if len(twin.Snapshot(target.ID).Targets) != 0 {
		t.Fatal("cross-snapshot restore mutated target Twin")
	}
}

func TestCheckpointManagerRollsBackTwinIfPersistenceFails(t *testing.T) {
	ctx := context.Background()
	source := activeSimulation("source-session")
	target := activeSimulation("target-session")
	store := &checkpointMemoryStore{
		sessions: map[string]domain.Session{source.ID: source, target.ID: target},
		items:    map[string]domain.SimulationCheckpoint{},
	}
	twin := NewDigitalTwin()
	if err := twin.BindSession(source); err != nil {
		t.Fatal(err)
	}
	if err := twin.SetTargetOnline(source.ID, "source-target", false); err != nil {
		t.Fatal(err)
	}
	manager := NewCheckpointManager(store, twin)
	checkpoint, err := manager.Capture(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := twin.BindSession(target); err != nil {
		t.Fatal(err)
	}
	if err := twin.SetTargetOnline(target.ID, "original-target", true); err != nil {
		t.Fatal(err)
	}
	before := twin.Snapshot(target.ID)
	store.markErr = errors.New("persistence unavailable")
	if _, err := manager.Restore(ctx, target.ID, checkpoint.ID); err == nil {
		t.Fatal("restore unexpectedly succeeded")
	}
	after := twin.Snapshot(target.ID)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("Twin rollback mismatch\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
}
