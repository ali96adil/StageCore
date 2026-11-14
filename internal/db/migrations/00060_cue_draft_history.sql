-- +goose Up
CREATE TABLE cue_draft_history_state (
    revision_id TEXT NOT NULL,
    actor_id TEXT NOT NULL CHECK(length(trim(actor_id)) BETWEEN 1 AND 256),
    next_sequence INTEGER NOT NULL DEFAULT 1 CHECK(next_sequence > 0),
    cursor_sequence INTEGER NOT NULL DEFAULT 0 CHECK(cursor_sequence >= 0),
    PRIMARY KEY (revision_id, actor_id),
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE CASCADE
);

CREATE TABLE cue_draft_history_events (
    revision_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK(sequence > 0),
    operation TEXT NOT NULL CHECK(operation IN ('CREATE', 'EDIT', 'DELETE', 'REORDER')),
    cue_id TEXT NOT NULL DEFAULT '',
    created_at_us INTEGER NOT NULL,
    delta_json TEXT NOT NULL CHECK(
        json_valid(delta_json) AND length(CAST(delta_json AS BLOB)) <= 4194304
    ),
    PRIMARY KEY (revision_id, actor_id, sequence),
    FOREIGN KEY (revision_id, actor_id)
        REFERENCES cue_draft_history_state(revision_id, actor_id) ON DELETE CASCADE
);

CREATE INDEX cue_draft_history_events_recent_idx
    ON cue_draft_history_events(revision_id, actor_id, sequence DESC);

-- +goose Down
DROP INDEX cue_draft_history_events_recent_idx;
DROP TABLE cue_draft_history_events;
DROP TABLE cue_draft_history_state;
