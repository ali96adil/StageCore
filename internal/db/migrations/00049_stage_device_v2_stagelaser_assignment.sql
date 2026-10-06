-- +goose Up
-- StageLaser v2 assignment is authorized only after the exact authenticated
-- device socket proves DISARMED + OFF with a known stable logical state.
CREATE TABLE stage_device_stagelaser_assignment_audit (
    assignment_id TEXT PRIMARY KEY CHECK(length(assignment_id) = 36),
    device_id TEXT NOT NULL
        REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL CHECK(actor_id <> ''),
    project_id TEXT NOT NULL CHECK(project_id <> ''),
    runtime_snapshot_id TEXT NOT NULL CHECK(runtime_snapshot_id <> ''),
    from_epoch INTEGER NOT NULL CHECK(from_epoch > 0),
    to_epoch INTEGER NOT NULL CHECK(to_epoch = from_epoch + 1),
    connection_generation INTEGER NOT NULL CHECK(connection_generation > 0),
    challenge_sha256 TEXT NOT NULL CHECK(length(challenge_sha256) = 64),
    state_quality TEXT NOT NULL CHECK(state_quality IN ('TRACKED', 'CONFIRMED')),
    committed_at_us INTEGER NOT NULL CHECK(committed_at_us > 0),
    UNIQUE(device_id, to_epoch),
    UNIQUE(device_id, challenge_sha256)
);

CREATE INDEX stage_device_stagelaser_assignment_audit_device_idx
ON stage_device_stagelaser_assignment_audit(device_id, committed_at_us DESC);

-- Extend the fail-closed command write boundary with exactly one additional
-- v2 authority path: an ACTIVE StageLaser scope backed by the safe-state audit.
DROP TRIGGER stage_device_legacy_command_insert_guard;

-- +goose StatementBegin
CREATE TRIGGER stage_device_legacy_command_insert_guard
BEFORE INSERT ON stage_device_commands
WHEN NOT EXISTS (
    SELECT 1
    FROM stage_device_assignments a
    JOIN stage_devices d ON d.device_id = a.device_id
    WHERE a.device_id = NEW.device_id
      AND d.enabled = 1
      AND (
          (
              d.protocol_version = 'stagecore.device/1'
              AND a.assignment_state = 'LEGACY'
              AND a.project_id = NEW.project_id
              AND d.project_id = NEW.project_id
          )
          OR
          (
              d.protocol_version = 'stagecore.device/2'
              AND d.device_kind = 'TABLET_PLAYER'
              AND COALESCE(d.profile_id, '') = 'stagecore.tablet-player'
              AND d.project_id IS NULL
              AND a.assignment_state = 'ACTIVE'
              AND a.project_id = NEW.project_id
              AND a.runtime_snapshot_id <> ''
              AND a.runtime_snapshot_id = COALESCE(NEW.runtime_snapshot_id, '')
          )
          OR
          (
              d.protocol_version = 'stagecore.device/2'
              AND COALESCE(d.profile_id, '') = 'stagecore.esp32-dmx-lighting-node'
              AND d.project_id IS NULL
              AND a.assignment_state = 'ACTIVE'
              AND a.project_id = NEW.project_id
              AND a.runtime_snapshot_id <> ''
              AND a.runtime_snapshot_id = COALESCE(NEW.runtime_snapshot_id, '')
              AND EXISTS (
                  SELECT 1 FROM stage_device_lighting_activation_audit x
                  WHERE x.device_id = a.device_id
                    AND x.project_id = a.project_id
                    AND x.runtime_snapshot_id = a.runtime_snapshot_id
                    AND x.assignment_epoch = a.assignment_epoch
              )
          )
          OR
          (
              d.protocol_version = 'stagecore.device/2'
              AND d.device_kind = 'GENERIC'
              AND COALESCE(d.profile_id, '') = 'stagecore.esp32-stagelaser'
              AND d.project_id IS NULL
              AND a.assignment_state = 'ACTIVE'
              AND a.project_id = NEW.project_id
              AND a.runtime_snapshot_id <> ''
              AND a.runtime_snapshot_id = COALESCE(NEW.runtime_snapshot_id, '')
              AND EXISTS (
                  SELECT 1 FROM stage_device_stagelaser_assignment_audit x
                  WHERE x.device_id = a.device_id
                    AND x.project_id = a.project_id
                    AND x.runtime_snapshot_id = a.runtime_snapshot_id
                    AND x.to_epoch = a.assignment_epoch
              )
          )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_ASSIGNMENT_FENCED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER stage_device_legacy_command_insert_guard;
DROP INDEX IF EXISTS stage_device_stagelaser_assignment_audit_device_idx;
DROP TABLE IF EXISTS stage_device_stagelaser_assignment_audit;

-- Restore schema-38 command authority: legacy v1 + v2 Tablet + audited v2 Lighting.
-- +goose StatementBegin
CREATE TRIGGER stage_device_legacy_command_insert_guard
BEFORE INSERT ON stage_device_commands
WHEN NOT EXISTS (
    SELECT 1
    FROM stage_device_assignments a
    JOIN stage_devices d ON d.device_id = a.device_id
    WHERE a.device_id = NEW.device_id
      AND d.enabled = 1
      AND (
          (
              d.protocol_version = 'stagecore.device/1'
              AND a.assignment_state = 'LEGACY'
              AND a.project_id = NEW.project_id
              AND d.project_id = NEW.project_id
          )
          OR
          (
              d.protocol_version = 'stagecore.device/2'
              AND d.device_kind = 'TABLET_PLAYER'
              AND COALESCE(d.profile_id, '') = 'stagecore.tablet-player'
              AND d.project_id IS NULL
              AND a.assignment_state = 'ACTIVE'
              AND a.project_id = NEW.project_id
              AND a.runtime_snapshot_id <> ''
              AND a.runtime_snapshot_id = COALESCE(NEW.runtime_snapshot_id, '')
          )
          OR
          (
              d.protocol_version = 'stagecore.device/2'
              AND COALESCE(d.profile_id, '') = 'stagecore.esp32-dmx-lighting-node'
              AND d.project_id IS NULL
              AND a.assignment_state = 'ACTIVE'
              AND a.project_id = NEW.project_id
              AND a.runtime_snapshot_id <> ''
              AND a.runtime_snapshot_id = COALESCE(NEW.runtime_snapshot_id, '')
              AND EXISTS (
                  SELECT 1 FROM stage_device_lighting_activation_audit x
                  WHERE x.device_id = a.device_id
                    AND x.project_id = a.project_id
                    AND x.runtime_snapshot_id = a.runtime_snapshot_id
                    AND x.assignment_epoch = a.assignment_epoch
              )
          )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_ASSIGNMENT_FENCED');
END;
-- +goose StatementEnd
