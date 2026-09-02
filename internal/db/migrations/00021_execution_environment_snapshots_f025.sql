-- +goose Up
CREATE TABLE execution_environment_snapshots (
    environment_snapshot_id TEXT PRIMARY KEY CHECK(length(environment_snapshot_id) = 36),
    environment_manifest_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    source_manifest_sha256 TEXT NOT NULL CHECK(length(source_manifest_sha256) = 64),
    snapshot_json TEXT NOT NULL CHECK(json_valid(snapshot_json)),
    content_sha256 TEXT NOT NULL CHECK(length(content_sha256) = 64),
    created_by TEXT NOT NULL CHECK(length(created_by) BETWEEN 1 AND 256),
    created_at_us INTEGER NOT NULL,
    FOREIGN KEY (environment_manifest_id) REFERENCES execution_environment_manifests(environment_manifest_id) ON DELETE CASCADE,
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    UNIQUE (environment_manifest_id, content_sha256)
);

CREATE INDEX idx_execution_environment_snapshots_manifest
    ON execution_environment_snapshots(environment_manifest_id, created_at_us DESC, environment_snapshot_id DESC);
CREATE INDEX idx_execution_environment_snapshots_revision
    ON execution_environment_snapshots(revision_id, environment_manifest_id);

-- +goose StatementBegin
CREATE TRIGGER f025_execution_environment_snapshot_revision_insert
BEFORE INSERT ON execution_environment_snapshots
WHEN NOT EXISTS (
    SELECT 1 FROM execution_environment_manifests eem
    WHERE eem.environment_manifest_id = NEW.environment_manifest_id
      AND eem.revision_id = NEW.revision_id
)
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_SNAPSHOT_REVISION_MISMATCH');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f025_execution_environment_snapshot_revision_update
BEFORE UPDATE ON execution_environment_snapshots
WHEN NOT EXISTS (
    SELECT 1 FROM execution_environment_manifests eem
    WHERE eem.environment_manifest_id = NEW.environment_manifest_id
      AND eem.revision_id = NEW.revision_id
)
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_SNAPSHOT_REVISION_MISMATCH');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_execution_environment_snapshots_insert
BEFORE INSERT ON execution_environment_snapshots
WHEN EXISTS (
    SELECT 1 FROM project_revisions pr
    JOIN f012_locked_projects lp ON lp.project_id = pr.project_id
    WHERE pr.revision_id = NEW.revision_id
)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_execution_environment_snapshots_update
BEFORE UPDATE ON execution_environment_snapshots
WHEN EXISTS (
    SELECT 1 FROM project_revisions pr
    JOIN f012_locked_projects lp ON lp.project_id = pr.project_id
    WHERE pr.revision_id IN (NEW.revision_id, OLD.revision_id)
)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_execution_environment_snapshots_delete
BEFORE DELETE ON execution_environment_snapshots
WHEN EXISTS (
    SELECT 1 FROM project_revisions pr
    JOIN f012_locked_projects lp ON lp.project_id = pr.project_id
    WHERE pr.revision_id = OLD.revision_id
)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS f012_lock_execution_environment_snapshots_delete;
DROP TRIGGER IF EXISTS f012_lock_execution_environment_snapshots_update;
DROP TRIGGER IF EXISTS f012_lock_execution_environment_snapshots_insert;
DROP TRIGGER IF EXISTS f025_execution_environment_snapshot_revision_update;
DROP TRIGGER IF EXISTS f025_execution_environment_snapshot_revision_insert;
DROP INDEX IF EXISTS idx_execution_environment_snapshots_revision;
DROP INDEX IF EXISTS idx_execution_environment_snapshots_manifest;
DROP TABLE IF EXISTS execution_environment_snapshots;
