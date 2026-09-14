-- +goose Up
-- F-024 Slice C: durable simulation-only checkpoints. A checkpoint records
-- logical Session progress plus a versioned Digital Twin snapshot. It never
-- represents physical device state and restoration must not replay commands.
CREATE TABLE simulation_checkpoints (
    checkpoint_id TEXT PRIMARY KEY,
    source_session_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    runtime_snapshot_id TEXT NOT NULL,
    state_contract_version INTEGER NOT NULL CHECK(state_contract_version > 0),
    captured_at_us INTEGER NOT NULL,
    current_cue_id TEXT,
    last_completed_cue_id TEXT,
    next_cue_id TEXT,
    twin_state_json TEXT NOT NULL,
    content_hash TEXT NOT NULL CHECK(length(content_hash) = 64),
    FOREIGN KEY (source_session_id) REFERENCES sessions(session_id) ON DELETE RESTRICT,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE CASCADE,
    FOREIGN KEY (runtime_snapshot_id) REFERENCES runtime_snapshots(runtime_snapshot_id) ON DELETE RESTRICT,
    FOREIGN KEY (current_cue_id) REFERENCES cues(cue_id) ON DELETE RESTRICT,
    FOREIGN KEY (last_completed_cue_id) REFERENCES cues(cue_id) ON DELETE RESTRICT,
    FOREIGN KEY (next_cue_id) REFERENCES cues(cue_id) ON DELETE RESTRICT
);

CREATE INDEX simulation_checkpoints_session_time_idx
    ON simulation_checkpoints(source_session_id, captured_at_us DESC);
CREATE INDEX simulation_checkpoints_snapshot_time_idx
    ON simulation_checkpoints(runtime_snapshot_id, captured_at_us DESC);

-- A simulation RANGE is a hard server-side execution boundary. Even a caller
-- that submits an explicit requested-next Cue cannot escape the declared range.
-- +goose StatementBegin
CREATE TRIGGER simulation_range_cue_guard
BEFORE INSERT ON cue_executions
WHEN EXISTS (
    SELECT 1
    FROM sessions s
    JOIN runtime_snapshots rs ON rs.runtime_snapshot_id = s.runtime_snapshot_id
    JOIN cues candidate ON candidate.revision_id = rs.revision_id
                       AND candidate.cue_id = NEW.cue_id
    JOIN cues range_start ON range_start.revision_id = rs.revision_id
                         AND range_start.cue_id = s.start_cue_id
    JOIN cues range_end ON range_end.revision_id = rs.revision_id
                       AND range_end.cue_id = json_extract(s.start_position_metadata_json, '$.end_cue_id')
    WHERE s.session_id = NEW.session_id
      AND s.session_type = 'SIMULATION'
      AND s.start_position_kind = 'RANGE'
      AND (
          candidate.order_index < range_start.order_index
          OR (candidate.order_index = range_start.order_index AND candidate.cue_id < range_start.cue_id)
          OR candidate.order_index > range_end.order_index
          OR (candidate.order_index = range_end.order_index AND candidate.cue_id > range_end.cue_id)
      )
)
BEGIN
    SELECT RAISE(ABORT, 'simulation cue outside range');
END;
-- +goose StatementEnd

-- The declared end Cue terminally closes the RANGE. This is intentionally a
-- Session lifecycle transition; the next GO is rejected as inactive instead of
-- silently continuing into Cues beyond the range.
-- +goose StatementBegin
CREATE TRIGGER simulation_range_complete_after_cue
AFTER UPDATE OF result ON cue_executions
WHEN OLD.result = 'RUNNING'
 AND NEW.result IN ('COMPLETED', 'FAILED', 'TIMED_OUT')
 AND EXISTS (
    SELECT 1
    FROM sessions s
    WHERE s.session_id = NEW.session_id
      AND s.session_type = 'SIMULATION'
      AND s.start_position_kind = 'RANGE'
      AND s.status = 'ACTIVE'
      AND json_extract(s.start_position_metadata_json, '$.end_cue_id') = NEW.cue_id
 )
BEGIN
    UPDATE sessions
    SET status = 'COMPLETED',
        lifecycle_state = 'COMPLETED',
        end_reason = 'RANGE_END_REACHED',
        ended_at_us = NEW.completed_at_us,
        next_cue_id = NULL
    WHERE session_id = NEW.session_id;

    INSERT INTO event_records (
        event_id, session_id, event_type, schema_version, occurred_at_us,
        observed_at_us, source_ref, project_id, runtime_snapshot_id,
        correlation_id, causation_id, priority, trace_context_json, payload_json
    )
    SELECT
        lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' ||
        lower(hex(randomblob(2))) || '-' || lower(hex(randomblob(2))) || '-' ||
        lower(hex(randomblob(6))),
        s.session_id, 'simulation.range.completed', 1,
        NEW.completed_at_us, NEW.completed_at_us, 'stagecore.simulator.range',
        s.project_id, s.runtime_snapshot_id, NEW.correlation_id, NEW.cue_execution_id,
        'P2', '{}',
        '{"end_cue_id":"' || NEW.cue_id || '","replay":false}'
    FROM sessions s WHERE s.session_id = NEW.session_id;
END;
-- +goose StatementEnd

-- Migration 00013 also advances next_cue_id after terminal Cue execution. The
-- relative order of AFTER triggers must not affect RANGE closure, so any later
-- next-cue write on an already completed RANGE is cleared again.
-- +goose StatementBegin
CREATE TRIGGER simulation_range_completed_has_no_next
AFTER UPDATE OF next_cue_id ON sessions
WHEN NEW.session_type = 'SIMULATION'
 AND NEW.start_position_kind = 'RANGE'
 AND NEW.status = 'COMPLETED'
 AND NEW.next_cue_id IS NOT NULL
BEGIN
    UPDATE sessions SET next_cue_id = NULL WHERE session_id = NEW.session_id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS simulation_range_completed_has_no_next;
DROP TRIGGER IF EXISTS simulation_range_complete_after_cue;
DROP TRIGGER IF EXISTS simulation_range_cue_guard;
DROP INDEX IF EXISTS simulation_checkpoints_snapshot_time_idx;
DROP INDEX IF EXISTS simulation_checkpoints_session_time_idx;
DROP TABLE IF EXISTS simulation_checkpoints;
