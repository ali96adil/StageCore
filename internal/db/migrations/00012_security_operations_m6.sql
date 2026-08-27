-- +goose Up
CREATE TABLE security_audit_records (
    audit_id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    occurred_at_us INTEGER NOT NULL,
    actor_user_id TEXT,
    actor_username TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL CHECK (result IN ('SUCCESS','REJECTED','FAILED')),
    reason TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    CHECK (length(event_type) BETWEEN 1 AND 128),
    CHECK (json_valid(metadata_json))
);
CREATE INDEX security_audit_time_idx ON security_audit_records (occurred_at_us DESC, audit_id DESC);
CREATE INDEX security_audit_type_idx ON security_audit_records (event_type, occurred_at_us DESC);

CREATE TABLE plugin_permission_grants (
    plugin_id TEXT NOT NULL,
    permission TEXT NOT NULL,
    granted INTEGER NOT NULL CHECK (granted IN (0,1)),
    updated_at_us INTEGER NOT NULL,
    updated_by TEXT NOT NULL,
    PRIMARY KEY (plugin_id, permission),
    CHECK (length(plugin_id) BETWEEN 1 AND 128),
    CHECK (length(permission) BETWEEN 1 AND 128)
);

-- Preserve the already-proven first-party OSC runtime authority as an explicit,
-- persistent baseline grant. M6 security administration can revoke it and the
-- Plugin Host will enforce the persisted state for subsequent executions.
INSERT INTO plugin_permission_grants
    (plugin_id, permission, granted, updated_at_us, updated_by)
VALUES
    ('stagecore.osc', 'network.udp.send', 1, 0, 'm6-baseline'),
    ('stagecore.osc', 'network.udp.listen', 1, 0, 'm6-baseline');

-- +goose Down
DROP TABLE IF EXISTS plugin_permission_grants;
DROP INDEX IF EXISTS security_audit_type_idx;
DROP INDEX IF EXISTS security_audit_time_idx;
DROP TABLE IF EXISTS security_audit_records;
