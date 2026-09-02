-- +goose Up
CREATE TABLE execution_environment_manifests (
    environment_manifest_id TEXT PRIMARY KEY CHECK(length(environment_manifest_id) = 36),
    revision_id TEXT NOT NULL,
    environment_key TEXT NOT NULL CHECK(length(environment_key) BETWEEN 1 AND 96),
    adapter_key TEXT NOT NULL CHECK(length(adapter_key) BETWEEN 1 AND 96),
    application_key TEXT NOT NULL CHECK(length(application_key) BETWEEN 1 AND 96),
    manifest_json TEXT NOT NULL CHECK(json_valid(manifest_json)),
    content_sha256 TEXT NOT NULL CHECK(length(content_sha256) = 64),
    created_by TEXT NOT NULL DEFAULT '',
    created_at_us INTEGER NOT NULL,
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    UNIQUE (revision_id, environment_key)
);

CREATE INDEX idx_execution_environment_manifests_revision
    ON execution_environment_manifests(revision_id, environment_key);

-- F-025 execution environments are revision configuration and therefore inherit
-- the same active-SHOW immutability boundary as cues, routes, media requirements,
-- and the rest of the Project configuration graph.
-- +goose StatementBegin
CREATE TRIGGER f012_lock_execution_environment_manifests_insert
BEFORE INSERT ON execution_environment_manifests
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
CREATE TRIGGER f012_lock_execution_environment_manifests_update
BEFORE UPDATE ON execution_environment_manifests
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
CREATE TRIGGER f012_lock_execution_environment_manifests_delete
BEFORE DELETE ON execution_environment_manifests
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
DROP TRIGGER IF EXISTS f012_lock_execution_environment_manifests_delete;
DROP TRIGGER IF EXISTS f012_lock_execution_environment_manifests_update;
DROP TRIGGER IF EXISTS f012_lock_execution_environment_manifests_insert;
DROP INDEX IF EXISTS idx_execution_environment_manifests_revision;
DROP TABLE IF EXISTS execution_environment_manifests;
