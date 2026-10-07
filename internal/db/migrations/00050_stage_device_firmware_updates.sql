-- +goose Up
-- Firmware maintenance is intentionally separate from stage_device_commands:
-- it has no Cue/session authority and is bound to one exact authenticated v2
-- device plus a short-lived immutable update manifest.
CREATE TABLE stage_device_firmware_updates (
    update_id TEXT PRIMARY KEY CHECK(length(update_id) = 36),
    device_id TEXT NOT NULL
        REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    profile_id TEXT NOT NULL CHECK(profile_id <> ''),
    current_version TEXT NOT NULL CHECK(current_version <> ''),
    target_version TEXT NOT NULL CHECK(target_version <> ''),
    source_revision TEXT NOT NULL CHECK(length(source_revision) = 40),
    artifact_path TEXT NOT NULL CHECK(artifact_path <> ''),
    artifact_size INTEGER NOT NULL CHECK(artifact_size > 0),
    artifact_sha256 TEXT NOT NULL CHECK(length(artifact_sha256) = 64),
    rollback_required INTEGER NOT NULL CHECK(rollback_required IN (0, 1)),
    manifest_json TEXT NOT NULL CHECK(json_valid(manifest_json)),
    state TEXT NOT NULL CHECK(state IN (
        'ISSUED', 'SENT', 'ACCEPTED', 'DOWNLOADING', 'VERIFYING',
        'WRITING', 'REBOOTING', 'COMPLETED', 'REJECTED', 'FAILED',
        'EXPIRED', 'INTERRUPTED'
    )),
    issued_by TEXT NOT NULL CHECK(issued_by <> ''),
    issued_at_us INTEGER NOT NULL CHECK(issued_at_us > 0),
    expires_at_us INTEGER NOT NULL CHECK(expires_at_us > issued_at_us),
    connection_generation INTEGER
        CHECK(connection_generation IS NULL OR connection_generation > 0),
    last_detail TEXT NOT NULL DEFAULT '',
    last_error_code TEXT NOT NULL DEFAULT '',
    updated_at_us INTEGER NOT NULL CHECK(updated_at_us > 0)
);

CREATE INDEX stage_device_firmware_updates_device_idx
ON stage_device_firmware_updates(device_id, issued_at_us DESC);

CREATE INDEX stage_device_firmware_updates_state_idx
ON stage_device_firmware_updates(state, updated_at_us DESC);

-- There may be only one actively delivered update per physical device.
-- ISSUED records are not included because an operator may abandon an issued
-- manifest and issue another; send-time selection remains explicit.
CREATE UNIQUE INDEX stage_device_firmware_updates_active_device_idx
ON stage_device_firmware_updates(device_id)
WHERE state IN ('SENT', 'ACCEPTED', 'DOWNLOADING', 'VERIFYING', 'WRITING', 'REBOOTING');

-- +goose Down
DROP INDEX IF EXISTS stage_device_firmware_updates_active_device_idx;
DROP INDEX IF EXISTS stage_device_firmware_updates_state_idx;
DROP INDEX IF EXISTS stage_device_firmware_updates_device_idx;
DROP TABLE IF EXISTS stage_device_firmware_updates;
