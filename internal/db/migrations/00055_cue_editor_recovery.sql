-- +goose Up
CREATE TABLE cue_editor_recovery (
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    generation TEXT NOT NULL,
    document TEXT NOT NULL CHECK (json_valid(document) AND length(CAST(document AS BLOB)) <= 16384),
    PRIMARY KEY (project_id, user_id)
);

-- +goose Down
DROP TABLE cue_editor_recovery;
