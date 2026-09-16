-- +goose Up
-- F-026 Slice F2 records only the revision-scoped Operator workflow choice.
-- Canonical Cue/Output/Target capability state remains the runtime authority.
CREATE TABLE visual_engine_configurations (
    revision_id TEXT PRIMARY KEY,
    engine_mode TEXT NOT NULL CHECK(engine_mode IN ('NATIVE', 'EXTERNAL')),
    updated_by TEXT NOT NULL DEFAULT '' CHECK(length(updated_by) <= 256),
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT
);

-- The mode is Project configuration and must obey the same active-SHOW
-- immutability boundary as other revision-scoped configuration.
-- +goose StatementBegin
CREATE TRIGGER f012_lock_visual_engine_configurations_insert
BEFORE INSERT ON visual_engine_configurations
WHEN EXISTS (
    SELECT 1
    FROM project_revisions pr
    JOIN f012_locked_projects lp ON lp.project_id = pr.project_id
    WHERE pr.revision_id = NEW.revision_id
)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_visual_engine_configurations_update
BEFORE UPDATE ON visual_engine_configurations
WHEN EXISTS (
    SELECT 1
    FROM project_revisions pr
    JOIN f012_locked_projects lp ON lp.project_id = pr.project_id
    WHERE pr.revision_id IN (NEW.revision_id, OLD.revision_id)
)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_visual_engine_configurations_delete
BEFORE DELETE ON visual_engine_configurations
WHEN EXISTS (
    SELECT 1
    FROM project_revisions pr
    JOIN f012_locked_projects lp ON lp.project_id = pr.project_id
    WHERE pr.revision_id = OLD.revision_id
)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS f012_lock_visual_engine_configurations_delete;
DROP TRIGGER IF EXISTS f012_lock_visual_engine_configurations_update;
DROP TRIGGER IF EXISTS f012_lock_visual_engine_configurations_insert;
DROP TABLE IF EXISTS visual_engine_configurations;
