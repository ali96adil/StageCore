-- +goose Up
-- F-028 keeps analytics derived from canonical Session / Flight Recorder truth.
-- This table stores only the operator's explicit training-set decision.
-- AUTO is represented by the absence of a row.
CREATE TABLE timing_session_selection (
    session_id TEXT PRIMARY KEY,
    mode TEXT NOT NULL CHECK(mode IN ('INCLUDE', 'EXCLUDE')),
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE CASCADE
);

CREATE INDEX timing_session_selection_mode_idx
    ON timing_session_selection(mode, updated_at_us);

-- +goose Down
DROP INDEX IF EXISTS timing_session_selection_mode_idx;
DROP TABLE IF EXISTS timing_session_selection;
