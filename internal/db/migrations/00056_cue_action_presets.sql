-- +goose Up
CREATE TABLE cue_action_presets (
    preset_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    name TEXT NOT NULL COLLATE NOCASE,
    action TEXT NOT NULL CHECK (json_valid(action) AND length(CAST(action AS BLOB)) <= 16384),
    UNIQUE (project_id, user_id, name)
);

-- +goose Down
DROP TABLE cue_action_presets;
