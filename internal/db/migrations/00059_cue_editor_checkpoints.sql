-- +goose Up
CREATE TABLE cue_editor_checkpoints (
    checkpoint_order INTEGER PRIMARY KEY AUTOINCREMENT,
    checkpoint_id TEXT NOT NULL UNIQUE,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    cue_id TEXT NOT NULL REFERENCES cues(cue_id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 64),
    created_at TEXT NOT NULL,
    document TEXT NOT NULL CHECK(json_valid(document) AND length(CAST(document AS BLOB)) <= 16384)
);
CREATE INDEX cue_editor_checkpoints_scope_idx
    ON cue_editor_checkpoints(project_id, user_id, cue_id, checkpoint_order DESC);

-- +goose Down
DROP INDEX cue_editor_checkpoints_scope_idx;
DROP TABLE cue_editor_checkpoints;
