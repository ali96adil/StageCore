-- +goose Up
-- Stage Device command authority now has two explicitly supported paths:
-- 1) legacy v1 Project-bound devices; and
-- 2) project-independent v2 Tablet Players after the Hub has committed ACTIVE
--    Project + Runtime Snapshot authority and the runtime path has separately
--    acknowledged that exact scope.
--
-- Keep the SQLite write boundary fail-closed. Lighting v2, UNASSIGNED/BLOCKED
-- devices, stale snapshots, disabled devices and client-owned Project claims
-- remain unable to insert a command even if a caller bypasses Repository code.
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
      )
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_ASSIGNMENT_FENCED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER stage_device_legacy_command_insert_guard;

-- Restore schema-30 behavior: only legacy v1 commands are executable.
-- +goose StatementBegin
CREATE TRIGGER stage_device_legacy_command_insert_guard
BEFORE INSERT ON stage_device_commands
WHEN NOT EXISTS (
    SELECT 1 FROM stage_device_assignments a
    JOIN stage_devices d ON d.device_id = a.device_id
    WHERE a.device_id = NEW.device_id
      AND a.assignment_state = 'LEGACY'
      AND a.project_id = NEW.project_id
      AND d.project_id = NEW.project_id
      AND d.enabled = 1
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_ASSIGNMENT_FENCED');
END;
-- +goose StatementEnd
