-- +goose Up
ALTER TABLE execution_environment_manifests
ADD COLUMN machine_role_id TEXT NULL REFERENCES machine_roles(machine_role_id) ON DELETE RESTRICT;

CREATE INDEX idx_execution_environment_manifests_machine_role
    ON execution_environment_manifests(machine_role_id)
    WHERE machine_role_id IS NOT NULL;

-- Binding metadata must never point across Project boundaries. Keep this
-- invariant in SQLite as well as Store code so direct SQL cannot create a
-- truthful-looking but invalid deployment mapping.
-- +goose StatementBegin
CREATE TRIGGER f025_execution_environment_machine_role_insert_guard
BEFORE INSERT ON execution_environment_manifests
WHEN NEW.machine_role_id IS NOT NULL
 AND NOT EXISTS (
    SELECT 1
    FROM project_revisions pr
    JOIN machine_roles mr ON mr.machine_role_id = NEW.machine_role_id
    WHERE pr.revision_id = NEW.revision_id
      AND mr.project_id = pr.project_id
 )
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_MACHINE_ROLE_PROJECT_MISMATCH');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f025_execution_environment_machine_role_update_guard
BEFORE UPDATE OF revision_id, machine_role_id ON execution_environment_manifests
WHEN NEW.machine_role_id IS NOT NULL
 AND NOT EXISTS (
    SELECT 1
    FROM project_revisions pr
    JOIN machine_roles mr ON mr.machine_role_id = NEW.machine_role_id
    WHERE pr.revision_id = NEW.revision_id
      AND mr.project_id = pr.project_id
 )
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_MACHINE_ROLE_PROJECT_MISMATCH');
END;
-- +goose StatementEnd

-- A bound role cannot later be moved to another Project behind the binding.
-- +goose StatementBegin
CREATE TRIGGER f025_bound_machine_role_project_update_guard
BEFORE UPDATE OF project_id ON machine_roles
WHEN EXISTS (
    SELECT 1
    FROM execution_environment_manifests eem
    JOIN project_revisions pr ON pr.revision_id = eem.revision_id
    WHERE eem.machine_role_id = OLD.machine_role_id
      AND pr.project_id <> NEW.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_MACHINE_ROLE_PROJECT_MISMATCH');
END;
-- +goose StatementEnd

-- Project revisions are not expected to move between Projects, but guard the
-- other side of the relation as a durable fail-closed invariant as well.
-- +goose StatementBegin
CREATE TRIGGER f025_bound_revision_project_update_guard
BEFORE UPDATE OF project_id ON project_revisions
WHEN EXISTS (
    SELECT 1
    FROM execution_environment_manifests eem
    JOIN machine_roles mr ON mr.machine_role_id = eem.machine_role_id
    WHERE eem.revision_id = OLD.revision_id
      AND mr.project_id <> NEW.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'EXECUTION_ENVIRONMENT_MACHINE_ROLE_PROJECT_MISMATCH');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS f025_bound_revision_project_update_guard;
DROP TRIGGER IF EXISTS f025_bound_machine_role_project_update_guard;
DROP TRIGGER IF EXISTS f025_execution_environment_machine_role_update_guard;
DROP TRIGGER IF EXISTS f025_execution_environment_machine_role_insert_guard;
DROP INDEX IF EXISTS idx_execution_environment_manifests_machine_role;
ALTER TABLE execution_environment_manifests DROP COLUMN machine_role_id;
