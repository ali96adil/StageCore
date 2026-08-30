-- +goose Up
-- F-012 derives the protected Project set from canonical active SHOW Sessions.
-- Runtime/session/event/execution/notes/telemetry tables are intentionally not locked.
CREATE VIEW f012_locked_projects AS
SELECT project_id FROM sessions WHERE session_type = 'SHOW' AND status = 'ACTIVE';

-- +goose StatementBegin
CREATE TRIGGER f012_lock_projects_update BEFORE UPDATE ON projects
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_projects_delete BEFORE DELETE ON projects
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = OLD.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_project_revisions_insert BEFORE INSERT ON project_revisions
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = NEW.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_project_revisions_update BEFORE UPDATE ON project_revisions
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_project_revisions_delete BEFORE DELETE ON project_revisions
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = OLD.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_cues_insert BEFORE INSERT ON cues
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = NEW.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_cues_update BEFORE UPDATE ON cues
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id IN (NEW.revision_id, OLD.revision_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_cues_delete BEFORE DELETE ON cues
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = OLD.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_actions_insert BEFORE INSERT ON actions
WHEN EXISTS (SELECT 1 FROM cues c JOIN project_revisions pr ON pr.revision_id = c.revision_id JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE c.cue_id = NEW.cue_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_actions_update BEFORE UPDATE ON actions
WHEN EXISTS (SELECT 1 FROM cues c JOIN project_revisions pr ON pr.revision_id = c.revision_id JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE c.cue_id IN (NEW.cue_id, OLD.cue_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_actions_delete BEFORE DELETE ON actions
WHEN EXISTS (SELECT 1 FROM cues c JOIN project_revisions pr ON pr.revision_id = c.revision_id JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE c.cue_id = OLD.cue_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_project_device_aliases_insert BEFORE INSERT ON project_device_aliases
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = NEW.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_project_device_aliases_update BEFORE UPDATE ON project_device_aliases
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_project_device_aliases_delete BEFORE DELETE ON project_device_aliases
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = OLD.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_input_definitions_insert BEFORE INSERT ON input_definitions
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = NEW.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_input_definitions_update BEFORE UPDATE ON input_definitions
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id IN (NEW.revision_id, OLD.revision_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_input_definitions_delete BEFORE DELETE ON input_definitions
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = OLD.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_output_definitions_insert BEFORE INSERT ON output_definitions
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = NEW.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_output_definitions_update BEFORE UPDATE ON output_definitions
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id IN (NEW.revision_id, OLD.revision_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_output_definitions_delete BEFORE DELETE ON output_definitions
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = OLD.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_routes_insert BEFORE INSERT ON routes
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = NEW.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_routes_update BEFORE UPDATE ON routes
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id IN (NEW.revision_id, OLD.revision_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_routes_delete BEFORE DELETE ON routes
WHEN EXISTS (SELECT 1 FROM project_revisions pr JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE pr.revision_id = OLD.revision_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_route_actions_insert BEFORE INSERT ON route_actions
WHEN EXISTS (SELECT 1 FROM routes r JOIN project_revisions pr ON pr.revision_id = r.revision_id JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE r.route_id = NEW.route_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_route_actions_update BEFORE UPDATE ON route_actions
WHEN EXISTS (SELECT 1 FROM routes r JOIN project_revisions pr ON pr.revision_id = r.revision_id JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE r.route_id IN (NEW.route_id, OLD.route_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_route_actions_delete BEFORE DELETE ON route_actions
WHEN EXISTS (SELECT 1 FROM routes r JOIN project_revisions pr ON pr.revision_id = r.revision_id JOIN f012_locked_projects lp ON lp.project_id = pr.project_id WHERE r.route_id = OLD.route_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_runtime_snapshots_insert BEFORE INSERT ON runtime_snapshots
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = NEW.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_runtime_snapshots_update BEFORE UPDATE ON runtime_snapshots
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_runtime_snapshots_delete BEFORE DELETE ON runtime_snapshots
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = OLD.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_machine_roles_insert BEFORE INSERT ON machine_roles
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = NEW.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_machine_roles_update BEFORE UPDATE ON machine_roles
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_machine_roles_delete BEFORE DELETE ON machine_roles
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = OLD.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_media_assets_insert BEFORE INSERT ON media_assets
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = NEW.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_media_assets_update BEFORE UPDATE ON media_assets
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id IN (NEW.project_id, OLD.project_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_media_assets_delete BEFORE DELETE ON media_assets
WHEN EXISTS (SELECT 1 FROM f012_locked_projects lp WHERE lp.project_id = OLD.project_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_media_content_versions_insert BEFORE INSERT ON media_content_versions
WHEN EXISTS (SELECT 1 FROM media_assets ma JOIN f012_locked_projects lp ON lp.project_id = ma.project_id WHERE ma.media_asset_id = NEW.media_asset_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_media_content_versions_update BEFORE UPDATE ON media_content_versions
WHEN EXISTS (SELECT 1 FROM media_assets ma JOIN f012_locked_projects lp ON lp.project_id = ma.project_id WHERE ma.media_asset_id IN (NEW.media_asset_id, OLD.media_asset_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_media_content_versions_delete BEFORE DELETE ON media_content_versions
WHEN EXISTS (SELECT 1 FROM media_assets ma JOIN f012_locked_projects lp ON lp.project_id = ma.project_id WHERE ma.media_asset_id = OLD.media_asset_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER f012_lock_machine_role_media_requirements_insert BEFORE INSERT ON machine_role_media_requirements
WHEN EXISTS (SELECT 1 FROM machine_roles mr JOIN f012_locked_projects lp ON lp.project_id = mr.project_id WHERE mr.machine_role_id = NEW.machine_role_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_machine_role_media_requirements_update BEFORE UPDATE ON machine_role_media_requirements
WHEN EXISTS (SELECT 1 FROM machine_roles mr JOIN f012_locked_projects lp ON lp.project_id = mr.project_id WHERE mr.machine_role_id IN (NEW.machine_role_id, OLD.machine_role_id))
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER f012_lock_machine_role_media_requirements_delete BEFORE DELETE ON machine_role_media_requirements
WHEN EXISTS (SELECT 1 FROM machine_roles mr JOIN f012_locked_projects lp ON lp.project_id = mr.project_id WHERE mr.machine_role_id = OLD.machine_role_id)
BEGIN
    SELECT RAISE(ABORT, 'SHOW_CONFIGURATION_LOCKED');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS f012_lock_machine_role_media_requirements_delete;
DROP TRIGGER IF EXISTS f012_lock_machine_role_media_requirements_update;
DROP TRIGGER IF EXISTS f012_lock_machine_role_media_requirements_insert;
DROP TRIGGER IF EXISTS f012_lock_media_content_versions_delete;
DROP TRIGGER IF EXISTS f012_lock_media_content_versions_update;
DROP TRIGGER IF EXISTS f012_lock_media_content_versions_insert;
DROP TRIGGER IF EXISTS f012_lock_media_assets_delete;
DROP TRIGGER IF EXISTS f012_lock_media_assets_update;
DROP TRIGGER IF EXISTS f012_lock_media_assets_insert;
DROP TRIGGER IF EXISTS f012_lock_machine_roles_delete;
DROP TRIGGER IF EXISTS f012_lock_machine_roles_update;
DROP TRIGGER IF EXISTS f012_lock_machine_roles_insert;
DROP TRIGGER IF EXISTS f012_lock_runtime_snapshots_delete;
DROP TRIGGER IF EXISTS f012_lock_runtime_snapshots_update;
DROP TRIGGER IF EXISTS f012_lock_runtime_snapshots_insert;
DROP TRIGGER IF EXISTS f012_lock_route_actions_delete;
DROP TRIGGER IF EXISTS f012_lock_route_actions_update;
DROP TRIGGER IF EXISTS f012_lock_route_actions_insert;
DROP TRIGGER IF EXISTS f012_lock_routes_delete;
DROP TRIGGER IF EXISTS f012_lock_routes_update;
DROP TRIGGER IF EXISTS f012_lock_routes_insert;
DROP TRIGGER IF EXISTS f012_lock_output_definitions_delete;
DROP TRIGGER IF EXISTS f012_lock_output_definitions_update;
DROP TRIGGER IF EXISTS f012_lock_output_definitions_insert;
DROP TRIGGER IF EXISTS f012_lock_input_definitions_delete;
DROP TRIGGER IF EXISTS f012_lock_input_definitions_update;
DROP TRIGGER IF EXISTS f012_lock_input_definitions_insert;
DROP TRIGGER IF EXISTS f012_lock_project_device_aliases_delete;
DROP TRIGGER IF EXISTS f012_lock_project_device_aliases_update;
DROP TRIGGER IF EXISTS f012_lock_project_device_aliases_insert;
DROP TRIGGER IF EXISTS f012_lock_actions_delete;
DROP TRIGGER IF EXISTS f012_lock_actions_update;
DROP TRIGGER IF EXISTS f012_lock_actions_insert;
DROP TRIGGER IF EXISTS f012_lock_cues_delete;
DROP TRIGGER IF EXISTS f012_lock_cues_update;
DROP TRIGGER IF EXISTS f012_lock_cues_insert;
DROP TRIGGER IF EXISTS f012_lock_project_revisions_delete;
DROP TRIGGER IF EXISTS f012_lock_project_revisions_update;
DROP TRIGGER IF EXISTS f012_lock_project_revisions_insert;
DROP TRIGGER IF EXISTS f012_lock_projects_delete;
DROP TRIGGER IF EXISTS f012_lock_projects_update;
DROP VIEW IF EXISTS f012_locked_projects;
