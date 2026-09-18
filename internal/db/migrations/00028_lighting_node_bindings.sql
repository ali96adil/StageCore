-- +goose Up
-- ESP32 DMX Lighting Node Slice 2.
-- Lighting configuration is revision-scoped product configuration. Device
-- identity/trust remains in the existing stage_devices + pairing authority.
CREATE TABLE lighting_node_revision_bindings (
    revision_id TEXT NOT NULL
        REFERENCES project_revisions(revision_id) ON DELETE CASCADE,
    device_id TEXT NOT NULL
        REFERENCES stage_devices(device_id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL,
    configuration_json TEXT NOT NULL,
    aliases_json TEXT NOT NULL DEFAULT '{}',
    updated_by TEXT NOT NULL,
    updated_at_us INTEGER NOT NULL,
    PRIMARY KEY (revision_id, device_id)
);

CREATE INDEX lighting_node_revision_bindings_device_idx
    ON lighting_node_revision_bindings(device_id, revision_id);

-- Preserve an explicit lighting binding whenever StageCore forks a revision.
-- Published Runtime Snapshots remain immutable because they already contain the
-- resolved binding captured from their validated source revision.
-- +goose StatementBegin
CREATE TRIGGER lighting_node_revision_bindings_inherit
AFTER INSERT ON project_revisions
WHEN NEW.parent_revision_id IS NOT NULL
BEGIN
    INSERT INTO lighting_node_revision_bindings (
        revision_id, device_id, profile_id, configuration_json, aliases_json,
        updated_by, updated_at_us
    )
    SELECT
        NEW.revision_id, parent.device_id, parent.profile_id,
        parent.configuration_json, parent.aliases_json,
        NEW.created_by, NEW.created_at_us
    FROM lighting_node_revision_bindings parent
    WHERE parent.revision_id = NEW.parent_revision_id;
END;
-- +goose StatementEnd

-- F-012 extension: direct SQL writes to lighting configuration are blocked
-- while an authoritative SHOW session is active for the owning Project.
-- +goose StatementBegin
CREATE TRIGGER f012_lock_lighting_node_bindings_insert BEFORE INSERT ON lighting_node_revision_bindings
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
CREATE TRIGGER f012_lock_lighting_node_bindings_update BEFORE UPDATE ON lighting_node_revision_bindings
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
CREATE TRIGGER f012_lock_lighting_node_bindings_delete BEFORE DELETE ON lighting_node_revision_bindings
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
DROP TRIGGER IF EXISTS f012_lock_lighting_node_bindings_delete;
DROP TRIGGER IF EXISTS f012_lock_lighting_node_bindings_update;
DROP TRIGGER IF EXISTS f012_lock_lighting_node_bindings_insert;
DROP TRIGGER IF EXISTS lighting_node_revision_bindings_inherit;
DROP INDEX IF EXISTS lighting_node_revision_bindings_device_idx;
DROP TABLE IF EXISTS lighting_node_revision_bindings;
