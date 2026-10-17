-- +goose Up
-- SC-024 phase 1: PROJECT-SCOPED GRANT REGISTRY ONLY.
-- No existing API authorization changes in this migration. Enforcement at
-- every Project/Cue/Vault/StageAudio/runtime endpoint is a later guarded phase.
CREATE TABLE project_capability_grants (
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
    user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE CASCADE,
    capability TEXT NOT NULL CHECK(capability IN (
        'project.view', 'project.rename', 'project.edit',
        'media.read', 'media.upload', 'media.manage', 'cue.edit',
        'snapshot.publish', 'rehearsal.control', 'show.go',
        'show.enter_exit', 'device.configure', 'export'
    )),
    granted_by_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    granted_at_us INTEGER NOT NULL CHECK(granted_at_us > 0),
    PRIMARY KEY(project_id, user_id, capability)
);
CREATE INDEX idx_project_capability_user
    ON project_capability_grants(user_id, capability, project_id);

-- The audit ledger is append-only at the application boundary; revoking a
-- capability removes its active grant but never deletes prior audit records.
CREATE TABLE project_capability_grant_audit (
    event_id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE RESTRICT,
    target_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    actor_user_id TEXT NOT NULL REFERENCES local_users(user_id) ON DELETE RESTRICT,
    capability TEXT NOT NULL CHECK(capability IN (
        'project.view', 'project.rename', 'project.edit',
        'media.read', 'media.upload', 'media.manage', 'cue.edit',
        'snapshot.publish', 'rehearsal.control', 'show.go',
        'show.enter_exit', 'device.configure', 'export'
    )),
    action TEXT NOT NULL CHECK(action IN ('GRANT', 'REVOKE')),
    created_at_us INTEGER NOT NULL CHECK(created_at_us > 0)
);
CREATE INDEX idx_project_capability_audit_scope
    ON project_capability_grant_audit(project_id, created_at_us, event_id);

-- +goose Down
DROP INDEX IF EXISTS idx_project_capability_audit_scope;
DROP TABLE IF EXISTS project_capability_grant_audit;
DROP INDEX IF EXISTS idx_project_capability_user;
DROP TABLE IF EXISTS project_capability_grants;
