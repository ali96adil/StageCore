package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

// ConfirmSimulationStartState records explicit operator acceptance that a
// selected simulation start will use the current virtual state without claiming
// that prior cue state was reconstructed or verified.
func (s *Store) ConfirmSimulationStartState(ctx context.Context, sessionID string) error {
	session, err := s.GetSessionFoundation(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Type != domain.SessionSimulation || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return fmt.Errorf("%w: start-state confirmation requires ACTIVE SIMULATION session", domain.ErrConflict)
	}
	if !session.StateTruth.ManualConfirmationRequired {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin simulation state confirmation: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET restoration_status = 'UNAVAILABLE', manual_confirmation_required = 0,
		    desired_state_ref = NULL, verified_state_ref = NULL
		WHERE session_id = ? AND session_type = 'SIMULATION'
		  AND status = 'ACTIVE' AND lifecycle_state = 'ACTIVE'
		  AND manual_confirmation_required = 1`, session.ID)
	if err != nil {
		return fmt.Errorf("confirm simulation start state: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if err := appendSimulationSessionEventTx(ctx, tx, s.clock, session, "simulation.start_state.confirmed", map[string]any{
		"restoration_status": "UNAVAILABLE",
		"verified":           false,
		"replay":             false,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit simulation start-state confirmation: %w", err)
	}
	return nil
}

// CompleteSimulationRange closes an ACTIVE simulation RANGE exactly when its
// declared end Cue has terminally executed. It clears logical next position so
// a later GO cannot escape the bounded range.
func (s *Store) CompleteSimulationRange(ctx context.Context, sessionID, completedCueID string) error {
	session, err := s.GetSessionFoundation(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Type != domain.SessionSimulation || session.StartPosition.Kind != domain.SessionStartRange || session.Status != domain.SessionActive {
		return fmt.Errorf("%w: active SIMULATION RANGE session required", domain.ErrConflict)
	}
	var metadata domain.SimulationRangeMetadata
	if err := json.Unmarshal(session.StartPosition.Metadata, &metadata); err != nil || metadata.EndCueID == "" {
		return fmt.Errorf("%w: invalid persisted RANGE metadata", domain.ErrConflict)
	}
	if metadata.EndCueID != completedCueID {
		return fmt.Errorf("%w: completed cue is not RANGE end", domain.ErrConflict)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin simulation range completion: %w", err)
	}
	defer tx.Rollback()
	now := s.clock.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET status = 'COMPLETED', lifecycle_state = 'COMPLETED',
		    end_reason = 'RANGE_END_REACHED', ended_at_us = ?, next_cue_id = NULL
		WHERE session_id = ? AND session_type = 'SIMULATION'
		  AND status = 'ACTIVE' AND lifecycle_state = 'ACTIVE'`, clock.UnixMicros(now), session.ID)
	if err != nil {
		return fmt.Errorf("complete simulation range: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if err := appendSimulationSessionEventTx(ctx, tx, s.clock, session, "simulation.range.completed", map[string]any{
		"end_cue_id": completedCueID,
		"replay":     false,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit simulation range completion: %w", err)
	}
	return nil
}

func appendSimulationSessionEventTx(ctx context.Context, tx *sql.Tx, c clock.Clock, session domain.Session, eventType string, payload any) error {
	eventID, err := stageid.New()
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal simulation session event: %w", err)
	}
	nowUS := clock.UnixMicros(c.Now().UTC())
	_, err = tx.ExecContext(ctx, `
		INSERT INTO event_records (
			event_id, session_id, event_type, schema_version, occurred_at_us,
			observed_at_us, source_ref, project_id, runtime_snapshot_id,
			correlation_id, causation_id, priority, trace_context_json, payload_json
		) VALUES (?, ?, ?, 1, ?, ?, 'stagecore.simulator.session', ?, ?, NULL, NULL, 'P2', '{}', ?)`,
		eventID, session.ID, eventType, nowUS, nowUS, session.ProjectID, session.RuntimeSnapshotID, string(body),
	)
	if err != nil {
		return fmt.Errorf("append simulation session event: %w", err)
	}
	return nil
}
