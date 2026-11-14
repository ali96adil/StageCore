-- +goose Up
ALTER TABLE cue_action_presets
    ADD COLUMN description TEXT NOT NULL DEFAULT ''
    CHECK (length(description) <= 280);

-- +goose Down
ALTER TABLE cue_action_presets DROP COLUMN description;
