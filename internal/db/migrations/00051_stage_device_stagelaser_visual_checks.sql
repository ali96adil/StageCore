-- +goose Up
-- Human optical check is an operator observation only. It NEVER overwrites
-- device-reported logical state, certifies safe OFF, or grants Cue/SHOW authority.
CREATE TABLE stage_device_stagelaser_visual_checks (
    check_id TEXT PRIMARY KEY CHECK(length(check_id) = 36),
    device_id TEXT NOT NULL REFERENCES stage_devices(device_id) ON DELETE RESTRICT,
    project_id TEXT NOT NULL CHECK(project_id <> ''),
    actor_id TEXT NOT NULL CHECK(actor_id <> ''),
    visual_state TEXT NOT NULL CHECK(visual_state IN ('OFF', 'ON', 'UNKNOWN')),
    device_reported_state TEXT NOT NULL CHECK(device_reported_state <> ''),
    device_connection_state TEXT NOT NULL CHECK(device_connection_state <> ''),
    device_boot_id TEXT NOT NULL CHECK(device_boot_id <> ''),
    checked_at_us INTEGER NOT NULL CHECK(checked_at_us > 0)
);

CREATE INDEX stage_device_stagelaser_visual_checks_device_idx
ON stage_device_stagelaser_visual_checks(device_id, project_id, checked_at_us DESC);

-- +goose Down
DROP INDEX IF EXISTS stage_device_stagelaser_visual_checks_device_idx;
DROP TABLE IF EXISTS stage_device_stagelaser_visual_checks;
