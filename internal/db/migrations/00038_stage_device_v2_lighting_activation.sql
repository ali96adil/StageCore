-- +goose Up
-- Audited activation of a project-independent v2 Lighting Node.
-- Transfer/epoch ACK leaves the node BLOCKED. Only a separately verified
-- configuration activation may bind a published Runtime Snapshot and grant
-- ACTIVE command authority.
CREATE TABLE stage_device_lighting_activation_audit (
    activation_id TEXT PRIMARY KEY CHECK(length(activation_id) = 36),
    device_id TEXT NOT NULL
        REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL CHECK(actor_id <> ''),
    project_id TEXT NOT NULL CHECK(project_id <> ''),
    runtime_snapshot_id TEXT NOT NULL CHECK(runtime_snapshot_id <> ''),
    assignment_epoch INTEGER NOT NULL CHECK(assignment_epoch > 1),
    connection_generation INTEGER NOT NULL CHECK(connection_generation > 0),
    challenge_sha256 TEXT NOT NULL CHECK(length(challenge_sha256) = 64),
    configuration_sha256 TEXT NOT NULL CHECK(length(configuration_sha256) = 64),
    committed_at_us INTEGER NOT NULL CHECK(committed_at_us > 0),
    UNIQUE(device_id, assignment_epoch, runtime_snapshot_id),
    UNIQUE(device_id, challenge_sha256)
);

CREATE INDEX stage_device_lighting_activation_device_idx
ON stage_device_lighting_activation_audit(device_id, committed_at_us DESC);

-- Schema 32 intentionally allowed only UNASSIGNED/BLOCKED transfer
-- reservations. Once audited ACTIVE lighting exists, a node must be able to
-- leave that Project without reprovisioning. Retain the same fail-closed
-- SHOW/command/project checks and additionally require the canonical activation
-- audit for an ACTIVE source scope.
DROP TRIGGER stage_device_transfer_insert_guard;

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
        AND (
            (
                a.assignment_state IN ('UNASSIGNED', 'BLOCKED')
                AND a.runtime_snapshot_id = ''
            )
            OR
            (
                a.assignment_state = 'ACTIVE'
                AND a.project_id IS NOT NULL
                AND a.runtime_snapshot_id <> ''
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

-- Extend the fail-closed command boundary with one additional authority path:
-- exact ACTIVE v2 Lighting Node + exact Hub-owned Project/Runtime Snapshot.
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
      )
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_ASSIGNMENT_FENCED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER stage_device_legacy_command_insert_guard;
DROP TRIGGER stage_device_transfer_insert_guard;
DROP INDEX IF EXISTS stage_device_lighting_activation_device_idx;
DROP TABLE IF EXISTS stage_device_lighting_activation_audit;

-- Restore the schema-32 transfer reservation guard: no ACTIVE source scope.
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

-- Restore schema-37 authority: legacy v1 + ACTIVE v2 Tablet only.
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
      )
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_ASSIGNMENT_FENCED');
END;
-- +goose StatementEnd
