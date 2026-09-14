package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
	stageid "github.com/ali96adil/StageCore/internal/id"
)

func (s *Store) CreateSimulationCheckpoint(ctx context.Context, sessionID string, stateContractVersion int, twinState json.RawMessage) (domain.SimulationCheckpoint, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || stateContractVersion <= 0 {
		return domain.SimulationCheckpoint{}, fmt.Errorf("%w: simulation session and state contract version are required", domain.ErrInvalidInput)
	}
	stateJSON, err := normalizeJSON(twinState, "{}")
	if err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("checkpoint twin state: %w", err)
	}
	session, err := s.GetSessionFoundation(ctx, sessionID)
	if err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	if session.Type != domain.SessionSimulation || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return domain.SimulationCheckpoint{}, fmt.Errorf("%w: checkpoint capture requires ACTIVE SIMULATION session", domain.ErrConflict)
	}
	if strings.TrimSpace(session.RuntimeSnapshotID) == "" {
		return domain.SimulationCheckpoint{}, fmt.Errorf("%w: simulation session has no Runtime Snapshot authority", domain.ErrConflict)
	}

	checkpointID, err := stageid.New()
	if err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	now := s.clock.Now().UTC()
	hashInput := struct {
		SessionID            string          `json:"session_id"`
		ProjectID            string          `json:"project_id"`
		RuntimeSnapshotID    string          `json:"runtime_snapshot_id"`
		StateContractVersion int             `json:"state_contract_version"`
		CurrentCueID         *string         `json:"current_cue_id,omitempty"`
		LastCompletedCueID   *string         `json:"last_completed_cue_id,omitempty"`
		NextCueID            *string         `json:"next_cue_id,omitempty"`
		TwinState            json.RawMessage `json:"twin_state"`
	}{
		SessionID:            session.ID,
		ProjectID:            session.ProjectID,
		RuntimeSnapshotID:    session.RuntimeSnapshotID,
		StateContractVersion: stateContractVersion,
		CurrentCueID:         session.CurrentCueID,
		LastCompletedCueID:   session.LastCompletedCueID,
		NextCueID:            session.NextCueID,
		TwinState:            json.RawMessage(stateJSON),
	}
	canonical, err := json.Marshal(hashInput)
	if err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("marshal checkpoint hash input: %w", err)
	}
	digest := sha256.Sum256(canonical)
	contentHash := hex.EncodeToString(digest[:])

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("begin checkpoint capture: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO simulation_checkpoints (
			checkpoint_id, source_session_id, project_id, runtime_snapshot_id,
			state_contract_version, captured_at_us, current_cue_id,
			last_completed_cue_id, next_cue_id, twin_state_json, content_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		checkpointID, session.ID, session.ProjectID, session.RuntimeSnapshotID,
		stateContractVersion, clock.UnixMicros(now), nullableString(session.CurrentCueID),
		nullableString(session.LastCompletedCueID), nullableString(session.NextCueID), stateJSON, contentHash,
	)
	if err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("insert simulation checkpoint: %w", err)
	}
	if err := s.appendSimulationCheckpointEventTx(ctx, tx, session, "simulation.checkpoint.captured", map[string]any{
		"checkpoint_id":          checkpointID,
		"state_contract_version": stateContractVersion,
		"content_hash":           contentHash,
	}); err != nil {
		return domain.SimulationCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("commit simulation checkpoint: %w", err)
	}
	return domain.SimulationCheckpoint{
		ID:                   checkpointID,
		SourceSessionID:      session.ID,
		ProjectID:            session.ProjectID,
		RuntimeSnapshotID:    session.RuntimeSnapshotID,
		StateContractVersion: stateContractVersion,
		CapturedAt:           now,
		CurrentCueID:         session.CurrentCueID,
		LastCompletedCueID:   session.LastCompletedCueID,
		NextCueID:            session.NextCueID,
		TwinState:            json.RawMessage(stateJSON),
		ContentHash:          contentHash,
	}, nil
}

