-- +goose Up
-- Durable audit/idempotency for the future authenticated v2 blackout transfer.
-- This table alone does NOT authorize transfers or verify physical DMX output.
CREATE TABLE stage_device_assignment_transfers (
    transfer_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    from_project_id TEXT NOT NULL DEFAULT '',
    to_project_id TEXT NOT NULL DEFAULT '',
    from_epoch INTEGER NOT NULL CHECK(from_epoch > 0),
    to_epoch INTEGER NOT NULL CHECK(to_epoch = from_epoch + 1),
    connection_generation INTEGER NOT NULL CHECK(connection_generation > 0),
    challenge_sha256 TEXT NOT NULL CHECK(length(challenge_sha256) = 64),
    next_state TEXT NOT NULL CHECK(next_state IN ('BLOCKED', 'UNASSIGNED')),
    committed_at_us INTEGER NOT NULL,
    UNIQUE(device_id, idempotency_key),
    UNIQUE(device_id, challenge_sha256)
);
CREATE INDEX stage_device_assignment_transfers_device_epoch_idx
ON stage_device_assignment_transfers(device_id, to_epoch);

-- +goose Down
DROP INDEX IF EXISTS stage_device_assignment_transfers_device_epoch_idx;
DROP TABLE IF EXISTS stage_device_assignment_transfers;
