package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/capability"
	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/contracts"
	"github.com/ali96adil/StageCore/internal/domain"
)

// StopSimulationWithEvent commits the Simulation STOP transition and its
// durable event in the same database transaction. This avoids a stopped
// Session with no matching event when the Project grant is revoked midway.
// It does not touch REHEARSAL, SHOW, external transports, or physical outputs.
func (s *Store) StopSimulationWithEvent(ctx context.Context, session domain.Session, event contracts.EventEnvelope) error {
	if session.ID == "" || session.ProjectID == "" || session.RuntimeSnapshotID == "" ||
		session.Type != domain.SessionSimulation || session.Status != domain.SessionActive ||
		session.LifecycleState != domain.SessionLifecycleActive {
		return fmt.Errorf("%w: active simulation session required", domain.ErrConflict)
	}
	if event.EventType != "simulation.stopped" || event.ProjectID != session.ProjectID ||
		event.RuntimeSnapshotID != session.RuntimeSnapshotID {
		return fmt.Errorf("%w: simulation STOP event scope mismatch", domain.ErrInvalidInput)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin simulation STOP: %w", err) }
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET status = ?, lifecycle_state = ?, end_reason = ?, ended_at_us = ?
		WHERE session_id = ? AND project_id = ? AND runtime_snapshot_id = ?
		  AND session_type = 'SIMULATION' AND status = 'ACTIVE' AND lifecycle_state = 'ACTIVE'`,
		domain.SessionCompleted, domain.SessionLifecycleStopped,
		strings.TrimSpace("operator stopped simulation"), clock.UnixMicros(s.clock.Now()),
		session.ID, session.ProjectID, session.RuntimeSnapshotID)
	if err != nil { return fmt.Errorf("stop simulation session: %w", err) }
	if err := requireOneRow(result); err != nil { return err }
	if _, err := s.appendEvent(ctx, tx, &session.ID, event); err != nil {
		return fmt.Errorf("append simulation STOP event: %w", err)
	}
	if err := capability.CheckWriteAuthorization(ctx, tx); err != nil { return err }
	if err := tx.Commit(); err != nil { return fmt.Errorf("commit simulation STOP: %w", err) }
	return nil
}
