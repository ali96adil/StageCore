-- +goose Up
-- Audited one-way migration for a known project-linked v1 ESP32 lighting node.
-- The migration removes legacy command authority and moves the assignment into
-- BLOCKED v2 state. It does not activate a Runtime Snapshot, enable commands,
-- or claim physical blackout verification.
CREATE TABLE stage_device_legacy_v2_migrations (
    migration_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL
        REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL,
    project_id TEXT NOT NULL
        REFERENCES projects(project_id) ON DELETE RESTRICT,
    from_protocol TEXT NOT NULL
        CHECK(from_protocol = 'stagecore.device/1'),
    to_protocol TEXT NOT NULL
        CHECK(to_protocol = 'stagecore.device/2'),
    from_epoch INTEGER NOT NULL CHECK(from_epoch > 0),
    to_epoch INTEGER NOT NULL CHECK(to_epoch = from_epoch + 1),
    next_state TEXT NOT NULL CHECK(next_state = 'BLOCKED'),
    migrated_at_us INTEGER NOT NULL,
    UNIQUE(device_id)
);

-- +goose Down
DROP TABLE IF EXISTS stage_device_legacy_v2_migrations;
