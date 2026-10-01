-- +goose Up
-- A stagecore.device/2 reconnect has already passed authenticated runtime
-- identity validation before Repository.RegisterUnassignedV2 is called.
-- Permit that narrow metadata refresh without weakening Project/assignment
-- authority or reopening the legacy v1 hello path.
DROP TRIGGER stage_device_legacy_hello_update_guard;

-- +goose StatementBegin
CREATE TRIGGER stage_device_legacy_hello_update_guard
BEFORE UPDATE ON stage_devices
WHEN EXISTS (
    SELECT 1 FROM stage_device_assignments
    WHERE device_id = OLD.device_id AND assignment_state <> 'LEGACY'
)
AND NOT (
    -- Explicit Hub disable/revocation remains allowed.
    (
        OLD.enabled = 1 AND NEW.enabled = 0
        AND NEW.device_id IS OLD.device_id
        AND NEW.project_id IS OLD.project_id
        AND NEW.profile_id IS OLD.profile_id
        AND NEW.device_kind IS OLD.device_kind
        AND NEW.display_name IS OLD.display_name
        AND NEW.platform IS OLD.platform
        AND NEW.architecture IS OLD.architecture
        AND NEW.client_version IS OLD.client_version
        AND NEW.protocol_version IS OLD.protocol_version
        AND NEW.capabilities_json IS OLD.capabilities_json
        AND NEW.group_name IS OLD.group_name
        AND NEW.location_name IS OLD.location_name
        AND NEW.created_at_us IS OLD.created_at_us
    )
    OR
    -- Authenticated v2 reconnect metadata refresh. Repository code is the
    -- authenticated caller; this SQL fence limits what that path may mutate.
    (
        OLD.enabled = 1 AND NEW.enabled = 1
        AND OLD.protocol_version = 'stagecore.device/2'
        AND NEW.protocol_version = 'stagecore.device/2'
        AND NEW.device_id IS OLD.device_id
        AND NEW.project_id IS OLD.project_id
        AND NEW.profile_id IS OLD.profile_id
        AND NEW.device_kind IS OLD.device_kind
        AND NEW.platform IS OLD.platform
        AND NEW.architecture IS OLD.architecture
        AND NEW.group_name IS OLD.group_name
        AND NEW.location_name IS OLD.location_name
        AND NEW.created_at_us IS OLD.created_at_us
    )
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_V1_HELLO_FENCED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER stage_device_legacy_hello_update_guard;

-- Restore the schema-30 fence.
-- +goose StatementBegin
CREATE TRIGGER stage_device_legacy_hello_update_guard
BEFORE UPDATE ON stage_devices
WHEN EXISTS (
    SELECT 1 FROM stage_device_assignments
    WHERE device_id = OLD.device_id AND assignment_state <> 'LEGACY'
)
AND NOT (
    OLD.enabled = 1 AND NEW.enabled = 0
    AND NEW.device_id IS OLD.device_id
    AND NEW.project_id IS OLD.project_id
    AND NEW.profile_id IS OLD.profile_id
    AND NEW.device_kind IS OLD.device_kind
    AND NEW.display_name IS OLD.display_name
    AND NEW.platform IS OLD.platform
    AND NEW.architecture IS OLD.architecture
    AND NEW.client_version IS OLD.client_version
    AND NEW.protocol_version IS OLD.protocol_version
    AND NEW.capabilities_json IS OLD.capabilities_json
    AND NEW.group_name IS OLD.group_name
    AND NEW.location_name IS OLD.location_name
    AND NEW.created_at_us IS OLD.created_at_us
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_V1_HELLO_FENCED');
END;
-- +goose StatementEnd
