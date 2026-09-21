-- +goose Up
-- Non-LEGACY assignment metadata is NOT executable by the v1 runtime. Fence
-- all legacy command insertions at the SQLite write boundary, including SQL
-- callers that bypass Repository.CreateCommand. This is intentionally NOT a
-- v2 command path: even ACTIVE v2 records stay dark until separately reviewed
-- authenticated epoch-scoped dispatch is implemented.
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

-- A legacy device.hello cannot quietly reset a PREPARING/BLOCKED/ACTIVE v2
-- assignment or change the device's legacy identity metadata. Dedicated Hub
-- control-plane migrations cannot reuse this ordinary v1 upsert path.
-- +goose StatementBegin
CREATE TRIGGER stage_device_legacy_hello_update_guard
BEFORE UPDATE ON stage_devices
WHEN EXISTS (
    SELECT 1 FROM stage_device_assignments
    WHERE device_id = OLD.device_id AND assignment_state <> 'LEGACY'
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_V1_HELLO_FENCED');
END;
-- +goose StatementEnd

-- A state change immediately removes the stored READY indication even if the
-- device goes offline before it can publish another observation.
-- +goose StatementBegin
CREATE TRIGGER stage_device_assignment_block_readiness
AFTER UPDATE OF assignment_state, project_id ON stage_device_assignments
WHEN NEW.assignment_state <> 'LEGACY'
BEGIN
    UPDATE stage_device_runtime_state SET readiness = 'BLOCKER'
    WHERE device_id = NEW.device_id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS stage_device_assignment_block_readiness;
DROP TRIGGER IF EXISTS stage_device_legacy_hello_update_guard;
DROP TRIGGER IF EXISTS stage_device_legacy_command_insert_guard;
