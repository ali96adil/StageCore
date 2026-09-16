-- +goose Up
-- F-026 Slice D reuses the existing F-007 LiveSource descriptor while adding
-- an explicit Companion/Machine Role execution placement. Stage Device binding
-- remains supported for Phase 4 clients, but a source may not use both paths.
ALTER TABLE live_video_sources
    ADD COLUMN execution_machine_role_id TEXT
        REFERENCES machine_roles(machine_role_id) ON DELETE SET NULL;

CREATE INDEX live_video_sources_machine_role_idx
    ON live_video_sources(execution_machine_role_id)
    WHERE execution_machine_role_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS live_video_sources_machine_role_idx;
ALTER TABLE live_video_sources DROP COLUMN execution_machine_role_id;
