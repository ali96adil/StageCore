-- +goose Up
-- SC-024: prospective Owner-managed Project access, intentionally NOT
-- enforced by existing HTTP/Cue/GO routes until a complete ACL cutover.
-- No existing users are auto-enrolled or granted additional privileges.
CREATE TABLE project_memberships (
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    created_by_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX project_memberships_user_idx
    ON project_memberships(user_id, project_id);

CREATE TABLE project_permission_grants (
    project_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    permission TEXT NOT NULL CHECK(length(trim(permission)) > 0),
    granted_by_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    granted_at_us INTEGER NOT NULL,
    PRIMARY KEY (project_id, user_id, permission),
    FOREIGN KEY (project_id, user_id) REFERENCES project_memberships(project_id, user_id) ON DELETE CASCADE
);

CREATE INDEX project_permission_grants_user_idx
    ON project_permission_grants(user_id, project_id, permission);

-- +goose Down
DROP INDEX IF EXISTS project_permission_grants_user_idx;
DROP TABLE IF EXISTS project_permission_grants;
DROP INDEX IF EXISTS project_memberships_user_idx;
DROP TABLE IF EXISTS project_memberships;
