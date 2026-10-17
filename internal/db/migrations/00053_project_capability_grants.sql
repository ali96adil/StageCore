-- +goose Up
-- SC-024: inert project-scoped authorization *data foundation*.
-- Existing HTTP routes remain on their established role checks until EVERY
-- project access path is audited and explicitly migrated to the new policy.
-- No default wildcard grants, no implicit access to copied Projects.
CREATE TABLE project_capability_grants (
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    capability TEXT NOT NULL CHECK (
        capability IN (
            'project.view', 'project.rename', 'project.edit',
            'cue.edit', 'snapshot.publish', 'rehearsal.control',
            'show.go', 'show.enter_exit', 'device.configure',
            'media.view', 'media.upload', 'media.manage',
            'audio.control', 'project.export'
        )
    ),
    granted_by_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    granted_at_us INTEGER NOT NULL,
    PRIMARY KEY (project_id, user_id, capability)
);
CREATE INDEX project_capability_grants_user_project_idx
    ON project_capability_grants(user_id, project_id);

-- +goose Down
DROP INDEX IF EXISTS project_capability_grants_user_project_idx;
DROP TABLE IF EXISTS project_capability_grants;
