-- +goose Up
-- Reusable Stage Device v2: preparatory, non-authoritative assignment metadata.
-- All migrated/legacy devices remain under the existing stage_devices.project_id
-- authority until an explicitly reviewed versioned v2 transfer is implemented.
-- This migration does not enable transfers, change runtime.ready, or alter the
-- existing project FK on stage_devices (which currently deletes on project
-- removal and requires a separate audited table rebuild/migration).
CREATE TABLE stage_device_assignments (
    device_id TEXT PRIMARY KEY
        REFERENCES stage_devices(device_id) ON DELETE CASCADE,
    project_id TEXT
        REFERENCES projects(project_id) ON DELETE SET NULL,
    assignment_epoch INTEGER NOT NULL DEFAULT 1
        CHECK(assignment_epoch > 0),
    assignment_state TEXT NOT NULL DEFAULT 'LEGACY'
        CHECK(assignment_state IN ('LEGACY', 'UNASSIGNED', 'PREPARING', 'BLOCKED', 'ACTIVE')),
    runtime_snapshot_id TEXT NOT NULL DEFAULT '',
    updated_at_us INTEGER NOT NULL,
    CHECK(assignment_state <> 'UNASSIGNED' OR (project_id IS NULL AND runtime_snapshot_id = '')),
    CHECK(assignment_state <> 'ACTIVE' OR project_id IS NOT NULL),
    CHECK(assignment_state NOT IN ('PREPARING', 'BLOCKED') OR runtime_snapshot_id = '')
);

-- Preserve existing project affiliation as LEGACY, not ACTIVE: no device has
-- completed the new authenticated assignment or blackout acknowledgment.
INSERT INTO stage_device_assignments
    (device_id, project_id, assignment_epoch, assignment_state,
     runtime_snapshot_id, updated_at_us)
SELECT device_id, project_id, 1, 'LEGACY', '', updated_at_us
FROM stage_devices;

-- A newly paired stagecore.device/1 client is still a legacy registration.
-- Only populate the sidecar; never let it grant project command authority.
-- +goose StatementBegin
CREATE TRIGGER stage_device_assignments_legacy_on_insert
AFTER INSERT ON stage_devices
BEGIN
    INSERT INTO stage_device_assignments
        (device_id, project_id, assignment_epoch, assignment_state,
         runtime_snapshot_id, updated_at_us)
    VALUES (NEW.device_id, NEW.project_id, 1, 'LEGACY', '', NEW.updated_at_us);
END;
-- +goose StatementEnd

-- Until device identity is decoupled from the old project's ON DELETE
-- CASCADE FK, fail closed instead of silently deleting a paired device and its
-- history. A future audited FK migration must replace this temporary guard.
-- +goose StatementBegin
CREATE TRIGGER stage_device_assignments_protect_project_delete
BEFORE DELETE ON projects
WHEN EXISTS (
    SELECT 1 FROM stage_devices WHERE project_id = OLD.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'STAGE_DEVICE_PROJECT_ASSIGNED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS stage_device_assignments_protect_project_delete;
DROP TRIGGER IF EXISTS stage_device_assignments_legacy_on_insert;
DROP TABLE IF EXISTS stage_device_assignments;
