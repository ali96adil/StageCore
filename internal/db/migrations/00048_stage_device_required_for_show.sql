-- +goose Up
-- Per-Project Stage Device rehearsal/show participation.
-- This is intentionally separate from stage_devices.enabled:
-- enabled controls durable Hub identity/command eligibility, while
-- required_for_show controls whether an assigned device participates in
-- generic SHOW readiness checks for its current Project.
ALTER TABLE stage_device_assignments
ADD COLUMN required_for_show INTEGER NOT NULL DEFAULT 1
CHECK(required_for_show IN (0, 1));

-- +goose Down
ALTER TABLE stage_device_assignments
DROP COLUMN required_for_show;
