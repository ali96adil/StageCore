-- +goose Up
CREATE TABLE execution_environment_rebuild_plans (
    rebuild_plan_id TEXT PRIMARY KEY CHECK(length(rebuild_plan_id) = 36),
    environment_manifest_id TEXT NOT NULL UNIQUE,
    revision_id TEXT NOT NULL,
    source_snapshot_id TEXT NOT NULL,
    source_snapshot_sha256 TEXT NOT NULL CHECK(length(source_snapshot_sha256) = 64),
    plan_json TEXT NOT NULL CHECK(json_valid(plan_json)),
    content_sha256 TEXT NOT NULL CHECK(length(content_sha256) = 64),
    created_by TEXT NOT NULL CHECK(length(created_by) BETWEEN 1 AND 256),
    created_at_us INTEGER NOT NULL,
    updated_by TEXT NOT NULL CHECK(length(updated_by) BETWEEN 1 AND 256),
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (environment_manifest_id) REFERENCES execution_environment_manifests(environment_manifest_id) ON DELETE CASCADE,
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    FOREIGN KEY (source_snapshot_id) REFERENCES execution_environment_snapshots(environment_snapshot_id) ON DELETE CASCADE
);

CREATE INDEX idx_execution_environment_rebuild_plans_revision
    ON execution_environment_rebuild_plans(revision_id, environment_manifest_id);

-- +goose StatementBegin
CREATE TRIGGER f025_execution_environment_rebuild_plan_scope_insert
BEFORE INSERT ON execution_environment_rebuild_plans
WHEN NOT EXISTS (
    SELECT 1
    FROM execution_environment_manifests eem
    JOIN execution_environment_snapshots ees
      ON ees.environment_snapshot_id = NEW.source_snapshot_id
    WHERE eem.environment_manifest_id = NEW.environment_manifest_id
      AND eem.revision_id = NEW.revision_id
      AND ees.environment_manifest_id = NEW.environment_manifest_id
      AND ees.revision_id = NEW.revision_id
      AND ees.content_sha256 = NEW.source_snapshot_sha256
)
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_REBUILD_PLAN_SCOPE_MISMATCH');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f025_execution_environment_rebuild_plan_scope_update
BEFORE UPDATE ON execution_environment_rebuild_plans
WHEN NOT EXISTS (
    SELECT 1
    FROM execution_environment_manifests eem
    JOIN execution_environment_snapshots ees
      ON ees.environment_snapshot_id = NEW.source_snapshot_id
    WHERE eem.environment_manifest_id = NEW.environment_manifest_id
      AND eem.revision_id = NEW.revision_id
      AND ees.environment_manifest_id = NEW.environment_manifest_id
      AND ees.revision_id = NEW.revision_id
      AND ees.content_sha256 = NEW.source_snapshot_sha256
)
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_REBUILD_PLAN_SCOPE_MISMATCH');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_execution_environment_rebuild_plans_insert
BEFORE INSERT ON execution_environment_rebuild_plans
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
CREATE TRIGGER f012_lock_execution_environment_rebuild_plans_update
BEFORE UPDATE ON execution_environment_rebuild_plans
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
CREATE TRIGGER f012_lock_execution_environment_rebuild_plans_delete
BEFORE DELETE ON execution_environment_rebuild_plans
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
DROP TRIGGER IF EXISTS f012_lock_execution_environment_rebuild_plans_delete;
DROP TRIGGER IF EXISTS f012_lock_execution_environment_rebuild_plans_update;
DROP TRIGGER IF EXISTS f012_lock_execution_environment_rebuild_plans_insert;
DROP TRIGGER IF EXISTS f025_execution_environment_rebuild_plan_scope_update;
DROP TRIGGER IF EXISTS f025_execution_environment_rebuild_plan_scope_insert;
DROP INDEX IF EXISTS idx_execution_environment_rebuild_plans_revision;
DROP TABLE IF EXISTS execution_environment_rebuild_plans;
