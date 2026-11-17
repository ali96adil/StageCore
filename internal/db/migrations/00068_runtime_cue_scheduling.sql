-- +goose Up
-- Pause only affects admission of future GO/JUMP requests. Accepted runs and
-- already dispatched external actions remain owned by their existing path.
CREATE TABLE runtime_cue_scheduling_state (
    session_id TEXT PRIMARY KEY,
    paused INTEGER NOT NULL DEFAULT 0 CHECK(paused IN (0, 1)),
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE RESTRICT
);

-- +goose Down
DROP TABLE IF EXISTS runtime_cue_scheduling_state;
