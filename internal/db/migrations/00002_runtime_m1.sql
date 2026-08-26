-- +goose Up
CREATE TABLE runtime_snapshots (
    runtime_snapshot_id TEXT PRIMARY KEY CHECK(length(runtime_snapshot_id) = 36),
    project_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    snapshot_version INTEGER NOT NULL CHECK(snapshot_version > 0),
    created_at_us INTEGER NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL CHECK(length(content_hash) = 64),
    manifest_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('PUBLISHED', 'SUPERSEDED', 'REVOKED')),
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    FOREIGN KEY (revision_id) REFERENCES project_revisions(revision_id) ON DELETE RESTRICT,
    UNIQUE (project_id, snapshot_version)
);

CREATE INDEX runtime_snapshots_revision_idx ON runtime_snapshots(revision_id);
CREATE INDEX runtime_snapshots_hash_idx ON runtime_snapshots(content_hash);

-- +goose StatementBegin
CREATE TRIGGER runtime_snapshot_content_immutable
BEFORE UPDATE ON runtime_snapshots
WHEN NEW.project_id IS NOT OLD.project_id
  OR NEW.revision_id IS NOT OLD.revision_id
  OR NEW.snapshot_version IS NOT OLD.snapshot_version
  OR NEW.created_at_us IS NOT OLD.created_at_us
  OR NEW.created_by IS NOT OLD.created_by
  OR NEW.content_hash IS NOT OLD.content_hash
  OR NEW.manifest_json IS NOT OLD.manifest_json
BEGIN
    SELECT RAISE(ABORT, 'runtime snapshot content is immutable');
END;
-- +goose StatementEnd

CREATE TABLE sessions (
    session_id TEXT PRIMARY KEY CHECK(length(session_id) = 36),
    project_id TEXT NOT NULL,
    runtime_snapshot_id TEXT NOT NULL,
    session_type TEXT NOT NULL CHECK(session_type IN ('REHEARSAL', 'SHOW', 'SIMULATION')),
    name TEXT NOT NULL DEFAULT '',
    started_at_us INTEGER NOT NULL,
    ended_at_us INTEGER NULL,
    status TEXT NOT NULL CHECK(status IN ('ACTIVE', 'COMPLETED', 'ABORTED')),
    current_cue_id TEXT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    FOREIGN KEY (runtime_snapshot_id) REFERENCES runtime_snapshots(runtime_snapshot_id) ON DELETE RESTRICT,
    FOREIGN KEY (current_cue_id) REFERENCES cues(cue_id) ON DELETE RESTRICT
);

CREATE TABLE cue_executions (
    cue_execution_id TEXT PRIMARY KEY CHECK(length(cue_execution_id) = 36),
    session_id TEXT NOT NULL,
    cue_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    trigger_source TEXT NOT NULL,
    started_at_us INTEGER NOT NULL,
    completed_at_us INTEGER NULL,
    result TEXT NOT NULL CHECK(result IN ('RUNNING', 'COMPLETED', 'FAILED', 'TIMED_OUT', 'CANCELLED')),
    manual_override INTEGER NOT NULL DEFAULT 0 CHECK(manual_override IN (0, 1)),
    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE RESTRICT,
    FOREIGN KEY (cue_id) REFERENCES cues(cue_id) ON DELETE RESTRICT
);

CREATE INDEX cue_executions_session_idx ON cue_executions(session_id, started_at_us);

CREATE TABLE action_executions (
    action_execution_id TEXT PRIMARY KEY CHECK(length(action_execution_id) = 36),
    cue_execution_id TEXT NOT NULL,
    action_id TEXT NOT NULL,
    started_at_us INTEGER NOT NULL,
    completed_at_us INTEGER NULL,
    result TEXT NOT NULL CHECK(result IN ('RUNNING', 'COMPLETED', 'FAILED', 'TIMED_OUT', 'CANCELLED')),
    latency_ms INTEGER NULL CHECK(latency_ms IS NULL OR latency_ms >= 0),
    response_summary TEXT NOT NULL DEFAULT '',
    error_code TEXT NULL,
    FOREIGN KEY (cue_execution_id) REFERENCES cue_executions(cue_execution_id) ON DELETE RESTRICT,
    FOREIGN KEY (action_id) REFERENCES actions(action_id) ON DELETE RESTRICT
);

CREATE INDEX action_executions_cue_idx ON action_executions(cue_execution_id, started_at_us);

CREATE TABLE event_records (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT NOT NULL UNIQUE CHECK(length(event_id) = 36),
    session_id TEXT NULL,
    event_type TEXT NOT NULL,
    schema_version INTEGER NOT NULL CHECK(schema_version > 0),
    occurred_at_us INTEGER NOT NULL,
    observed_at_us INTEGER NOT NULL,
    source_ref TEXT NOT NULL,
    project_id TEXT NOT NULL,
    runtime_snapshot_id TEXT NULL,
    correlation_id TEXT NULL,
    causation_id TEXT NULL,
    priority TEXT NOT NULL CHECK(priority IN ('P0', 'P1', 'P2', 'P3')),
    trace_context_json TEXT NOT NULL DEFAULT '{}',
    payload_json TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE RESTRICT,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    FOREIGN KEY (runtime_snapshot_id) REFERENCES runtime_snapshots(runtime_snapshot_id) ON DELETE RESTRICT
);

CREATE INDEX event_records_session_sequence_idx ON event_records(session_id, sequence);
CREATE INDEX event_records_correlation_idx ON event_records(correlation_id, sequence);

CREATE TABLE command_records (
    command_id TEXT PRIMARY KEY CHECK(length(command_id) = 36),
    command_type TEXT NOT NULL,
    schema_version INTEGER NOT NULL CHECK(schema_version > 0),
    issued_at_us INTEGER NOT NULL,
    project_id TEXT NOT NULL,
    runtime_snapshot_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('ACCEPTED', 'REJECTED', 'COMPLETED', 'FAILED', 'TIMED_OUT', 'CANCELLED')),
    result_json TEXT NOT NULL DEFAULT '{}',
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE RESTRICT,
    FOREIGN KEY (runtime_snapshot_id) REFERENCES runtime_snapshots(runtime_snapshot_id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX command_records_idempotency_idx
    ON command_records(project_id, command_type, idempotency_key)
    WHERE idempotency_key <> '';

-- +goose Down
DROP INDEX IF EXISTS command_records_idempotency_idx;
DROP TABLE IF EXISTS command_records;
DROP INDEX IF EXISTS event_records_correlation_idx;
DROP INDEX IF EXISTS event_records_session_sequence_idx;
DROP TABLE IF EXISTS event_records;
DROP INDEX IF EXISTS action_executions_cue_idx;
DROP TABLE IF EXISTS action_executions;
DROP INDEX IF EXISTS cue_executions_session_idx;
DROP TABLE IF EXISTS cue_executions;
DROP TABLE IF EXISTS sessions;
DROP TRIGGER IF EXISTS runtime_snapshot_content_immutable;
DROP INDEX IF EXISTS runtime_snapshots_hash_idx;
DROP INDEX IF EXISTS runtime_snapshots_revision_idx;
DROP TABLE IF EXISTS runtime_snapshots;
