-- +goose Up
-- Runtime emergency state is session-scoped operational state, not Project
-- configuration. It remains writable during SHOW so an emergency action can
-- fail closed and survive Hub/browser restart.
CREATE TABLE runtime_safety_state (
    session_id TEXT PRIMARY KEY,
    managed_output_blackout INTEGER NOT NULL DEFAULT 0 CHECK(managed_output_blackout IN (0, 1)),
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE RESTRICT
);

-- +goose Down
DROP TABLE IF EXISTS runtime_safety_state;
