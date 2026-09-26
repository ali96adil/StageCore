-- +goose Up
-- Project-independent Tablet Player v2 assignment audit.
-- A row is written only by the trusted Hub after a current authenticated
-- tablet socket reports the dedicated assignment safe-media state.
CREATE TABLE stage_device_tablet_assignment_audit (
    assignment_id TEXT PRIMARY KEY CHECK(length(assignment_id) = 36),
    device_id TEXT NOT NULL
        REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL CHECK(actor_id <> ''),
    from_project_id TEXT NOT NULL DEFAULT '',
    to_project_id TEXT NOT NULL DEFAULT '',
    from_runtime_snapshot_id TEXT NOT NULL DEFAULT '',
    to_runtime_snapshot_id TEXT NOT NULL DEFAULT '',
    from_epoch INTEGER NOT NULL CHECK(from_epoch > 0),
    to_epoch INTEGER NOT NULL CHECK(to_epoch = from_epoch + 1),
    connection_generation INTEGER NOT NULL CHECK(connection_generation > 0),
    challenge_sha256 TEXT NOT NULL CHECK(length(challenge_sha256) = 64),
    next_state TEXT NOT NULL CHECK(next_state IN ('ACTIVE', 'UNASSIGNED')),
    committed_at_us INTEGER NOT NULL CHECK(committed_at_us > 0),
    UNIQUE(device_id, to_epoch),
    UNIQUE(device_id, challenge_sha256),
    CHECK(
        (next_state = 'ACTIVE' AND to_project_id <> '' AND to_runtime_snapshot_id <> '')
        OR
        (next_state = 'UNASSIGNED' AND to_project_id = '' AND to_runtime_snapshot_id = '')
    )
);

CREATE INDEX stage_device_tablet_assignment_audit_device_idx
ON stage_device_tablet_assignment_audit(device_id, committed_at_us DESC);

-- +goose Down
DROP INDEX IF EXISTS stage_device_tablet_assignment_audit_device_idx;
DROP TABLE IF EXISTS stage_device_tablet_assignment_audit;