func (s *Store) GetSimulationCheckpoint(ctx context.Context, checkpointID string) (domain.SimulationCheckpoint, error) {
	var checkpoint domain.SimulationCheckpoint
	var capturedUS int64
	var currentCue, lastCompleted, nextCue sql.NullString
	var twinState string
	err := s.db.QueryRowContext(ctx, `
		SELECT checkpoint_id, source_session_id, project_id, runtime_snapshot_id,
		       state_contract_version, captured_at_us, current_cue_id,
		       last_completed_cue_id, next_cue_id, twin_state_json, content_hash
		FROM simulation_checkpoints WHERE checkpoint_id = ?`, strings.TrimSpace(checkpointID)).Scan(
		&checkpoint.ID, &checkpoint.SourceSessionID, &checkpoint.ProjectID, &checkpoint.RuntimeSnapshotID,
		&checkpoint.StateContractVersion, &capturedUS, &currentCue, &lastCompleted, &nextCue,
		&twinState, &checkpoint.ContentHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SimulationCheckpoint{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.SimulationCheckpoint{}, fmt.Errorf("get simulation checkpoint: %w", err)
	}
	checkpoint.CapturedAt = clock.FromUnixMicros(capturedUS)
	checkpoint.TwinState = json.RawMessage(twinState)
	assignCheckpointOptional(currentCue, &checkpoint.CurrentCueID)
	assignCheckpointOptional(lastCompleted, &checkpoint.LastCompletedCueID)
	assignCheckpointOptional(nextCue, &checkpoint.NextCueID)
	return checkpoint, nil
}

func (s *Store) MarkSimulationCheckpointRestored(ctx context.Context, sessionID, checkpointID string) error {
	session, err := s.GetSessionFoundation(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return err
	}
	checkpoint, err := s.GetSimulationCheckpoint(ctx, strings.TrimSpace(checkpointID))
	if err != nil {
		return err
	}
	if session.Type != domain.SessionSimulation || session.Status != domain.SessionActive || session.LifecycleState != domain.SessionLifecycleActive {
		return fmt.Errorf("%w: checkpoint restore requires ACTIVE SIMULATION session", domain.ErrConflict)
	}
	if session.ProjectID != checkpoint.ProjectID || session.RuntimeSnapshotID != checkpoint.RuntimeSnapshotID {
		return fmt.Errorf("%w: checkpoint authority does not match target session", domain.ErrConflict)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin checkpoint restore state: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET current_cue_id = ?, last_completed_cue_id = ?, next_cue_id = ?,
		    restoration_status = 'RESTORABLE', desired_state_ref = ?,
		    verified_state_ref = ?, manual_confirmation_required = 0
		WHERE session_id = ? AND session_type = 'SIMULATION'
		  AND status = 'ACTIVE' AND lifecycle_state = 'ACTIVE'
		  AND project_id = ? AND runtime_snapshot_id = ?`,
		nullableString(checkpoint.CurrentCueID), nullableString(checkpoint.LastCompletedCueID),
		nullableString(checkpoint.NextCueID), checkpoint.ID, checkpoint.ID,
		session.ID, checkpoint.ProjectID, checkpoint.RuntimeSnapshotID,
	)
	if err != nil {
		return fmt.Errorf("mark simulation checkpoint restored: %w", err)
	}
	if err := requireOneRow(result); err != nil {
		return err
	}
	if err := s.appendSimulationCheckpointEventTx(ctx, tx, session, "simulation.checkpoint.restored", map[string]any{
		"checkpoint_id": checkpoint.ID,
		"content_hash":  checkpoint.ContentHash,
		"replay":        false,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit checkpoint restore state: %w", err)
	}
	return nil
}

func (s *Store) appendSimulationCheckpointEventTx(ctx context.Context, tx *sql.Tx, session domain.Session, eventType string, payload any) error {
	eventID, err := stageid.New()
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal simulation checkpoint event: %w", err)
	}
	nowUS := clock.UnixMicros(s.clock.Now().UTC())
	_, err = tx.ExecContext(ctx, `
		INSERT INTO event_records (
			event_id, session_id, event_type, schema_version, occurred_at_us,
			observed_at_us, source_ref, project_id, runtime_snapshot_id,
			correlation_id, causation_id, priority, trace_context_json, payload_json
		) VALUES (?, ?, ?, 1, ?, ?, 'stagecore.simulator.checkpoint', ?, ?, NULL, NULL, 'P2', '{}', ?)`,
		eventID, session.ID, eventType, nowUS, nowUS, session.ProjectID, session.RuntimeSnapshotID, string(body),
	)
	if err != nil {
		return fmt.Errorf("append simulation checkpoint event: %w", err)
	}
	return nil
}

func assignCheckpointOptional(source sql.NullString, target **string) {
	if !source.Valid {
		return
	}
	value := source.String
	*target = &value
}
