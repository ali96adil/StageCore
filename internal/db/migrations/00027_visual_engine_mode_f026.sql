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

-- +goose Down
DROP TABLE IF EXISTS visual_engine_revision_settings;
