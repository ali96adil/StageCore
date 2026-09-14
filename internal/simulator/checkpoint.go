package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/domain"
)

// SimulationCheckpointPersistence is the durable boundary used by the
// simulation checkpoint manager. It deliberately contains no physical device
// or transport state.
type SimulationCheckpointPersistence interface {
	GetSessionFoundation(context.Context, string) (domain.Session, error)
	CreateSimulationCheckpoint(context.Context, string, int, json.RawMessage) (domain.SimulationCheckpoint, error)
	GetSimulationCheckpoint(context.Context, string) (domain.SimulationCheckpoint, error)
	MarkSimulationCheckpointRestored(context.Context, string, string) error
}

type CheckpointManager struct {
	store SimulationCheckpointPersistence
	twin  *DigitalTwin
}

func NewCheckpointManager(store SimulationCheckpointPersistence, twin *DigitalTwin) *CheckpointManager {
	return &CheckpointManager{store: store, twin: twin}
}

// Capture records logical Session progress in Store plus a copied Digital Twin
// snapshot. No command is executed while capturing a checkpoint.
func (m *CheckpointManager) Capture(ctx context.Context, sessionID string) (domain.SimulationCheckpoint, error) {
	if m == nil || m.store == nil || m.twin == nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("simulation checkpoint manager is unavailable")
	}
	session, err := m.store.GetSessionFoundation(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	if session.Type != domain.SessionSimulation || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return domain.SimulationCheckpoint{}, fmt.Errorf("checkpoint capture requires ACTIVE SIMULATION session")
	}
	if err := m.twin.BindSession(session); err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	snapshot := m.twin.Snapshot(session.ID)
	if snapshot.RuntimeSnapshotID != session.RuntimeSnapshotID {
		return domain.SimulationCheckpoint{}, fmt.Errorf("digital twin Runtime Snapshot binding does not match session")
	}
	stateJSON, err := json.Marshal(snapshot)
	if err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("marshal digital twin checkpoint: %w", err)
	}
	return m.store.CreateSimulationCheckpoint(ctx, session.ID, DigitalTwinStateContractVersion1, stateJSON)
}

// Restore installs checkpointed virtual state directly. Historical cue/action
// commands are never replayed. If durable Session truth cannot be committed,
// the previous in-memory Twin state is restored before returning the error.
func (m *CheckpointManager) Restore(ctx context.Context, sessionID, checkpointID string) (domain.SimulationCheckpoint, error) {
	if m == nil || m.store == nil || m.twin == nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("simulation checkpoint manager is unavailable")
	}
	session, err := m.store.GetSessionFoundation(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	checkpoint, err := m.store.GetSimulationCheckpoint(ctx, strings.TrimSpace(checkpointID))
	if err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	if session.Type != domain.SessionSimulation || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return domain.SimulationCheckpoint{}, fmt.Errorf("checkpoint restore requires ACTIVE SIMULATION session")
	}
	if checkpoint.ProjectID != session.ProjectID || checkpoint.RuntimeSnapshotID != session.RuntimeSnapshotID {
		return domain.SimulationCheckpoint{}, fmt.Errorf("checkpoint authority does not match target simulation session")
	}
	if checkpoint.StateContractVersion != DigitalTwinStateContractVersion1 {
		return domain.SimulationCheckpoint{}, fmt.Errorf("unsupported Digital Twin checkpoint state version %d", checkpoint.StateContractVersion)
	}
	var restored SessionSnapshot
	if err := json.Unmarshal(checkpoint.TwinState, &restored); err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("decode Digital Twin checkpoint: %w", err)
	}
	if restored.Version != DigitalTwinStateContractVersion1 || restored.RuntimeSnapshotID != session.RuntimeSnapshotID {
		return domain.SimulationCheckpoint{}, fmt.Errorf("checkpoint Digital Twin state does not match target Runtime Snapshot")
	}

	previous := m.twin.Snapshot(session.ID)
	if err := m.twin.RestoreSnapshot(session, restored); err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	if err := m.store.MarkSimulationCheckpointRestored(ctx, session.ID, checkpoint.ID); err != nil {
		_ = m.twin.RestoreSnapshot(session, previous)
		return domain.SimulationCheckpoint{}, err
	}
	return checkpoint, nil
}

// RestoreSnapshot copies a previously validated simulation state into an
// authoritative target Session. Session identity is intentionally rebased;
// Runtime Snapshot authority may not change.
func (d *DigitalTwin) RestoreSnapshot(session domain.Session, snapshot SessionSnapshot) error {
	if d == nil {
		return fmt.Errorf("digital twin is unavailable")
	}
	if session.Type != domain.SessionSimulation || strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.RuntimeSnapshotID) == "" {
		return fmt.Errorf("restore requires authoritative SIMULATION session")
	}
	if snapshot.Version != DigitalTwinStateContractVersion1 {
		return fmt.Errorf("unsupported Digital Twin state version %d", snapshot.Version)
	}
	if strings.TrimSpace(snapshot.RuntimeSnapshotID) != session.RuntimeSnapshotID {
		return fmt.Errorf("Digital Twin checkpoint Runtime Snapshot mismatch")
	}

	state := &twinSession{
		runtimeSnapshotID: session.RuntimeSnapshotID,
		targets:           make(map[string]*TargetState, len(snapshot.Targets)),
		faults:            append([]FaultScenario(nil), snapshot.Faults...),
	}
	for _, target := range snapshot.Targets {
		copy := cloneTargetState(target)
		copy.StateTruth.Version = DigitalTwinStateContractVersion1
		copy.StateTruth.Scope = "SIMULATION_ONLY"
		copy.StateTruth.Restorable = true
		copy.StateTruth.RestorationReason = "checkpoint_restored"
		state.targets[copy.TargetRef] = &copy
	}

	d.mu.Lock()
	d.sessions[session.ID] = state
	d.mu.Unlock()
	return nil
}
