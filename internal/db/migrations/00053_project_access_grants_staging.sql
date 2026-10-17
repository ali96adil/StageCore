-- +goose Up
-- SC-024 phase 1: store desired per-project grants but DO NOT enforce them.
-- Existing global roles, project list, media access and GO behavior remain
-- unchanged until every project-bound endpoint is gated and qualified.
CREATE TABLE project_access_grants (
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    capability TEXT NOT NULL CHECK (capability IN (
        'project.view','project.rename','project.edit','media.read',
        'media.upload','media.manage','cue.edit','snapshot.publish',
        'rehearsal.control','show.go','show.enter_exit','device.configure',
        'project.export'
    )),
    granted_by TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    granted_at_us INTEGER NOT NULL,
    PRIMARY KEY (project_id,user_id,capability)
);
CREATE INDEX project_access_grants_user_idx ON project_access_grants(user_id,project_id);

-- Replacement/revocation history is persisted atomically with the grant
-- transaction. These changes are *staged policy*, not effective runtime ACL.
CREATE TABLE project_access_grant_changes (
    change_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    target_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    actor_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    previous_capabilities_json TEXT NOT NULL CHECK(json_valid(previous_capabilities_json)),
    new_capabilities_json TEXT NOT NULL CHECK(json_valid(new_capabilities_json)),
    changed_at_us INTEGER NOT NULL
);
CREATE INDEX project_access_grant_changes_project_idx
    ON project_access_grant_changes(project_id,changed_at_us);

-- +goose Down
DROP INDEX IF EXISTS project_access_grant_changes_project_idx;
DROP TABLE IF EXISTS project_access_grant_changes;
DROP INDEX IF EXISTS project_access_grants_user_idx;
DROP TABLE IF EXISTS project_access_grants;
