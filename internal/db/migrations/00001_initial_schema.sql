-- +goose Up
CREATE TABLE projects (
    project_id TEXT PRIMARY KEY CHECK(length(project_id) = 36),
    name TEXT NOT NULL CHECK(length(trim(name)) > 0),
    description TEXT NOT NULL DEFAULT '',
    lifecycle_state TEXT NOT NULL CHECK(lifecycle_state IN ('ACTIVE', 'FINAL', 'ARCHIVED')),
    current_revision_id TEXT NULL,
    default_venue_profile_id TEXT NULL,
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (current_revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE project_revisions (
    revision_id TEXT PRIMARY KEY CHECK(length(revision_id) = 36),
    project_id TEXT NOT NULL,
    revision_number INTEGER NOT NULL CHECK(revision_number > 0),
    status TEXT NOT NULL CHECK(status IN ('DRAFT', 'VALIDATED', 'SUPERSEDED')),
    parent_revision_id TEXT NULL,
    created_at_us INTEGER NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    change_note TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    FOREIGN KEY (parent_revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    UNIQUE (project_id, revision_number)
);

CREATE UNIQUE INDEX one_draft_revision_per_project
    ON project_revisions(project_id)
    WHERE status = 'DRAFT';

CREATE TABLE cues (
    cue_id TEXT PRIMARY KEY CHECK(length(cue_id) = 36),
    revision_id TEXT NOT NULL,
    display_label TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL CHECK(length(trim(name)) > 0),
    order_index INTEGER NOT NULL CHECK(order_index >= 0),
    cue_type TEXT NOT NULL DEFAULT 'STANDARD',
    criticality TEXT NOT NULL DEFAULT 'NORMAL' CHECK(criticality IN ('NORMAL', 'CRITICAL', 'SAFETY_CRITICAL')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    execution_policy_json TEXT NOT NULL DEFAULT '{}',
    notes_summary TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    UNIQUE (revision_id, order_index)
);

CREATE TABLE actions (
    action_id TEXT PRIMARY KEY CHECK(length(action_id) = 36),
    cue_id TEXT NOT NULL,
    order_index INTEGER NOT NULL CHECK(order_index >= 0),
    execution_mode TEXT NOT NULL CHECK(execution_mode IN ('SEQUENTIAL', 'PARALLEL', 'PARALLEL_BARRIER')),
    target_ref TEXT NOT NULL,
    capability_key TEXT NOT NULL CHECK(length(trim(capability_key)) > 0),
    parameters_json TEXT NOT NULL DEFAULT '{}',
    timeout_policy_json TEXT NOT NULL DEFAULT '{}',
    error_policy_json TEXT NOT NULL DEFAULT '{}',
    priority_class TEXT NOT NULL CHECK(priority_class IN ('P0', 'P1', 'P2', 'P3')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    FOREIGN KEY (cue_id) REFERENCES cues(cue_id) ON DELETE CASCADE,
    UNIQUE (cue_id, order_index)
);

CREATE TABLE project_device_aliases (
    alias_id TEXT PRIMARY KEY CHECK(length(alias_id) = 36),
    project_id TEXT NOT NULL,
    logical_name TEXT NOT NULL CHECK(length(trim(logical_name)) > 0),
    logical_type TEXT NOT NULL DEFAULT 'GENERIC',
    target_ref TEXT NOT NULL DEFAULT '',
    group_name TEXT NOT NULL DEFAULT '',
    project_config_json TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    UNIQUE (project_id, logical_name)
);

CREATE TABLE input_definitions (
    input_id TEXT PRIMARY KEY CHECK(length(input_id) = 36),
    revision_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK(length(trim(name)) > 0),
    source_ref TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK(length(trim(event_type)) > 0),
    value_schema_json TEXT NOT NULL DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    UNIQUE (revision_id, name)
);

CREATE TABLE output_definitions (
    output_id TEXT PRIMARY KEY CHECK(length(output_id) = 36),
    revision_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK(length(trim(name)) > 0),
    target_ref TEXT NOT NULL,
    capability_key TEXT NOT NULL CHECK(length(trim(capability_key)) > 0),
    value_schema_json TEXT NOT NULL DEFAULT '{}',
    criticality TEXT NOT NULL DEFAULT 'NORMAL' CHECK(criticality IN ('NORMAL', 'CRITICAL', 'SAFETY_CRITICAL')),
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    UNIQUE (revision_id, name)
);

CREATE TABLE routes (
    route_id TEXT PRIMARY KEY CHECK(length(route_id) = 36),
    revision_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK(length(trim(name)) > 0),
    input_id TEXT NOT NULL,
    condition_definition_json TEXT NOT NULL DEFAULT 'null',
    transform_definition_json TEXT NOT NULL DEFAULT 'null',
    delay_ms INTEGER NULL CHECK(delay_ms IS NULL OR delay_ms >= 0),
    debounce_ms INTEGER NULL CHECK(debounce_ms IS NULL OR debounce_ms >= 0),
    priority_class TEXT NOT NULL CHECK(priority_class IN ('P0', 'P1', 'P2', 'P3')),
    error_policy_json TEXT NOT NULL DEFAULT '{}',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    FOREIGN KEY (input_id) REFERENCES input_definitions(input_id) ON DELETE RESTRICT,
    UNIQUE (revision_id, name)
);

CREATE TABLE route_actions (
    route_action_id TEXT PRIMARY KEY CHECK(length(route_action_id) = 36),
    route_id TEXT NOT NULL,
    order_index INTEGER NOT NULL CHECK(order_index >= 0),
    output_id TEXT NULL,
    cue_id TEXT NULL,
    parameters_json TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (route_id) REFERENCES routes(route_id) ON DELETE CASCADE,
    FOREIGN KEY (output_id) REFERENCES output_definitions(output_id) ON DELETE RESTRICT,
    FOREIGN KEY (cue_id) REFERENCES cues(cue_id) ON DELETE RESTRICT,
    CHECK ((output_id IS NOT NULL AND cue_id IS NULL) OR (output_id IS NULL AND cue_id IS NOT NULL)),
    UNIQUE (route_id, order_index)
);

-- +goose Down
DROP TABLE IF EXISTS route_actions;
DROP TABLE IF EXISTS routes;
DROP TABLE IF EXISTS output_definitions;
DROP TABLE IF EXISTS input_definitions;
DROP TABLE IF EXISTS project_device_aliases;
DROP TABLE IF EXISTS actions;
DROP TABLE IF EXISTS cues;
DROP INDEX IF EXISTS one_draft_revision_per_project;
DROP TABLE IF EXISTS project_revisions;
DROP TABLE IF EXISTS projects;
