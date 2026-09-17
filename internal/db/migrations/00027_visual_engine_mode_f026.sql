-- +goose Up
-- F-026 Slice F2 stores the operator's Visual Engine choice on the editable
-- Project Revision. Absence of a row intentionally means EXTERNAL for backward
-- compatibility with projects created before this migration.
CREATE TABLE visual_engine_revision_settings (
    revision_id TEXT PRIMARY KEY
        REFERENCES project_revisions(revision_id) ON DELETE CASCADE,
    engine_mode TEXT NOT NULL
        CHECK (engine_mode IN ('NATIVE', 'EXTERNAL')),
    updated_by TEXT NOT NULL,
    updated_at_us INTEGER NOT NULL
);

-- Revision forks must preserve an explicit Visual Engine choice regardless of
-- which configuration surface creates the successor Draft. A parent without an
-- explicit row continues to inherit the compatibility default EXTERNAL.
-- +goose StatementBegin
CREATE TRIGGER visual_engine_revision_settings_inherit
AFTER INSERT ON project_revisions
WHEN NEW.parent_revision_id IS NOT NULL
BEGIN
    INSERT INTO visual_engine_revision_settings (
        revision_id, engine_mode, updated_by, updated_at_us
    )
    SELECT NEW.revision_id, parent.engine_mode, NEW.created_by, NEW.created_at_us
    FROM visual_engine_revision_settings parent
    WHERE parent.revision_id = NEW.parent_revision_id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS visual_engine_revision_settings_inherit;
DROP TABLE IF EXISTS visual_engine_revision_settings;
