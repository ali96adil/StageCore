-- +goose Up
-- Phase 4 shared Stage Device persistence for F-003/F-006/F-007/F-022.
-- Runtime observations remain distinct from desired configuration so reconnects
-- cannot silently convert stale observed state into operator intent.
CREATE TABLE stage_devices (
    device_id TEXT PRIMARY KEY,
    project_id TEXT,
    profile_id TEXT,
    device_kind TEXT NOT NULL CHECK(device_kind IN ('TABLET_PLAYER', 'STAGE_DISPLAY', 'RENDER_NODE', 'GENERIC')),
    display_name TEXT NOT NULL,
    platform TEXT NOT NULL DEFAULT '',
    architecture TEXT NOT NULL DEFAULT '',
    client_version TEXT NOT NULL DEFAULT '',
    protocol_version TEXT NOT NULL DEFAULT 'stagecore.device/1',
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    group_name TEXT NOT NULL DEFAULT '',
    location_name TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE CASCADE
);

CREATE INDEX stage_devices_project_kind_idx
    ON stage_devices(project_id, device_kind, display_name);

CREATE TABLE stage_device_runtime_state (
    device_id TEXT PRIMARY KEY,
    connection_state TEXT NOT NULL CHECK(connection_state IN ('ONLINE', 'OFFLINE', 'STALE', 'REVOKED')),
    readiness TEXT NOT NULL CHECK(readiness IN ('READY', 'WARNING', 'ADVISORY', 'BLOCKER', 'UNKNOWN')),
    last_seen_at_us INTEGER NOT NULL,
    observed_state_json TEXT NOT NULL DEFAULT '{}',
    network_state_json TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (device_id) REFERENCES stage_devices(device_id) ON DELETE CASCADE
);

CREATE INDEX stage_device_runtime_last_seen_idx
    ON stage_device_runtime_state(last_seen_at_us);

CREATE TABLE stage_device_commands (
    command_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT,
    device_id TEXT NOT NULL,
    command_type TEXT NOT NULL,
    runtime_snapshot_id TEXT NOT NULL DEFAULT '',
    issued_at_us INTEGER NOT NULL,
    deadline_at_us INTEGER,
    issuer TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL DEFAULT 'P1',
    idempotency_key TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL CHECK(status IN ('ACCEPTED', 'REJECTED', 'COMPLETED', 'FAILED', 'TIMED_OUT', 'CANCELLED')),
    result_json TEXT,
    completed_at_us INTEGER,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE SET NULL,
    FOREIGN KEY (device_id) REFERENCES stage_devices(device_id) ON DELETE CASCADE
);

CREATE INDEX stage_device_commands_device_time_idx
    ON stage_device_commands(device_id, issued_at_us DESC);
CREATE INDEX stage_device_commands_correlation_idx
    ON stage_device_commands(correlation_id, issued_at_us DESC);
CREATE UNIQUE INDEX stage_device_commands_idempotency_idx
    ON stage_device_commands(device_id, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE stage_display_state (
    device_id TEXT PRIMARY KEY,
    mode TEXT NOT NULL CHECK(mode IN ('IDLE', 'MESSAGE', 'COUNTDOWN', 'ALERT', 'BLACKOUT')),
    payload_json TEXT NOT NULL DEFAULT '{}',
    command_id TEXT,
    effective_at_us INTEGER NOT NULL,
    expires_at_us INTEGER,
    FOREIGN KEY (device_id) REFERENCES stage_devices(device_id) ON DELETE CASCADE,
    FOREIGN KEY (command_id) REFERENCES stage_device_commands(command_id) ON DELETE SET NULL
);

CREATE TABLE live_video_sources (
    source_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    source_class TEXT NOT NULL CHECK(source_class IN ('LOCAL_CAMERA', 'USB_CAPTURE', 'NETWORK_STREAM')),
    execution_device_id TEXT,
    profile_id TEXT,
    endpoint_ref TEXT NOT NULL DEFAULT '',
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    config_json TEXT NOT NULL DEFAULT '{}',
    required INTEGER NOT NULL DEFAULT 0 CHECK(required IN (0, 1)),
    desired_enabled INTEGER NOT NULL DEFAULT 1 CHECK(desired_enabled IN (0, 1)),
    readiness TEXT NOT NULL DEFAULT 'UNKNOWN' CHECK(readiness IN ('READY', 'WARNING', 'ADVISORY', 'BLOCKER', 'UNKNOWN')),
    last_observed_at_us INTEGER,
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(project_id) ON DELETE CASCADE,
    FOREIGN KEY (execution_device_id) REFERENCES stage_devices(device_id) ON DELETE SET NULL
);

CREATE INDEX live_video_sources_project_idx
    ON live_video_sources(project_id, source_class, name);

CREATE TABLE network_observations (
    observation_id TEXT PRIMARY KEY,
    target_kind TEXT NOT NULL CHECK(target_kind IN ('HUB', 'COMPANION', 'STAGE_DEVICE', 'LIVE_SOURCE', 'ENDPOINT')),
    target_id TEXT NOT NULL,
    observed_at_us INTEGER NOT NULL,
    reachability TEXT NOT NULL CHECK(reachability IN ('REACHABLE', 'UNREACHABLE', 'UNKNOWN')),
    transport_state TEXT NOT NULL DEFAULT 'UNKNOWN',
    latency_ms REAL,
    jitter_ms REAL,
    address TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    details_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX network_observations_target_time_idx
    ON network_observations(target_kind, target_id, observed_at_us DESC);

-- +goose Down
DROP INDEX IF EXISTS network_observations_target_time_idx;
DROP TABLE IF EXISTS network_observations;
DROP INDEX IF EXISTS live_video_sources_project_idx;
DROP TABLE IF EXISTS live_video_sources;
DROP TABLE IF EXISTS stage_display_state;
DROP INDEX IF EXISTS stage_device_commands_idempotency_idx;
DROP INDEX IF EXISTS stage_device_commands_correlation_idx;
DROP INDEX IF EXISTS stage_device_commands_device_time_idx;
DROP TABLE IF EXISTS stage_device_commands;
DROP INDEX IF EXISTS stage_device_runtime_last_seen_idx;
DROP TABLE IF EXISTS stage_device_runtime_state;
DROP INDEX IF EXISTS stage_devices_project_kind_idx;
DROP TABLE IF EXISTS stage_devices;
