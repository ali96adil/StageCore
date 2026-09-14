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

-- +goose Down
DROP INDEX IF EXISTS simulation_checkpoints_snapshot_time_idx;
DROP INDEX IF EXISTS simulation_checkpoints_session_time_idx;
DROP TABLE IF EXISTS simulation_checkpoints;
