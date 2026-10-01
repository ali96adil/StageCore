-- +goose Up
-- Preserve stale Tablet identities for audit/history while removing them from
-- active operational inventory. Decommission never hard-deletes device trust
-- history or prior assignments.
CREATE TABLE stage_device_decommission_audit (
    decommission_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL REFERENCES stage_device_identity_registry(device_id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    assignment_state TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    runtime_snapshot_id TEXT NOT NULL DEFAULT '',
    assignment_epoch INTEGER NOT NULL,
    decommissioned_at_us INTEGER NOT NULL
);
CREATE INDEX stage_device_decommission_audit_device_idx
ON stage_device_decommission_audit(device_id, decommissioned_at_us);

-- +goose Down
DROP INDEX IF EXISTS stage_device_decommission_audit_device_idx;
DROP TABLE IF EXISTS stage_device_decommission_audit;
