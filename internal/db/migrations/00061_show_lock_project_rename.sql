-- +goose Up
-- A Project title does not change any published or active runtime identity.
-- Keep the configuration lock on all other Project-row changes while allowing
-- the metadata-only rename operation during an active SHOW.
DROP TRIGGER IF EXISTS f012_lock_projects_update;

-- +goose StatementBegin
CREATE TRIGGER f012_lock_projects_update BEFORE UPDATE ON projects
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
AND NOT (
    NEW.project_id IS OLD.project_id
    AND NEW.name IS NOT OLD.name
    AND NEW.description IS OLD.description
    AND NEW.lifecycle_state IS OLD.lifecycle_state
    AND NEW.current_revision_id IS OLD.current_revision_id
    AND NEW.default_venue_profile_id IS OLD.default_venue_profile_id
    AND NEW.created_at_us IS OLD.created_at_us
    AND NEW.updated_at_us IS NOT OLD.updated_at_us
)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS f012_lock_projects_update;

-- +goose StatementBegin
CREATE TRIGGER f012_lock_projects_update BEFORE UPDATE ON projects
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
