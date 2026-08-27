-- +goose Up
CREATE TABLE companions (
    companion_id TEXT PRIMARY KEY CHECK(length(companion_id) = 36),
    display_name TEXT NOT NULL,
    hostname TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    architecture TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL DEFAULT '',
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    last_seen_at_us INTEGER NOT NULL,
    trust_state TEXT NOT NULL CHECK(trust_state IN ('UNTRUSTED', 'TRUSTED', 'REVOKED')),
    readiness TEXT NOT NULL CHECK(readiness IN ('UNKNOWN', 'SYNCING', 'READY', 'DEGRADED', 'OFFLINE', 'MISMATCH', 'BLOCKED')),
    applied_runtime_snapshot_id TEXT NULL,
    config_hash TEXT NOT NULL DEFAULT '',
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (applied_runtime_snapshot_id) REFERENCES runtime_snapshots(runtime_snapshot_id) ON DELETE RESTRICT
);

CREATE INDEX companions_last_seen_idx ON companions(last_seen_at_us);

CREATE TABLE machine_roles (
    machine_role_id TEXT PRIMARY KEY CHECK(length(machine_role_id) = 36),
    project_id TEXT NOT NULL,
    role_key TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    required_capabilities_json TEXT NOT NULL DEFAULT '[]',
    required_runtime_snapshot_id TEXT NULL,
    required_config_hash TEXT NOT NULL DEFAULT '',
    required INTEGER NOT NULL DEFAULT 1 CHECK(required IN (0, 1)),
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    FOREIGN KEY (required_runtime_snapshot_id) REFERENCES runtime_snapshots(runtime_snapshot_id) ON DELETE RESTRICT,
    UNIQUE (project_id, role_key)
);

CREATE TABLE role_assignments (
    role_assignment_id TEXT PRIMARY KEY CHECK(length(role_assignment_id) = 36),
    machine_role_id TEXT NOT NULL,
    companion_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('UNASSIGNED', 'ASSIGNED', 'SYNCING', 'READY', 'DEGRADED', 'OFFLINE', 'MISMATCH', 'RELEASED')),
    assigned_at_us INTEGER NOT NULL,
    released_at_us INTEGER NULL,
    last_evaluated_at_us INTEGER NOT NULL,
    FOREIGN KEY (machine_role_id) REFERENCES machine_roles(machine_role_id) ON DELETE RESTRICT,
    FOREIGN KEY (companion_id) REFERENCES companions(companion_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX role_assignments_one_active_per_role_idx
    ON role_assignments(machine_role_id)
    WHERE state <> 'RELEASED';

CREATE INDEX role_assignments_companion_idx ON role_assignments(companion_id, state);

-- +goose Down
DROP INDEX IF EXISTS role_assignments_companion_idx;
DROP INDEX IF EXISTS role_assignments_one_active_per_role_idx;
DROP TABLE IF EXISTS role_assignments;
DROP TABLE IF EXISTS machine_roles;
DROP INDEX IF EXISTS companions_last_seen_idx;
DROP TABLE IF EXISTS companions;
