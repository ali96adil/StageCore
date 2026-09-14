package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ali96adil/StageCore/internal/clock"
	"github.com/ali96adil/StageCore/internal/domain"
)

// ListSimulationCheckpoints returns durable simulation-only checkpoints for one
// published Runtime Snapshot. The snapshot filter is mandatory so checkpoints
// can never be presented as reusable across immutable executable definitions.
func (s *Store) ListSimulationCheckpoints(ctx context.Context, projectID, runtimeSnapshotID string, limit int) ([]domain.SimulationCheckpoint, error) {
	projectID = strings.TrimSpace(projectID)
	runtimeSnapshotID = strings.TrimSpace(runtimeSnapshotID)
	if projectID == "" || runtimeSnapshotID == "" {
		return nil, fmt.Errorf("%w: project and Runtime Snapshot are required", domain.ErrInvalidInput)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT checkpoint_id, source_session_id, project_id, runtime_snapshot_id,
		       state_contract_version, captured_at_us, current_cue_id,
		       last_completed_cue_id, next_cue_id, twin_state_json, content_hash
		FROM simulation_checkpoints
		WHERE project_id = ? AND runtime_snapshot_id = ?
		ORDER BY captured_at_us DESC, checkpoint_id DESC
		LIMIT ?`, projectID, runtimeSnapshotID, limit)
	if err != nil {
		return nil, fmt.Errorf("list simulation checkpoints: %w", err)
	}
	defer rows.Close()

	items := make([]domain.SimulationCheckpoint, 0)
	for rows.Next() {
		var checkpoint domain.SimulationCheckpoint
		var capturedUS int64
		var currentCue, lastCompleted, nextCue sql.NullString
		var twinState string
		if err := rows.Scan(
			&checkpoint.ID, &checkpoint.SourceSessionID, &checkpoint.ProjectID, &checkpoint.RuntimeSnapshotID,
			&checkpoint.StateContractVersion, &capturedUS, &currentCue, &lastCompleted, &nextCue,
			&twinState, &checkpoint.ContentHash,
		); err != nil {
			return nil, fmt.Errorf("scan simulation checkpoint: %w", err)
		}
		checkpoint.CapturedAt = clock.FromUnixMicros(capturedUS)
		checkpoint.TwinState = json.RawMessage(twinState)
		assignCheckpointOptional(currentCue, &checkpoint.CurrentCueID)
		assignCheckpointOptional(lastCompleted, &checkpoint.LastCompletedCueID)
		assignCheckpointOptional(nextCue, &checkpoint.NextCueID)
		items = append(items, checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate simulation checkpoints: %w", err)
	}
	return items, nil
}
