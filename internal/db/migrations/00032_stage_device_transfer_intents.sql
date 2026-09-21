-- +goose Up
-- Durable *intent* only. This table cannot grant assignment authority, emit
-- a DMX frame, prove blackout, or commit a Project transfer. It reserves one
-- current authenticated transport generation for future blackout negotiation.
CREATE TABLE stage_device_transfer_intents (
    transfer_id TEXT PRIMARY KEY CHECK(length(transfer_id) = 36),
    device_id TEXT NOT NULL REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    from_project_id TEXT,
    to_project_id TEXT,
    expected_epoch INTEGER NOT NULL CHECK(expected_epoch > 0 AND expected_epoch < 9223372036854775807),
    connection_generation INTEGER NOT NULL CHECK(connection_generation > 0),
    expected_channels INTEGER NOT NULL CHECK(expected_channels BETWEEN 1 AND 512),
    challenge_sha256 TEXT NOT NULL CHECK(length(challenge_sha256) = 64),
    status TEXT NOT NULL CHECK(status IN ('PENDING', 'EXPIRED', 'CANCELLED', 'ACKED', 'COMMITTED')),
    requested_by TEXT NOT NULL CHECK(requested_by <> ''),
    created_at_us INTEGER NOT NULL,
    expires_at_us INTEGER NOT NULL CHECK(expires_at_us > created_at_us),
    updated_at_us INTEGER NOT NULL,
    CHECK(from_project_id IS NOT to_project_id)
);

-- A second transfer cannot silently replace a first in-flight operation.
CREATE UNIQUE INDEX stage_device_transfer_pending_unique
    ON stage_device_transfer_intents(device_id)
    WHERE status IN ('PENDING', 'ACKED');

CREATE INDEX stage_device_transfer_recent_idx
    ON stage_device_transfer_intents(device_id, created_at_us DESC);

-- A raw SQL insert must pass the same preflight invariants as the repository;
-- a matching client-provided Project ID alone cannot create an intent.
-- +goose StatementBegin
CREATE TRIGGER stage_device_transfer_insert_guard
BEFORE INSERT ON stage_device_transfer_intents
WHEN NEW.status <> 'PENDING'
  OR NOT EXISTS (
      SELECT 1
      FROM stage_devices d
      JOIN stage_device_assignments a ON a.device_id = d.device_id
      WHERE d.device_id = NEW.device_id
        AND d.protocol_version = 'stagecore.device/2'
        AND d.enabled = 1
        AND d.project_id IS NULL
        AND d.profile_id = 'stagecore.esp32-dmx-lighting-node'
        AND a.project_id IS NEW.from_project_id
        AND a.assignment_epoch = NEW.expected_epoch
        AND a.assignment_state IN ('UNASSIGNED', 'BLOCKED')
        AND a.runtime_snapshot_id = ''
  )
  OR (NEW.from_project_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM projects p WHERE p.project_id = NEW.from_project_id
  ))
  OR (NEW.to_project_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM projects p WHERE p.project_id = NEW.to_project_id
  ))
  OR EXISTS (
      SELECT 1 FROM stage_device_commands c
      WHERE c.device_id = NEW.device_id AND c.status = 'ACCEPTED'
  )
  OR EXISTS (
      SELECT 1 FROM sessions s
      WHERE s.session_type = 'SHOW' AND s.status = 'ACTIVE'
        AND (s.project_id = NEW.from_project_id OR s.project_id = NEW.to_project_id)
  )
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_TRANSFER_PREFLIGHT_FENCED');
END;
-- +goose StatementEnd

-- An expired or cancelled intent is a historical record, never resumable.
-- ACKED/COMMITTED must be added by a separately reviewed authenticated
-- transport + transactional CAS implementation. This slice cannot write them.
-- +goose StatementBegin
CREATE TRIGGER stage_device_transfer_status_guard
BEFORE UPDATE ON stage_device_transfer_intents
WHEN NOT (
    OLD.status = 'PENDING' AND NEW.status IN ('EXPIRED', 'CANCELLED')
    AND NEW.transfer_id IS OLD.transfer_id
    AND NEW.device_id IS OLD.device_id
    AND NEW.from_project_id IS OLD.from_project_id
    AND NEW.to_project_id IS OLD.to_project_id
    AND NEW.expected_epoch IS OLD.expected_epoch
    AND NEW.connection_generation IS OLD.connection_generation
    AND NEW.expected_channels IS OLD.expected_channels
    AND NEW.challenge_sha256 IS OLD.challenge_sha256
    AND NEW.requested_by IS OLD.requested_by
    AND NEW.created_at_us IS OLD.created_at_us
    AND NEW.expires_at_us IS OLD.expires_at_us
    AND NEW.updated_at_us >= OLD.updated_at_us
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_TRANSFER_TRANSITION_FENCED');
END;
-- +goose StatementEnd

-- A v2 BLOCKED/ACTIVE node must be explicitly unassigned (new epoch +
-- blackout) before deleting its currently owning Project. The v1 deletion
-- guard in migration 29 only checks stage_devices.project_id; v2 keeps its
-- Hub-owned project in the independent sidecar.
-- +goose StatementBegin
CREATE TRIGGER stage_device_v2_assignment_protect_project_delete
BEFORE DELETE ON projects
WHEN EXISTS (
    SELECT 1 FROM stage_device_assignments
    WHERE project_id = OLD.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_PROJECT_ASSIGNED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS stage_device_v2_assignment_protect_project_delete;
DROP TRIGGER IF EXISTS stage_device_transfer_status_guard;
DROP TRIGGER IF EXISTS stage_device_transfer_insert_guard;
DROP INDEX IF EXISTS stage_device_transfer_recent_idx;
DROP INDEX IF EXISTS stage_device_transfer_pending_unique;
DROP TABLE IF EXISTS stage_device_transfer_intents;
